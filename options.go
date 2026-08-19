package spoo

import (
	"net/http"
	"strings"
)

// An Option configures a [Client] at construction.
type Option func(*Client)

// WithAPIKey authenticates every call with a spoo_... API key.
// Shorthand for WithTokenSource(StaticAPIKey(key)); an empty key leaves
// the client anonymous.
func WithAPIKey(key string) Option {
	return func(c *Client) {
		if key != "" {
			c.tokens = StaticAPIKey(key)
		}
	}
}

// WithTokenSource supplies credentials through a [TokenSource] — use
// [StaticTokens] for a device-flow JWT pair, or your own implementation
// to persist rotations.
func WithTokenSource(tokens TokenSource) Option {
	return func(c *Client) { c.tokens = tokens }
}

// WithBaseURL points the client at a different deployment; self-hosted
// instances are first-class. The default is https://spoo.me.
func WithBaseURL(base string) Option {
	return func(c *Client) { c.base = strings.TrimRight(base, "/") }
}

// WithHTTPClient replaces the underlying *http.Client — the seam for
// custom transports, proxies, and mocks. The client is copied and its
// redirect policy extended so the X-Spoo-Client header still never
// leaks across hosts.
func WithHTTPClient(hc *http.Client) Option {
	return func(c *Client) { c.http = hc }
}

// WithMaxRetries sets how many times a failed request is retried
// (connection errors, 408, 429, 5xx). The default is 2; 0 disables
// retries.
func WithMaxRetries(n int) Option {
	return func(c *Client) { c.maxRetries = max(n, 0) }
}

// WithClientTag sets the X-Spoo-Client attribution header, e.g.
// "cli/1.4.0" for an app built on the SDK. The default is
// sdk-go/<version>.
func WithClientTag(tag string) Option {
	return func(c *Client) { c.clientTag = tag }
}
