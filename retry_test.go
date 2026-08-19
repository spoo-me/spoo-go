package spoo

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/spoo-me/spoo-go/option"
)

// fastRetries makes backoff negligible so retry tests run instantly.
func fastRetries(c *Client) { c.retryBase = time.Millisecond }

func TestRetriesTransient5xxThenSucceeds(t *testing.T) {
	var attempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if attempts.Add(1) <= 2 {
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write([]byte(`{"error":"upstream hiccup"}`))
			return
		}
		w.Write([]byte(`{"user":{"id":"1","email":"a@b.c"}}`))
	}))
	defer srv.Close()

	c := NewClient(option.WithBaseURL(srv.URL))
	fastRetries(c)
	u, err := c.Me(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if u.Email != "a@b.c" {
		t.Fatalf("user = %+v", u)
	}
	if attempts.Load() != 3 {
		t.Fatalf("attempts = %d, want 3 (two retries)", attempts.Load())
	}
}

func TestRetriesExhaustedReturnTheError(t *testing.T) {
	var attempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusBadGateway)
		w.Write([]byte(`{"error":"still down","code":"internal_error"}`))
	}))
	defer srv.Close()

	c := NewClient(option.WithBaseURL(srv.URL), option.WithMaxRetries(2))
	fastRetries(c)
	_, err := c.Me(context.Background())
	var apiErr *Error
	if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusBadGateway {
		t.Fatalf("err = %v, want *Error with 502", err)
	}
	if attempts.Load() != 3 {
		t.Fatalf("attempts = %d, want 3", attempts.Load())
	}
}

func TestNoRetryOnClientError(t *testing.T) {
	var attempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error":"bad alias","code":"validation_error"}`))
	}))
	defer srv.Close()

	c := NewClient(option.WithBaseURL(srv.URL))
	fastRetries(c)
	if _, err := c.Me(context.Background()); err == nil {
		t.Fatal("want error")
	}
	if attempts.Load() != 1 {
		t.Fatalf("attempts = %d, want 1 (4xx must not retry)", attempts.Load())
	}
}

func TestWithMaxRetriesZeroDisablesRetries(t *testing.T) {
	var attempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	c := NewClient(option.WithBaseURL(srv.URL), option.WithMaxRetries(0))
	fastRetries(c)
	if _, err := c.Me(context.Background()); err == nil {
		t.Fatal("want error")
	}
	if attempts.Load() != 1 {
		t.Fatalf("attempts = %d, want 1", attempts.Load())
	}
}

// Retry-After is authoritative over computed backoff: with a pathological
// base delay, a Retry-After: 0 hint must make the retry immediate.
func TestRetryHonorsRetryAfter(t *testing.T) {
	var attempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if attempts.Add(1) == 1 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			w.Write([]byte(`{"error":"slow down"}`))
			return
		}
		w.Write([]byte(`{"user":{"id":"1"}}`))
	}))
	defer srv.Close()

	c := NewClient(option.WithBaseURL(srv.URL))
	c.retryBase = time.Hour // would hang the test if backoff were used
	start := time.Now()
	if _, err := c.Me(context.Background()); err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("retry took %v, Retry-After: 0 was not honored", elapsed)
	}
	if attempts.Load() != 2 {
		t.Fatalf("attempts = %d, want 2", attempts.Load())
	}
}

// failOnceTransport drops the first request at the transport level to
// simulate a connection error, then delegates.
type failOnceTransport struct {
	failed atomic.Bool
	next   http.RoundTripper
}

func (f *failOnceTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if f.failed.CompareAndSwap(false, true) {
		return nil, errors.New("connection reset by peer")
	}
	return f.next.RoundTrip(req)
}

func TestRetriesConnectionErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"user":{"id":"1"}}`))
	}))
	defer srv.Close()

	hc := &http.Client{Transport: &failOnceTransport{next: http.DefaultTransport}}
	c := NewClient(option.WithBaseURL(srv.URL), option.WithHTTPClient(hc))
	fastRetries(c)
	if _, err := c.Me(context.Background()); err != nil {
		t.Fatalf("connection error was not retried: %v", err)
	}
}

// A retried POST must replay its full body on the next attempt.
func TestRetryReplaysRequestBody(t *testing.T) {
	var attempts atomic.Int32
	var lastBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if attempts.Add(1) == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		lastBody = body
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"short_url":"https://spoo.me/x","alias":"x"}`))
	}))
	defer srv.Close()

	c := NewClient(option.WithBaseURL(srv.URL))
	fastRetries(c)
	if _, err := c.Shorten(context.Background(), ShortenRequest{LongURL: "https://example.com"}); err != nil {
		t.Fatal(err)
	}
	if string(lastBody) != `{"long_url":"https://example.com"}` {
		t.Fatalf("replayed body = %q", lastBody)
	}
}

// A POST answered 500 may have done the work already; replaying it
// could create the resource twice, so it must surface immediately.
func TestPostNotRetriedOn500(t *testing.T) {
	var attempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":"boom","code":"internal_error"}`))
	}))
	defer srv.Close()

	c := NewClient(option.WithBaseURL(srv.URL))
	fastRetries(c)
	_, err := c.Shorten(context.Background(), ShortenRequest{LongURL: "https://example.com"})
	if err == nil {
		t.Fatal("want the 500 surfaced")
	}
	if attempts.Load() != 1 {
		t.Fatalf("attempts = %d, want 1 (non-idempotent 500 must not retry)", attempts.Load())
	}
}

// 429 and 503 mean the server provably did no work, so even a POST
// retries on them.
func TestPostRetriedOn429(t *testing.T) {
	var attempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if attempts.Add(1) == 1 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			w.Write([]byte(`{"error":"slow down","code":"rate_limit_exceeded"}`))
			return
		}
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"short_url":"https://spoo.me/x","alias":"x"}`))
	}))
	defer srv.Close()

	c := NewClient(option.WithBaseURL(srv.URL))
	fastRetries(c)
	if _, err := c.Shorten(context.Background(), ShortenRequest{LongURL: "https://example.com"}); err != nil {
		t.Fatal(err)
	}
	if attempts.Load() != 2 {
		t.Fatalf("attempts = %d, want 2", attempts.Load())
	}
}

// GET is idempotent, so a 500 retries.
func TestGetRetriedOn500(t *testing.T) {
	var attempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if attempts.Add(1) == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(`{"error":"boom","code":"internal_error"}`))
			return
		}
		w.Write([]byte(`{"user":{"id":"1"}}`))
	}))
	defer srv.Close()

	c := NewClient(option.WithBaseURL(srv.URL))
	fastRetries(c)
	if _, err := c.Me(context.Background()); err != nil {
		t.Fatal(err)
	}
	if attempts.Load() != 2 {
		t.Fatalf("attempts = %d, want 2", attempts.Load())
	}
}

// A dropped connection on a POST may have reached the server, so it
// must not be replayed either.
func TestPostNotRetriedOnConnectionError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"short_url":"https://spoo.me/x","alias":"x"}`))
	}))
	defer srv.Close()

	failer := &failOnceTransport{next: http.DefaultTransport}
	hc := &http.Client{Transport: failer}
	c := NewClient(option.WithBaseURL(srv.URL), option.WithHTTPClient(hc))
	fastRetries(c)
	if _, err := c.Shorten(context.Background(), ShortenRequest{LongURL: "https://example.com"}); err == nil {
		t.Fatal("want the connection error surfaced, not a replayed POST")
	}
}

func TestRetryStopsOnContextCancel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	c := NewClient(option.WithBaseURL(srv.URL))
	c.retryBase = time.Hour
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err := c.Me(ctx)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want context.DeadlineExceeded", err)
	}
}
