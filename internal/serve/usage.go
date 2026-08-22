// usage.go — what this machine has spent, read back out of the stats files.
package serve

import (
	"net/http"
	"strconv"
	"time"

	"reasonix/internal/config"
	"reasonix/internal/stats"
)

const (
	usageDefaultDays = 30
	usageMaxDays     = 365
)

// usage answers the panel's one question: what did the last N days cost, in
// tokens and in money. The records have been accumulating since the recorder
// was wired in; nothing here writes.
func (s *Server) usage(w http.ResponseWriter, r *http.Request) {
	days := usageDefaultDays
	if raw := r.URL.Query().Get("days"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > usageMaxDays {
			refuse(w, http.StatusBadRequest, codeBadValue, "days must be between 1 and 365",
				map[string]any{"field": "days", "allowed": "1-365"})
			return
		}
		days = parsed
	}
	// Whole days in local time: a range that ended mid-afternoon would drop the
	// morning's turns from "today".
	now := time.Now()
	to := time.Date(now.Year(), now.Month(), now.Day(), 23, 59, 59, 0, now.Location())
	from := to.AddDate(0, 0, -(days - 1))
	report, err := stats.NewWriter(config.StatsDir()).Query(stats.SourceFilter{
		Source: r.URL.Query().Get("source"),
		From:   from,
		To:     to,
	})
	if err != nil {
		refuse(w, http.StatusInternalServerError, "internal.failed", "could not read the usage records", nil)
		return
	}
	writeJSON(w, report)
}
