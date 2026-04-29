package nudge

import (
	"fmt"
	"strings"
	"time"
)

// SimpleCron parses a very limited cron-like expression for nudges.
// Supported formats: "every N minutes", "every N hours", "daily at HH:MM", "hourly"
func SimpleCron(expr string, from time.Time) (*time.Time, error) {
	expr = strings.ToLower(strings.TrimSpace(expr))
	if expr == "" {
		return nil, fmt.Errorf("empty cron expression")
	}
	if expr == "hourly" {
		next := from.Add(time.Hour).Truncate(time.Hour)
		return &next, nil
	}
	if strings.HasPrefix(expr, "daily at ") {
		timeStr := strings.TrimPrefix(expr, "daily at ")
		ref := time.Date(from.Year(), from.Month(), from.Day(), 0, 0, 0, 0, from.Location())
		t, err := time.Parse("15:04", timeStr)
		if err != nil {
			return nil, err
		}
		next := ref.Add(time.Hour*time.Duration(t.Hour()) + time.Minute*time.Duration(t.Minute()))
		if next.Before(from) || next.Equal(from) {
			next = next.Add(24 * time.Hour)
		}
		return &next, nil
	}
	if strings.HasPrefix(expr, "every ") {
		parts := strings.Fields(strings.TrimPrefix(expr, "every "))
		if len(parts) >= 2 {
			var n int
			fmt.Sscanf(parts[0], "%d", &n)
			if n > 0 {
				unit := parts[1]
				if strings.HasPrefix(unit, "minute") {
					next := from.Add(time.Duration(n) * time.Minute)
					return &next, nil
				}
				if strings.HasPrefix(unit, "hour") {
					next := from.Add(time.Duration(n) * time.Hour)
					return &next, nil
				}
			}
		}
	}
	return nil, fmt.Errorf("unsupported cron expression: %s", expr)
}

// FormatNudges returns a banner string for pending nudges.
func FormatNudges(nudges []Nudge) string {
	if len(nudges) == 0 {
		return ""
	}
	var lines []string
	lines = append(lines, "📌 Nudge:" )
	for i, n := range nudges {
		lines = append(lines, fmt.Sprintf("   %d. [%s] %s", i+1, n.Type, n.Text))
	}
	lines = append(lines, "   Ketik /nudge run <nomor> untuk eksekusi sekarang.")
	return strings.Join(lines, "\n")
}
