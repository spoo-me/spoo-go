package spoo

import (
	"fmt"
	"strconv"
	"time"
)

// ParseExpiry normalizes human expiry input to a time.Time for the
// request types. Durations ("30m", "72h") are relative to now; bare
// epoch seconds are converted; anything else must parse as ISO 8601.
// Empty input yields the zero time (no expiry).
func ParseExpiry(raw string, now time.Time) (time.Time, error) {
	if raw == "" {
		return time.Time{}, nil
	}
	if d, err := time.ParseDuration(raw); err == nil {
		if d <= 0 {
			return time.Time{}, fmt.Errorf("expiry duration must be positive, got %q", raw)
		}
		return now.Add(d).UTC(), nil
	}
	if epoch, err := strconv.ParseInt(raw, 10, 64); err == nil {
		return time.Unix(epoch, 0).UTC(), nil
	}
	for _, layout := range timestampLayouts {
		if t, err := time.Parse(layout, raw); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("unrecognized expiry %q (want a duration, epoch seconds, or ISO 8601)", raw)
}
