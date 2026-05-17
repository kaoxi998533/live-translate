package quota

import (
	"encoding/json"
	"net/http"

	"github.com/live-translate-platform/api/internal/auth"
	"github.com/live-translate-platform/api/internal/store"
)

type CurrentResponse struct {
	PeriodStart          string `json:"periodStart"`
	PeriodEnd            string `json:"periodEnd"`
	WeeklyLimitSeconds   int    `json:"weeklyLimitSeconds"`
	UsedSeconds          int    `json:"usedSeconds"`
	RemainingSeconds     int    `json:"remainingSeconds"`
	UsageRefreshTimezone string `json:"usageRefreshTimezone"`
}

func HandleCurrent(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	entitlement := store.Default.Entitlement(user.ID)

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(CurrentResponse{
		PeriodStart:          entitlement.PeriodStart.Format("2006-01-02T15:04:05Z07:00"),
		PeriodEnd:            entitlement.PeriodEnd.Format("2006-01-02T15:04:05Z07:00"),
		WeeklyLimitSeconds:   entitlement.WeeklyLimitSeconds,
		UsedSeconds:          entitlement.UsedSeconds,
		RemainingSeconds:     entitlement.RemainingSeconds,
		UsageRefreshTimezone: "UTC",
	})
}
