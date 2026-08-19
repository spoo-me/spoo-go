package spoo

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/spoo-me/spoo-go/internal/transport"
)

// Sentinel conditions the API signals on 401 responses. A 401 means one
// of three things — dead session, password-protected link, or plain
// missing auth — and these let callers branch without string matching.
// Both are surfaced through [Error]: test with [errors.Is].
var (
	// ErrSessionExpired marks a device-flow session whose refresh token
	// no longer works. The only recovery is a fresh login.
	ErrSessionExpired = errors.New("session expired")

	// ErrLinkPasswordProtected marks a 401 that is a property of the
	// link, not of the session: the link's stats require the link
	// password, supplied via PublicStatsQuery.Password.
	ErrLinkPasswordProtected = errors.New("link is password protected")

	// ErrLinkBlocked marks a 451: the link was taken down by the
	// safety pipeline because its destination was flagged. This is a
	// verdict on the link, not a transient failure — see also
	// [IsBlocked].
	ErrLinkBlocked = errors.New("link is blocked")
)

// ErrTokenSourceRequired is returned by [Client.ForceRefresh] when the
// client has no refresh-capable TokenSource to rotate.
var ErrTokenSourceRequired = errors.New("no refresh-capable token source configured")

// ErrMissingLongURL is returned by [Client.Shorten] before any request
// goes out when ShortenRequest.LongURL is empty. An empty required
// field is a programming error at the call site, so it fails fast
// instead of spending a round trip on a guaranteed 422.
var ErrMissingLongURL = errors.New("spoo: ShortenRequest.LongURL is required")

// RateLimit is the backend's rate-limit state parsed from the
// X-RateLimit-* and Retry-After response headers (zero when absent).
// The backend reports the shortest rate-limit window that applies to
// the endpoint.
type RateLimit struct {
	// Limit is the request budget of the reported window.
	Limit int
	// Remaining is how much of the budget is left.
	Remaining int
	// Reset is when the reported window resets.
	Reset time.Time
	// RetryAfter is the server-mandated wait, sent on 429 responses.
	RetryAfter time.Duration
}

// Error mirrors the backend's error envelope {error, code, field,
// details} plus the response metadata that matters for handling it
// programmatically.
type Error struct {
	// StatusCode is the HTTP status of the response.
	StatusCode int `json:"-"`
	// Code is the backend's machine-readable error code, an open
	// string enum in lowercase snake_case: "conflict",
	// "authentication_error", "not_found", "rate_limit_exceeded",
	// "payload_too_large", "blocked", "gone", and so on. The one
	// uppercase outlier is "EMAIL_NOT_VERIFIED". Read from the body,
	// with the X-Error-Code header as fallback for the edge-composed
	// responses whose bodies carry no envelope.
	Code string `json:"code"`
	// Message is the human-readable error message.
	Message string `json:"error"`
	// Field names the offending request field on validation errors.
	Field string `json:"field"`
	// Details optionally carries structured context for the error.
	Details any `json:"details"`
	// RequestID is the X-Request-ID header, for support correlation.
	RequestID string `json:"-"`
	// RateLimit carries the parsed X-RateLimit-* headers.
	RateLimit RateLimit `json:"-"`

	sentinel error
}

func (e *Error) Error() string {
	if e.Field != "" {
		return fmt.Sprintf("%s (field %q)", e.Message, e.Field)
	}
	return e.Message
}

// Unwrap exposes the sentinel condition (ErrSessionExpired,
// ErrLinkPasswordProtected) attached to this error, if any.
func (e *Error) Unwrap() error { return e.sentinel }

// IsNotFound reports whether err is an API 404 — for the resolve-first
// endpoints that means "no such link, or not yours".
func IsNotFound(err error) bool {
	var apiErr *Error
	return errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusNotFound
}

// IsRateLimited reports whether err is an API 429. The wait to observe
// is in the error's RateLimit.RetryAfter (the client has already
// retried, so a 429 surfacing here means the budget is truly gone).
func IsRateLimited(err error) bool {
	var apiErr *Error
	return errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusTooManyRequests
}

// IsBlocked reports whether err is an API 451: the link was taken down
// by the safety pipeline. Integrators should branch on this to tell
// "the link was removed" apart from "something broke".
// errors.Is(err, ErrLinkBlocked) reports the same condition.
func IsBlocked(err error) bool {
	var apiErr *Error
	return errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusUnavailableForLegalReasons
}

// newError builds an *Error from an HTTP error response, consuming (but
// not closing) the body.
func newError(resp *http.Response) *Error {
	e := &Error{StatusCode: resp.StatusCode, Message: resp.Status}
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	_ = json.Unmarshal(data, e)
	if e.Code == "" {
		e.Code = resp.Header.Get("X-Error-Code")
	}
	e.RequestID = resp.Header.Get("X-Request-ID")
	e.RateLimit = parseRateLimit(resp.Header)
	switch resp.StatusCode {
	case http.StatusUnauthorized:
		switch resp.Header.Get("X-Error-Code") {
		case "password_required", "invalid_password":
			e.sentinel = ErrLinkPasswordProtected
		}
	case http.StatusUnavailableForLegalReasons:
		e.sentinel = ErrLinkBlocked
	}
	return e
}

func parseRateLimit(h http.Header) RateLimit {
	var rl RateLimit
	rl.Limit, _ = strconv.Atoi(h.Get("X-RateLimit-Limit"))
	rl.Remaining, _ = strconv.Atoi(h.Get("X-RateLimit-Remaining"))
	if epoch, err := strconv.ParseInt(h.Get("X-RateLimit-Reset"), 10, 64); err == nil {
		rl.Reset = time.Unix(epoch, 0)
	}
	rl.RetryAfter, _ = transport.ParseRetryAfter(h.Get("Retry-After"), time.Now())
	return rl
}
