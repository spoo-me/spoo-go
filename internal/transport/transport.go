// Package transport carries the HTTP machinery behind the spoo client:
// retry policy, redirect header hygiene, and download filename parsing.
// Nothing here is consumer-facing.
package transport

import (
	"context"
	"io"
	"math/rand/v2"
	"mime"
	"net/http"
	"strconv"
	"time"
)

const (
	// RetryBaseDelay is the exponential-backoff unit.
	RetryBaseDelay = 500 * time.Millisecond
	// retryMaxDelay caps a single computed backoff.
	retryMaxDelay = 10 * time.Second
)

// IdempotentMethod reports whether an HTTP method is safe to replay
// unconditionally. The set matches the TS and Python SDKs exactly:
// GET, PUT, DELETE.
func IdempotentMethod(method string) bool {
	switch method {
	case http.MethodGet, http.MethodPut, http.MethodDelete:
		return true
	}
	return false
}

// RetryableStatus reports whether a response status is worth retrying
// for the given method. Idempotent methods retry on 408, 429, 500,
// 502, 503 and 504. Non-idempotent methods (POST, PATCH) retry only on
// 429 and 503, the statuses where the server provably did no work; a
// replayed POST after a 500 or 504 could have created the resource
// twice. The sets match the TS and Python SDKs exactly.
func RetryableStatus(method string, status int) bool {
	switch status {
	case http.StatusTooManyRequests, http.StatusServiceUnavailable:
		return true
	case http.StatusRequestTimeout,
		http.StatusInternalServerError,
		http.StatusBadGateway,
		http.StatusGatewayTimeout:
		return IdempotentMethod(method)
	}
	return false
}

// RetryDelay computes the wait before retry number attempt+1. A
// parsable Retry-After header is authoritative; otherwise exponential
// backoff from base with equal jitter (half fixed, half random) capped
// at retryMaxDelay.
func RetryDelay(base time.Duration, attempt int, retryAfter string) time.Duration {
	if d, ok := ParseRetryAfter(retryAfter, time.Now()); ok {
		return d
	}
	d := min(base<<attempt, retryMaxDelay)
	return d/2 + rand.N(d/2+1) //nolint:gosec // math/rand jitter; no security material
}

// ParseRetryAfter reads a Retry-After header value: delay seconds or an
// HTTP date (RFC 9110 §10.2.3). ok is false when absent or malformed.
func ParseRetryAfter(v string, now time.Time) (_ time.Duration, ok bool) {
	if v == "" {
		return 0, false
	}
	if secs, err := strconv.Atoi(v); err == nil {
		return max(time.Duration(secs)*time.Second, 0), true
	}
	if at, err := http.ParseTime(v); err == nil {
		return max(at.Sub(now), 0), true
	}
	return 0, false
}

// Sleep waits for d or until ctx is done, whichever comes first.
func Sleep(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Drain discards a response about to be retried so the underlying
// connection can be reused.
func Drain(resp *http.Response) {
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
	_ = resp.Body.Close()
}

// WithRedirectHeaderStrip copies hc and extends its redirect policy: Go
// forwards custom headers on redirects, including cross-origin ones.
// Attribution belongs to the spoo API only, so X-Spoo-Client is dropped
// whenever a redirect leaves the original host. Go itself strips
// Authorization on cross-domain hops. The caller's own CheckRedirect,
// when present, still runs afterwards.
func WithRedirectHeaderStrip(hc *http.Client) *http.Client {
	cp := *hc
	next := hc.CheckRedirect
	cp.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if req.URL.Host != via[0].URL.Host {
			req.Header.Del("X-Spoo-Client")
		}
		if next != nil {
			return next(req, via)
		}
		return nil
	}
	return &cp
}

// ContentDispositionFilename extracts the filename from a
// Content-Disposition header. mime.ParseMediaType decodes RFC 2231/5987
// extended params, so filename* surfaces under the "filename" key
// already decoded. ok is false when the header carries no usable name.
func ContentDispositionFilename(disposition string) (string, bool) {
	if _, params, err := mime.ParseMediaType(disposition); err == nil {
		if name := params["filename"]; name != "" {
			return name, true
		}
	}
	return "", false
}
