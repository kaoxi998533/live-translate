package auth

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/live-translate-platform/api/internal/store"
)

type contextKey string

const userContextKey contextKey = "user"

type User struct {
	ID    string `json:"id"`
	Email string `json:"email"`
}

type credentialsRequest struct {
	Email       string `json:"email"`
	Password    string `json:"password"`
	DisplayName string `json:"displayName"`
}

type authResponse struct {
	Token string     `json:"token"`
	User  store.User `json:"user"`
}

type tokenPayload struct {
	UserID string `json:"userId"`
	Exp    int64  `json:"exp"`
}

func HandleRegister(w http.ResponseWriter, r *http.Request) {
	var request credentialsRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if !validCredentials(request.Email, request.Password) {
		writeError(w, http.StatusBadRequest, "valid email and password are required")
		return
	}

	user, err := store.Default.Register(strings.ToLower(request.Email), request.Password, request.DisplayName)
	if err == store.ErrEmailTaken {
		writeError(w, http.StatusConflict, "email already registered")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not register user")
		return
	}

	writeJSON(w, http.StatusCreated, authResponse{Token: signToken(user.ID), User: user})
}

func HandleLogin(w http.ResponseWriter, r *http.Request) {
	var request credentialsRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	user, err := store.Default.Authenticate(strings.ToLower(request.Email), request.Password)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "invalid email or password")
		return
	}

	writeJSON(w, http.StatusOK, authResponse{Token: signToken(user.ID), User: user})
}

func RequireUser(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if token == "" {
			writeError(w, http.StatusUnauthorized, "missing bearer token")
			return
		}

		payload, ok := verifyToken(token)
		if !ok || payload.Exp < time.Now().Unix() {
			writeError(w, http.StatusUnauthorized, "invalid bearer token")
			return
		}

		user, exists := store.Default.User(payload.UserID)
		if !exists {
			writeError(w, http.StatusUnauthorized, "user not found")
			return
		}

		ctx := context.WithValue(r.Context(), userContextKey, User{ID: user.ID, Email: user.Email})
		next(w, r.WithContext(ctx))
	}
}

func UserFromContext(ctx context.Context) User {
	user, _ := ctx.Value(userContextKey).(User)
	return user
}

func validCredentials(email string, password string) bool {
	return strings.Contains(email, "@") && len(password) >= 8
}

func signToken(userID string) string {
	payload := tokenPayload{
		UserID: userID,
		Exp:    time.Now().Add(30 * 24 * time.Hour).Unix(),
	}
	body, _ := json.Marshal(payload)
	encoded := base64.RawURLEncoding.EncodeToString(body)
	signature := sign(encoded)
	return encoded + "." + signature
}

func verifyToken(token string) (tokenPayload, bool) {
	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		return tokenPayload{}, false
	}
	if !hmac.Equal([]byte(sign(parts[0])), []byte(parts[1])) {
		return tokenPayload{}, false
	}
	body, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return tokenPayload{}, false
	}
	var payload tokenPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		return tokenPayload{}, false
	}
	return payload, true
}

func sign(value string) string {
	mac := hmac.New(sha256.New, []byte(env("JWT_SECRET", "dev-secret-change-me")))
	mac.Write([]byte(value))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func env(key string, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}
