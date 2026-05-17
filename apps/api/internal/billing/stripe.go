package billing

import (
	"encoding/json"
	"net/http"
)

type WebhookResponse struct {
	Received bool `json:"received"`
}

func HandleStripeWebhook(w http.ResponseWriter, _ *http.Request) {
	// Verify Stripe signature and persist subscription events before production.
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(WebhookResponse{Received: true})
}
