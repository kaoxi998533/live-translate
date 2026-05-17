package realtime

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/live-translate-platform/api/internal/auth"
	"github.com/live-translate-platform/api/internal/store"
)

type ClientSecretRequest struct {
	PartyALanguage string `json:"partyALanguage"`
	PartyBLanguage string `json:"partyBLanguage"`
	ListenMode     string `json:"listenMode"`
}

type ClientSecretResponse struct {
	Value     string `json:"value"`
	ExpiresAt int64  `json:"expiresAt,omitempty"`
	Model     string `json:"model"`
}

func HandleClientSecret(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	entitlement := store.Default.Entitlement(user.ID)
	if !entitlement.CanTranslate {
		writeJSON(w, http.StatusPaymentRequired, entitlement)
		return
	}

	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		writeError(w, http.StatusServiceUnavailable, "实时翻译服务尚未配置")
		return
	}

	var request ClientSecretRequest
	_ = json.NewDecoder(r.Body).Decode(&request)
	if request.PartyALanguage == "" {
		request.PartyALanguage = "zh"
	}
	if request.PartyBLanguage == "" {
		request.PartyBLanguage = "en"
	}
	if request.ListenMode == "" {
		request.ListenMode = "auto"
	}

	model := env("OPENAI_REALTIME_MODEL", "gpt-realtime")
	transcriptionModel := env("OPENAI_REALTIME_TRANSCRIPTION_MODEL", "gpt-4o-mini-transcribe")
	payload := map[string]any{
		"expires_after": map[string]any{
			"anchor":  "created_at",
			"seconds": envInt("OPENAI_REALTIME_SESSION_TTL_SECONDS", 600),
		},
		"session": map[string]any{
			"type":              "realtime",
			"model":             model,
			"output_modalities": []string{"audio"},
			"instructions":      instructions(request.PartyALanguage, request.PartyBLanguage),
			"tracing":           "auto",
			"audio": map[string]any{
				"input": map[string]any{
					"transcription": map[string]any{
						"model": transcriptionModel,
					},
					"turn_detection": turnDetection(request.ListenMode),
				},
				"output": map[string]any{
					"voice": env("OPENAI_REALTIME_VOICE", "marin"),
					"speed": envFloat("OPENAI_REALTIME_OUTPUT_SPEED", 1),
				},
			},
		},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "无法创建实时翻译请求")
		return
	}

	req, err := http.NewRequest(http.MethodPost, "https://api.openai.com/v1/realtime/client_secrets", bytes.NewReader(body))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "无法创建实时翻译请求")
		return
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		writeError(w, http.StatusBadGateway, "暂时无法连接实时翻译服务")
		return
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		writeJSON(w, resp.StatusCode, map[string]string{
			"error":  "实时翻译服务拒绝了本次请求",
			"detail": string(respBody),
		})
		return
	}

	var openAIResponse struct {
		Value     string `json:"value"`
		ExpiresAt int64  `json:"expires_at"`
	}
	if err := json.Unmarshal(respBody, &openAIResponse); err != nil {
		writeError(w, http.StatusBadGateway, "实时翻译服务返回异常")
		return
	}

	writeJSON(w, http.StatusOK, ClientSecretResponse{
		Value:     openAIResponse.Value,
		ExpiresAt: openAIResponse.ExpiresAt,
		Model:     model,
	})
}

func turnDetection(listenMode string) any {
	if listenMode == "hold" {
		return nil
	}
	return map[string]any{
		"type":                "server_vad",
		"threshold":           envFloat("OPENAI_REALTIME_VAD_THRESHOLD", 0.78),
		"prefix_padding_ms":   envInt("OPENAI_REALTIME_VAD_PREFIX_PADDING_MS", 250),
		"silence_duration_ms": envInt("OPENAI_REALTIME_VAD_SILENCE_DURATION_MS", 450),
		"create_response":     envBool("OPENAI_REALTIME_VAD_CREATE_RESPONSE", true),
		"interrupt_response":  envBool("OPENAI_REALTIME_VAD_INTERRUPT_RESPONSE", true),
	}
}

func instructions(partyALanguage string, partyBLanguage string) string {
	partyA := languageName(partyALanguage)
	partyB := languageName(partyBLanguage)
	return "You are a live two-way interpreter for a conversation between two people. " +
		"Person A speaks " + partyA + ". Person B speaks " + partyB + ". " +
		"When you hear " + partyA + ", translate it into natural " + partyB + ". " +
		"When you hear " + partyB + ", translate it into natural " + partyA + ". " +
		"Silently infer which of the two configured languages is being spoken. " +
		"Only output the translation for the other person. Do not explain, summarize, answer questions, or add commentary. " +
		"Preserve names, numbers, units, tone, politeness, and intent. Keep the result concise and spoken naturally."
}

func languageName(code string) string {
	switch code {
	case "ar":
		return "Arabic"
	case "de":
		return "German"
	case "en":
		return "English"
	case "es":
		return "Spanish"
	case "fr":
		return "French"
	case "hi":
		return "Hindi"
	case "id":
		return "Indonesian"
	case "it":
		return "Italian"
	case "ja":
		return "Japanese"
	case "ko":
		return "Korean"
	case "pt":
		return "Portuguese"
	case "ru":
		return "Russian"
	case "th":
		return "Thai"
	case "vi":
		return "Vietnamese"
	case "zh":
		return "Mandarin Chinese"
	default:
		return code
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

func env(key string, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}

func envInt(key string, fallback int) int {
	var value int
	if _, err := fmt.Sscanf(os.Getenv(key), "%d", &value); err != nil {
		return fallback
	}
	return value
}

func envFloat(key string, fallback float64) float64 {
	var value float64
	if _, err := fmt.Sscanf(os.Getenv(key), "%f", &value); err != nil {
		return fallback
	}
	return value
}

func envBool(key string, fallback bool) bool {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value != "false"
}
