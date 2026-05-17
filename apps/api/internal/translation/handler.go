package translation

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/live-translate-platform/api/internal/auth"
	"github.com/live-translate-platform/api/internal/store"
)

type CreateSessionRequest struct {
	PartyALanguage string `json:"partyALanguage"`
	PartyBLanguage string `json:"partyBLanguage"`
	InputMode      string `json:"inputMode"`
}

type CreateSessionResponse struct {
	ID               string `json:"id"`
	Status           string `json:"status"`
	StartedAt        string `json:"startedAt"`
	RemainingSeconds int    `json:"remainingSeconds"`
}

type AddUsageRequest struct {
	Seconds int `json:"seconds"`
}

func HandleCreateSession(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	var request CreateSessionRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if request.PartyALanguage == "" || request.PartyBLanguage == "" || request.InputMode == "" {
		writeError(w, http.StatusBadRequest, "partyALanguage, partyBLanguage, and inputMode are required")
		return
	}

	session, entitlement, err := store.Default.CreateSession(user.ID, request.PartyALanguage, request.PartyBLanguage, request.InputMode)
	if errors.Is(err, store.ErrQuotaExceeded) {
		writeJSON(w, http.StatusPaymentRequired, entitlement)
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not create translation session")
		return
	}

	writeJSON(w, http.StatusCreated, CreateSessionResponse{
		ID:               session.ID,
		Status:           session.Status,
		StartedAt:        session.StartedAt.Format("2006-01-02T15:04:05Z07:00"),
		RemainingSeconds: entitlement.RemainingSeconds,
	})
}

func HandleAddUsage(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	var request AddUsageRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	entitlement, err := store.Default.AddUsage(user.ID, r.PathValue("id"), request.Seconds)
	if errors.Is(err, store.ErrQuotaExceeded) {
		writeJSON(w, http.StatusPaymentRequired, entitlement)
		return
	}
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, entitlement)
}

func HandleEndSession(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	session, err := store.Default.EndSession(user.ID, r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, session)
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
