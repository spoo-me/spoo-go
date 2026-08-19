// Package option configures a spoo.Client at construction:
//
//	client := spoo.NewClient(
//		option.WithAPIKey(os.Getenv("SPOO_API_KEY")),
//		option.WithMaxRetries(3),
//	)
//
// All options are optional; an empty option list yields an anonymous
// client for the public endpoints.
package option

import (
	"net/http"

	"github.com/spoo-me/spoo-go/internal/requestconfig"
)

// A RequestOption configures a spoo.Client at construction.
type RequestOption = requestconfig.RequestOption

// WithAPIKey authenticates every call with a spoo_... API key.
// Shorthand for WithTokenSource(spoo.StaticAPIKey(key)); an empty key
// leaves the client anonymous.
func WithAPIKey(key string) RequestOption {
	return func(c *requestconfig.Config) {
		if key != "" {
			c.Tokens = requestconfig.StaticAPIKey(key)
		}
	}
}

// WithTokenSource supplies credentials through a spoo.TokenSource: use
// spoo.StaticTokens for a device-flow JWT pair, or your own
// implementation to persist rotations.
func WithTokenSource(tokens requestconfig.TokenSource) RequestOption {
	return func(c *requestconfig.Config) { c.Tokens = tokens }
}

// WithBaseURL points the client at a different deployment; self-hosted
// instances are first-class. The default is https://spoo.me.
func WithBaseURL(base string) RequestOption {
	return func(c *requestconfig.Config) { c.BaseURL = base }
}

// WithHTTPClient replaces the underlying *http.Client, the seam for
// custom transports, proxies, and mocks. The client is copied and its
// redirect policy extended so the X-Spoo-Client header still never
// leaks across hosts.
func WithHTTPClient(hc *http.Client) RequestOption {
	return func(c *requestconfig.Config) { c.HTTPClient = hc }
}

// WithMaxRetries sets how many times a failed request is retried
// (connection errors, 408, 429, 5xx). The default is 2; 0 disables
// retries.
func WithMaxRetries(n int) RequestOption {
	return func(c *requestconfig.Config) { c.MaxRetries = max(n, 0) }
}

// WithClientTag sets the X-Spoo-Client attribution header, e.g.
// "cli/1.4.0" for an app built on the SDK. The default is
// sdk-go/<version>.
func WithClientTag(tag string) RequestOption {
	return func(c *requestconfig.Config) { c.ClientTag = tag }
}
