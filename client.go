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
	"strings"
	"sync"
	"time"

	"github.com/spoo-me/spoo-go/internal/requestconfig"
	"github.com/spoo-me/spoo-go/internal/transport"
	"github.com/spoo-me/spoo-go/option"
)

// DefaultBaseURL is the hosted spoo.me API; see option.WithBaseURL for
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

	// emojiMu guards the ETag-validated emoji-set cache.
	emojiMu    sync.Mutex
	emojiETag  string
	emojiCache *EmojiSet
}

// NewClient returns a Client for the hosted spoo.me API, anonymous
// unless an auth option is given. See the option package for
// configuration.
func NewClient(opts ...option.RequestOption) *Client {
	cfg := requestconfig.Config{
		BaseURL:    DefaultBaseURL,
		MaxRetries: defaultMaxRetries,
	}
	for _, opt := range opts {
		opt(&cfg)
	}
	c := &Client{
		base:       strings.TrimRight(cfg.BaseURL, "/"),
		tokens:     cfg.Tokens,
		maxRetries: max(cfg.MaxRetries, 0),
		clientTag:  cfg.ClientTag,
		retryBase:  transport.RetryBaseDelay,
	}
	if c.clientTag == "" {
		c.clientTag = defaultClientTag()
	}
	hc := cfg.HTTPClient
	if hc == nil {
		hc = &http.Client{Timeout: 30 * time.Second}
	}
	c.http = transport.WithRedirectHeaderStrip(hc)
	return c
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
	resp, err := c.send(ctx, method, path, query, body, creds, nil)
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
			_ = resp.Body.Close()
			if creds, err = c.refreshCredentials(ctx, creds); err != nil {
				return nil, err
			}
			if resp, err = c.send(ctx, method, path, query, body, creds, nil); err != nil {
				return nil, err
			}
		}
	}
	return resp, nil
}

// send builds and performs one HTTP call, retrying transient failures
// (connection errors, 408, 429, 5xx) with exponential backoff and
// jitter, honoring Retry-After. Callers own the response body.
func (c *Client) send(ctx context.Context, method, path string, query url.Values, body any, creds Credentials, extra http.Header) (*http.Response, error) {
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
		resp, err := c.sendOnce(ctx, method, u, payload, creds, extra)
		if err != nil {
			if attempt >= c.maxRetries || ctx.Err() != nil {
				return nil, err
			}
		} else if !transport.RetryableStatus(resp.StatusCode) || attempt >= c.maxRetries {
			return resp, nil
		}
		var retryAfter string
		if resp != nil {
			retryAfter = resp.Header.Get("Retry-After")
			transport.Drain(resp)
		}
		if err := transport.Sleep(ctx, transport.RetryDelay(c.retryBase, attempt, retryAfter)); err != nil {
			return nil, err
		}
	}
}

func (c *Client) sendOnce(ctx context.Context, method, u string, payload []byte, creds Credentials, extra http.Header) (*http.Response, error) {
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
	if bearer := creds.Bearer(); bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	for name, values := range extra {
		req.Header[name] = values
	}
	return c.http.Do(req)
}

// ForceRefresh invalidates the current access token and refreshes the
// device-flow pair immediately, persisting the rotation through the
// TokenSource. It shares the client's single-flight guarantee, so
// concurrent callers trigger at most one exchange. It fails with
// ErrTokenSourceRequired when the client has no refresh-capable source.
func (c *Client) ForceRefresh(ctx context.Context) (Credentials, error) {
	creds, err := c.credentials(ctx)
	if err != nil {
		return Credentials{}, err
	}
	if creds.RefreshToken == "" {
		return Credentials{}, ErrTokenSourceRequired
	}
	return c.refreshCredentials(ctx, creds)
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
