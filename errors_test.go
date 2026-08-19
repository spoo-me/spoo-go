package spoo

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/spoo-me/spoo-go/option"
)

func TestErrorParsesRateLimitAndRequestID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-Request-ID", "req-abc123")
		w.Header().Set("X-RateLimit-Limit", "100")
		w.Header().Set("X-RateLimit-Remaining", "0")
		w.Header().Set("X-RateLimit-Reset", "1755500000")
		w.Header().Set("Retry-After", "30")
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte(`{"error":"rate limit exceeded","code":"rate_limit_exceeded"}`))
	}))
	defer srv.Close()

	c := NewClient(option.WithBaseURL(srv.URL), option.WithMaxRetries(0))
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

// 451 is the live safety takedown: integrators branch on it to tell
// "the link was removed" apart from "something broke".
func TestBlocked451(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-Error-Code", "blocked")
		w.WriteHeader(http.StatusUnavailableForLegalReasons)
		w.Write([]byte(`{"error":"This link has been blocked","code":"blocked"}`))
	}))
	defer srv.Close()

	c := NewClient(option.WithBaseURL(srv.URL))
	_, err := c.PublicStats(context.Background(), "scam", PublicStatsQuery{})
	if !IsBlocked(err) {
		t.Fatalf("err = %v, want IsBlocked", err)
	}
	if !errors.Is(err, ErrLinkBlocked) {
		t.Fatalf("err = %v, want ErrLinkBlocked sentinel", err)
	}
	var apiErr *Error
	if !errors.As(err, &apiErr) || apiErr.Code != "blocked" {
		t.Fatalf("err = %v, want code blocked", err)
	}
	if IsBlocked(&Error{StatusCode: 404}) {
		t.Fatal("404 must not read as blocked")
	}
}

// An edge-composed 451 whose body is HTML still yields a usable code
// via the X-Error-Code header fallback.
func TestBlocked451EdgeComposedBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-Error-Code", "blocked")
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusUnavailableForLegalReasons)
		w.Write([]byte(`<!doctype html><title>451</title>`))
	}))
	defer srv.Close()

	c := NewClient(option.WithBaseURL(srv.URL))
	_, err := c.Me(context.Background())
	var apiErr *Error
	if !errors.As(err, &apiErr) || apiErr.Code != "blocked" || !IsBlocked(err) {
		t.Fatalf("err = %v, want header-derived blocked code", err)
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
			w.Write([]byte(`{"error":"invalid refresh token","code":"authentication_error"}`))
			return
		}
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":"token expired","code":"authentication_error"}`))
	}))
	defer srv.Close()

	c := NewClient(option.WithBaseURL(srv.URL), option.WithTokenSource(StaticTokens("deadAT", "deadRT")))
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
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":"authentication required","code":"authentication_error"}`))
	}))
	defer srv.Close()

	c := NewClient(option.WithBaseURL(srv.URL), option.WithAPIKey("spoo_bad"))
	_, err := c.Me(context.Background())
	if errors.Is(err, ErrSessionExpired) || errors.Is(err, ErrLinkPasswordProtected) {
		t.Fatalf("err = %v, want no sentinel on a plain 401", err)
	}
	var apiErr *Error
	if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusUnauthorized {
		t.Fatalf("err = %v, want *Error 401", err)
	}
}
