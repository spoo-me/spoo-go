package spoo

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestErrorParsesRateLimitAndRequestID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Request-ID", "req-abc123")
		w.Header().Set("X-RateLimit-Limit", "100")
		w.Header().Set("X-RateLimit-Remaining", "0")
		w.Header().Set("X-RateLimit-Reset", "1755500000")
		w.Header().Set("Retry-After", "30")
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte(`{"error":"rate limit exceeded","code":"RATE_LIMIT_ERROR"}`))
	}))
	defer srv.Close()

	c := NewClient(WithBaseURL(srv.URL), WithMaxRetries(0))
	_, err := c.Me(context.Background())
	if !IsRateLimited(err) {
		t.Fatalf("err = %v, want IsRateLimited", err)
	}
	var apiErr *Error
	if !errors.As(err, &apiErr) {
		t.Fatalf("err = %v, want *Error", err)
	}
	if apiErr.RequestID != "req-abc123" {
		t.Errorf("RequestID = %q", apiErr.RequestID)
	}
	rl := apiErr.RateLimit
	if rl.Limit != 100 || rl.Remaining != 0 {
		t.Errorf("RateLimit = %+v", rl)
	}
	if !rl.Reset.Equal(time.Unix(1755500000, 0)) {
		t.Errorf("Reset = %v, want epoch 1755500000", rl.Reset)
	}
	if rl.RetryAfter != 30*time.Second {
		t.Errorf("RetryAfter = %v, want 30s", rl.RetryAfter)
	}
}

func TestIsRateLimitedRejectsOtherErrors(t *testing.T) {
	if IsRateLimited(errors.New("nope")) {
		t.Fatal("plain errors must not read as rate-limited")
	}
	if IsRateLimited(&Error{StatusCode: 404}) {
		t.Fatal("404 must not read as rate-limited")
	}
	if !IsNotFound(&Error{StatusCode: 404}) {
		t.Fatal("404 must read as not-found")
	}
}

// A dead refresh token is the session-expired condition — surfaced as a
// typed sentinel, not a prompt string.
func TestRefreshRejectionIsSessionExpired(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/auth/device/refresh" {
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`{"error":"invalid refresh token","code":"AUTHENTICATION_ERROR"}`))
			return
		}
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":"token expired","code":"AUTHENTICATION_ERROR"}`))
	}))
	defer srv.Close()

	c := NewClient(WithBaseURL(srv.URL), WithTokenSource(StaticTokens("deadAT", "deadRT")))
	_, err := c.Me(context.Background())
	if !errors.Is(err, ErrSessionExpired) {
		t.Fatalf("err = %v, want ErrSessionExpired", err)
	}
	var apiErr *Error
	if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusUnauthorized {
		t.Fatalf("err = %v, want the wrapped *Error too", err)
	}
}

// The sentinel must not fire for auth failures that are not about the
// refresh token: a plain 401 without a refresh token is just an *Error.
func TestPlain401IsNotSessionExpired(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":"authentication required","code":"AUTHENTICATION_ERROR"}`))
	}))
	defer srv.Close()

	c := NewClient(WithBaseURL(srv.URL), WithAPIKey("spoo_bad"))
	_, err := c.Me(context.Background())
	if errors.Is(err, ErrSessionExpired) || errors.Is(err, ErrLinkPasswordProtected) {
		t.Fatalf("err = %v, want no sentinel on a plain 401", err)
	}
	var apiErr *Error
	if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusUnauthorized {
		t.Fatalf("err = %v, want *Error 401", err)
	}
}
