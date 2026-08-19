package spoo

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sync"
	"time"
)

// DefaultBaseURL is the hosted spoo.me API; see [WithBaseURL] for
// self-hosted deployments.
const DefaultBaseURL = "https://spoo.me"

const defaultMaxRetries = 2

// Client is a spoo.me API client. Construct it with [NewClient]; the
// zero value is not usable. A Client is safe for concurrent use.
type Client struct {
	base       string
	http       *http.Client
	tokens     TokenSource
	maxRetries int
	clientTag  string

	// retryBase is the exponential-backoff unit, shrunk in tests.
	retryBase time.Duration

	// refreshMu single-flights token refresh: under concurrent 401s
	// only one goroutine may spend the rotating refresh token — a
	// second spender would persist a dead pair.
	refreshMu sync.Mutex
}

// NewClient returns a Client for the hosted spoo.me API, anonymous
// unless an auth option is given. See [Option] for configuration.
func NewClient(opts ...Option) *Client {
	c := &Client{
		base:       DefaultBaseURL,
		maxRetries: defaultMaxRetries,
		retryBase:  retryBaseDelay,
	}
	for _, opt := range opts {
		opt(c)
	}
	if c.clientTag == "" {
		c.clientTag = defaultClientTag()
	}
	if c.http == nil {
		c.http = &http.Client{Timeout: 30 * time.Second}
	}
	c.http = withRedirectHeaderStrip(c.http)
	return c
}

// withRedirectHeaderStrip copies hc and extends its redirect policy: Go
// forwards custom headers on redirects, including cross-origin ones.
// Attribution belongs to the spoo API only, so X-Spoo-Client is dropped
// whenever a redirect leaves the original host. Go itself strips
// Authorization on cross-domain hops.
func withRedirectHeaderStrip(hc *http.Client) *http.Client {
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

// credentials returns the current credentials, or the anonymous zero
// value when no TokenSource is configured.
func (c *Client) credentials(ctx context.Context) (Credentials, error) {
	if c.tokens == nil {
		return Credentials{}, nil
	}
	return c.tokens.Token(ctx)
}

func (c *Client) do(ctx context.Context, method, path string, query url.Values, body, out any) error {
	resp, err := c.request(ctx, method, path, query, body)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return decode(resp, out)
}

// request performs an authenticated call and returns the raw response,
// refreshing device tokens once on 401. Callers own the response body.
func (c *Client) request(ctx context.Context, method, path string, query url.Values, body any) (*http.Response, error) {
	creds, err := c.credentials(ctx)
	if err != nil {
		return nil, err
	}
	resp, err := c.send(ctx, method, path, query, body, creds)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusUnauthorized {
		// The public stats endpoint answers 401 for password-protected
		// links. That is a property of the link, not of the session, so
		// refreshing tokens can't help — newError attaches
		// ErrLinkPasswordProtected instead of blaming the login.
		switch resp.Header.Get("X-Error-Code") {
		case "password_required", "invalid_password":
			defer resp.Body.Close()
			return nil, newError(resp)
		}
		if creds.RefreshToken != "" {
			resp.Body.Close()
			if creds, err = c.refreshCredentials(ctx, creds); err != nil {
				return nil, err
			}
			if resp, err = c.send(ctx, method, path, query, body, creds); err != nil {
				return nil, err
			}
		}
	}
	return resp, nil
}

// send builds and performs one HTTP call, retrying transient failures
// (connection errors, 408, 429, 5xx) with exponential backoff and
// jitter, honoring Retry-After. Callers own the response body.
func (c *Client) send(ctx context.Context, method, path string, query url.Values, body any, creds Credentials) (*http.Response, error) {
	u := c.base + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	var payload []byte
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		payload = data
	}
	for attempt := 0; ; attempt++ {
		resp, err := c.sendOnce(ctx, method, u, payload, creds)
		if err != nil {
			if attempt >= c.maxRetries || ctx.Err() != nil {
				return nil, err
			}
		} else if !retryableStatus(resp.StatusCode) || attempt >= c.maxRetries {
			return resp, nil
		}
		var retryAfter string
		if resp != nil {
			retryAfter = resp.Header.Get("Retry-After")
			drain(resp)
		}
		if err := sleep(ctx, c.retryDelay(attempt, retryAfter)); err != nil {
			return nil, err
		}
	}
}

func (c *Client) sendOnce(ctx context.Context, method, u string, payload []byte, creds Credentials) (*http.Response, error) {
	var rdr io.Reader
	if payload != nil {
		rdr = bytes.NewReader(payload)
	}
	req, err := http.NewRequestWithContext(ctx, method, u, rdr)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "spoo-go")
	req.Header.Set("X-Spoo-Client", c.clientTag)
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if bearer := creds.bearer(); bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	return c.http.Do(req)
}

// refreshCredentials exchanges the refresh token for a new pair and
// persists it via TokenSource.Update. The backend rotates refresh
// tokens, so the stored pair must be replaced — and only one goroutine
// may perform the exchange (see refreshMu).
func (c *Client) refreshCredentials(ctx context.Context, stale Credentials) (Credentials, error) {
	c.refreshMu.Lock()
	defer c.refreshMu.Unlock()

	// Another goroutine may have refreshed while we waited on the
	// lock; if the source already holds a different access token, use
	// it instead of spending the (now rotated-dead) refresh token.
	current, err := c.credentials(ctx)
	if err != nil {
		return Credentials{}, err
	}
	if current.AccessToken != stale.AccessToken {
		return current, nil
	}

	pair, err := c.deviceRefresh(ctx, "", current.RefreshToken)
	if err != nil {
		// A definitive rejection of the refresh token means the
		// session is gone; transient failures (5xx, 429, network) are
		// not a verdict on the session.
		var apiErr *Error
		if errors.As(err, &apiErr) && apiErr.sentinel == nil {
			switch apiErr.StatusCode {
			case http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden:
				apiErr.sentinel = ErrSessionExpired
			}
		}
		return Credentials{}, fmt.Errorf("refreshing session: %w", err)
	}
	updated := current
	updated.AccessToken = pair.AccessToken
	updated.RefreshToken = pair.RefreshToken
	if c.tokens != nil {
		if err := c.tokens.Update(ctx, updated); err != nil {
			return Credentials{}, err
		}
	}
	return updated, nil
}

func decode(resp *http.Response, out any) error {
	if resp.StatusCode >= 400 {
		return newError(resp)
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}
