package spoo

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"
)

// Timestamp is a time.Time that absorbs the API's mixed wire formats:
// some endpoints emit Unix seconds, others ISO 8601 strings, and
// nullable fields emit null. The zero value means "not set" (null on
// the wire). It embeds time.Time, so all its methods are available.
//
// Request fields use plain time.Time and always serialize as RFC 3339,
// which every endpoint accepts.
type Timestamp struct {
	time.Time
}

// timestampLayouts are the string formats seen on the wire, tried in
// order. Layouts without an offset parse as UTC.
var timestampLayouts = []string{
	time.RFC3339Nano,
	"2006-01-02T15:04:05.999999999",
	"2006-01-02",
}

// UnmarshalJSON accepts Unix seconds, ISO 8601 strings, null, and the
// empty string (the latter two yield the zero value).
func (t *Timestamp) UnmarshalJSON(data []byte) error {
	s := string(data)
	if s == "null" || s == `""` {
		t.Time = time.Time{}
		return nil
	}
	if strings.HasPrefix(s, `"`) {
		var raw string
		if err := json.Unmarshal(data, &raw); err != nil {
			return err
		}
		for _, layout := range timestampLayouts {
			if parsed, err := time.Parse(layout, raw); err == nil {
				t.Time = parsed
				return nil
			}
		}
		return fmt.Errorf("spoo: unrecognized timestamp %q", raw)
	}
	var epoch float64
	if err := json.Unmarshal(data, &epoch); err != nil {
		return fmt.Errorf("spoo: unrecognized timestamp %s: %w", s, err)
	}
	sec, frac := math.Modf(epoch)
	t.Time = time.Unix(int64(sec), int64(math.Round(frac*1e9))).UTC()
	return nil
}

// MarshalJSON writes RFC 3339, or null for the zero value.
func (t Timestamp) MarshalJSON() ([]byte, error) {
	if t.IsZero() {
		return []byte("null"), nil
	}
	return json.Marshal(t.Time)
}
