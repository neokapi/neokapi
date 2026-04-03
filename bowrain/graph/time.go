package graph

import "time"

// Supported time formats for parsing validity timestamps.
var timeFormats = []string{
	time.RFC3339Nano,
	time.RFC3339,
	"2006-01-02T15:04:05",
	"2006-01-02",
}

// parseTime tries multiple time formats.
func parseTime(s string) (time.Time, error) {
	var lastErr error
	for _, layout := range timeFormats {
		t, err := time.Parse(layout, s)
		if err == nil {
			return t, nil
		}
		lastErr = err
	}
	return time.Time{}, lastErr
}

// formatTime formats a time as RFC3339.
func formatTime(t time.Time) string {
	return t.UTC().Format(time.RFC3339)
}
