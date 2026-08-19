package spoo

import (
	"context"
	"io"
	"math/rand/v2"
	"net/http"
	"time"
)

const (
	retryBaseDelay = 500 * time.Millisecond
	retryMaxDelay  = 10 * time.Second
)

// retryableStatus reports whether a response status is worth retrying:
// timeouts, rate limits, and server-side failures.
func retryableStatus(status int) bool {
	return status == http.StatusRequestTimeout ||
		status == http.StatusTooManyRequests ||
		status >= 500
}

// retryDelay computes the wait before retry number attempt+1. A parsable
// Retry-After header is authoritative; otherwise exponential backoff
// with equal jitter (half fixed, half random) capped at retryMaxDelay.
func (c *Client) retryDelay(attempt int, retryAfter string) time.Duration {
	if d, ok := parseRetryAfter(retryAfter, time.Now()); ok {
		return d
	}
	d := min(c.retryBase<<attempt, retryMaxDelay)
	return d/2 + rand.N(d/2+1)
}

// sleep waits for d or until ctx is done, whichever comes first.
func sleep(ctx context.Context, d time.Duration) error {
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

// drain discards a response we are about to retry so the underlying
// connection can be reused.
func drain(resp *http.Response) {
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
	resp.Body.Close()
}
