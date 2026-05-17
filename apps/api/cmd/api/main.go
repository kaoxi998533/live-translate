package main

import (
	"log"
	"net/http"
	"os"
	"time"

	"github.com/live-translate-platform/api/internal/auth"
	"github.com/live-translate-platform/api/internal/billing"
	"github.com/live-translate-platform/api/internal/entitlement"
	"github.com/live-translate-platform/api/internal/quota"
	"github.com/live-translate-platform/api/internal/realtime"
	"github.com/live-translate-platform/api/internal/translation"
)

func main() {
	addr := env("API_ADDR", ":8080")

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", healthz)
	mux.HandleFunc("POST /v1/auth/register", auth.HandleRegister)
	mux.HandleFunc("POST /v1/auth/login", auth.HandleLogin)
	mux.HandleFunc("GET /v1/me", auth.RequireUser(handleMe))
	mux.HandleFunc("GET /v1/entitlements", auth.RequireUser(entitlement.HandleGet))
	mux.HandleFunc("GET /v1/quota/current", auth.RequireUser(quota.HandleCurrent))
	mux.HandleFunc("POST /v1/translation/sessions", auth.RequireUser(translation.HandleCreateSession))
	mux.HandleFunc("POST /v1/translation/sessions/{id}/usage", auth.RequireUser(translation.HandleAddUsage))
	mux.HandleFunc("POST /v1/translation/sessions/{id}/end", auth.RequireUser(translation.HandleEndSession))
	mux.HandleFunc("POST /v1/realtime/client-secret", auth.RequireUser(realtime.HandleClientSecret))
	mux.HandleFunc("POST /v1/billing/stripe/webhook", billing.HandleStripeWebhook)

	server := &http.Server{
		Addr:              addr,
		Handler:           withRequestLog(withCORS(mux)),
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

func handleMe(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	writeJSON(w, http.StatusOK, user)
}

func withRequestLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("%s %s %s", r.Method, r.URL.Path, time.Since(start).Round(time.Millisecond))
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
