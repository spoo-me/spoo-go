package transport

import (
	"net/http"
	"testing"
	"time"
)

// Only a parsable Retry-After longer than maxRetryAfter trips the cap;
// exactly 60s is still honored, and absent or malformed headers fall
// through to computed backoff.
func TestRetryAfterExceedsCap(t *testing.T) {
	tests := []struct {
		v    string
		want bool
	}{
		{"", false},
		{"garbage", false},
		{"0", false},
		{"60", false},
		{"61", true},
		{"3600", true},
		{time.Now().Add(2 * time.Hour).UTC().Format(http.TimeFormat), true},
		{time.Now().Add(5 * time.Second).UTC().Format(http.TimeFormat), false},
	}
	for _, tt := range tests {
		if got := RetryAfterExceedsCap(tt.v); got != tt.want {
			t.Errorf("RetryAfterExceedsCap(%q) = %v, want %v", tt.v, got, tt.want)
		}
	}
}

// The filename is server-supplied and consumers hand it straight to
// os.Create, so path-shaped suggestions must never survive: relative
// traversal reduces to its last segment, absolute paths are rejected
// entirely, and rejection means the caller's synthesized default wins.
func TestContentDispositionFilenameSanitizes(t *testing.T) {
	tests := []struct {
		disposition string
		want        string
		ok          bool
	}{
		// benign passthrough
		{`attachment; filename="stats-launch.xlsx"`, "stats-launch.xlsx", true},
		{`attachment; filename*=UTF-8''sp%C3%B6%C3%B6-export.zip`, "spöö-export.zip", true},
		// relative traversal reduces to the basename
		{`attachment; filename="../../../evil.json"`, "evil.json", true},
		{`attachment; filename="..\..\evil.json"`, "evil.json", true},
		{`attachment; filename="dir/nested/name.csv"`, "name.csv", true},
		// RFC 5987 decoding happens first, then sanitization
		{`attachment; filename*=utf-8''%2e%2e%2f%2e%2e%2fesc.json`, "esc.json", true},
		// absolute paths are rejected outright
		{`attachment; filename="/tmp/absolute-evil.json"`, "", false},
		{`attachment; filename="\\host\share\evil.json"`, "", false},
		{`attachment; filename="C:\evil.json"`, "", false},
		{`attachment; filename="c:/evil.json"`, "", false},
		// names that reduce to nothing
		{`attachment; filename=".."`, "", false},
		{`attachment; filename="."`, "", false},
		{`attachment; filename="dir/"`, "", false},
		{`attachment; filename="dir/.."`, "", false},
		// no usable header at all
		{`attachment`, "", false},
		{``, "", false},
	}
	for _, tt := range tests {
		got, ok := ContentDispositionFilename(tt.disposition)
		if got != tt.want || ok != tt.ok {
			t.Errorf("ContentDispositionFilename(%q) = %q, %v; want %q, %v",
				tt.disposition, got, ok, tt.want, tt.ok)
		}
	}
}
