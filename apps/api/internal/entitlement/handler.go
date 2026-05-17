package entitlement

import (
	"encoding/json"
	"net/http"

	"github.com/live-translate-platform/api/internal/auth"
	"github.com/live-translate-platform/api/internal/store"
)

func HandleGet(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(store.Default.Entitlement(user.ID))
}
