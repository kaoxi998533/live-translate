package billing

import (
	"encoding/json"
	"net/http"
	"os"

	"github.com/live-translate-platform/api/internal/auth"
	"github.com/live-translate-platform/api/internal/store"
)

type WebhookResponse struct {
	Received bool   `json:"received"`
	Provider string `json:"provider"`
}

type CreateOrderRequest struct {
	PlanID   string `json:"planId"`
	Provider string `json:"provider"`
}

func HandleCreateOrder(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	var request CreateOrderRequest
	_ = json.NewDecoder(r.Body).Decode(&request)

	if request.PlanID == "" {
		request.PlanID = "premium"
	}
	if request.Provider == "" {
		request.Provider = os.Getenv("BILLING_PRIMARY_PROVIDER")
	}
	if request.Provider == "" {
		request.Provider = "wechat_pay"
	}
	if request.PlanID != "premium" {
		writeError(w, http.StatusBadRequest, "暂时只能购买高级会员")
		return
	}
	if !validProvider(request.Provider) {
		writeError(w, http.StatusBadRequest, "暂不支持该支付方式")
		return
	}

	order, err := store.Default.CreatePaymentOrder(user.ID, request.PlanID, request.Provider)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "无法创建支付订单")
		return
	}
	writeJSON(w, http.StatusCreated, order)
}

func HandleDevMarkOrderPaid(w http.ResponseWriter, r *http.Request) {
	if os.Getenv("APP_ENV") == "production" {
		writeError(w, http.StatusNotFound, "接口不存在")
		return
	}

	user := auth.UserFromContext(r.Context())
	nextUser, err := store.Default.MarkPaymentOrderPaid(user.ID, r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, nextUser)
}

func HandleStripeWebhook(w http.ResponseWriter, _ *http.Request) {
	// Verify Stripe signature and persist subscription events before production.
	writeJSON(w, http.StatusOK, WebhookResponse{Received: true, Provider: "stripe"})
}

func HandleWeChatPayNotify(w http.ResponseWriter, _ *http.Request) {
	// Verify WeChat Pay platform certificate/signature and decrypt resource before production.
	writeJSON(w, http.StatusOK, WebhookResponse{Received: true, Provider: "wechat_pay"})
}

func validProvider(provider string) bool {
	switch provider {
	case "wechat_pay", "stripe", "apple_iap", "google_play", "alipay":
		return true
	default:
		return false
	}
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
