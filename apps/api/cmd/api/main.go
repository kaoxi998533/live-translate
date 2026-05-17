package main

import (
	"context"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/live-translate-platform/api/internal/auth"
	"github.com/live-translate-platform/api/internal/billing"
	"github.com/live-translate-platform/api/internal/entitlement"
	"github.com/live-translate-platform/api/internal/quota"
	"github.com/live-translate-platform/api/internal/realtime"
	"github.com/live-translate-platform/api/internal/store"
	"github.com/live-translate-platform/api/internal/translation"
)

func main() {
	addr := env("API_ADDR", ":8080")
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := store.Default.Connect(ctx, os.Getenv("DATABASE_URL"), env("MIGRATION_PATH", "../../infra/migrations/001_initial.sql")); err != nil {
		log.Fatalf("database init failed: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", healthz)
	mux.HandleFunc("GET /readyz", readyz)
	mux.HandleFunc("POST /v1/auth/register", auth.HandleRegister)
	mux.HandleFunc("POST /v1/auth/login", auth.HandleLogin)
	mux.HandleFunc("GET /v1/me", auth.RequireUser(handleMe))
	mux.HandleFunc("GET /v1/entitlements", auth.RequireUser(entitlement.HandleGet))
	mux.HandleFunc("GET /v1/quota/current", auth.RequireUser(quota.HandleCurrent))
	mux.HandleFunc("POST /v1/translation/sessions", auth.RequireUser(translation.HandleCreateSession))
	mux.HandleFunc("POST /v1/translation/sessions/{id}/usage", auth.RequireUser(translation.HandleAddUsage))
	mux.HandleFunc("POST /v1/translation/sessions/{id}/end", auth.RequireUser(translation.HandleEndSession))
	mux.HandleFunc("POST /v1/realtime/client-secret", auth.RequireUser(realtime.HandleClientSecret))
	mux.HandleFunc("POST /v1/billing/orders", auth.RequireUser(billing.HandleCreateOrder))
	mux.HandleFunc("POST /v1/dev/billing/orders/{id}/mark-paid", auth.RequireUser(billing.HandleDevMarkOrderPaid))
	mux.HandleFunc("POST /v1/billing/stripe/webhook", billing.HandleStripeWebhook)
	mux.HandleFunc("POST /v1/billing/wechat/notify", billing.HandleWeChatPayNotify)

	server := &http.Server{
		Addr:              addr,
		Handler:           withRequestLog(withSecurityHeaders(withCORS(newRateLimiter(envInt("RATE_LIMIT_PER_MINUTE", 120)).Middleware(mux)))),
		ReadHeaderTimeout: 5 * time.Second,
	}

	log.Printf("api listening on %s", addr)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}

func healthz(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func readyz(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if err := store.Default.Ping(ctx); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "error", "database": "down"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "database": "ok"})
}

func handleMe(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	writeJSON(w, http.StatusOK, user)
}

func withRequestLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("method=%s path=%s remote=%s duration=%s", r.Method, r.URL.Path, clientIP(r), time.Since(start).Round(time.Millisecond))
	})
}

func withSecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		next.ServeHTTP(w, r)
	})
}

func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", env("APP_ORIGIN", "*"))
		w.Header().Set("Access-Control-Allow-Headers", "authorization, content-type")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func env(key string, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}

func envInt(key string, fallback int) int {
	value, err := strconv.Atoi(os.Getenv(key))
	if err != nil || value <= 0 {
		return fallback
	}
	return value
}

func clientIP(r *http.Request) string {
	if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
		return forwarded
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

type rateLimiter struct {
	mu       sync.Mutex
	limit    int
	counters map[string]rateCounter
}

type rateCounter struct {
	windowStart time.Time
	count       int
}

func newRateLimiter(limit int) *rateLimiter {
	return &rateLimiter{limit: limit, counters: make(map[string]rateCounter)}
}

func (rl *rateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodOptions || r.URL.Path == "/healthz" || r.URL.Path == "/readyz" {
			next.ServeHTTP(w, r)
			return
		}
		if !rl.allow(clientIP(r), time.Now()) {
			writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "请求过于频繁，请稍后再试"})
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (rl *rateLimiter) allow(key string, now time.Time) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	counter := rl.counters[key]
	if counter.windowStart.IsZero() || now.Sub(counter.windowStart) >= time.Minute {
		rl.counters[key] = rateCounter{windowStart: now, count: 1}
		return true
	}
	if counter.count >= rl.limit {
		return false
	}
	counter.count++
	rl.counters[key] = counter
	return true
}
