// Package requestconfig holds the client configuration state that the
// root spoo package and the option package share. Options mutate a
// Config here instead of the client directly, which is what lets
// option avoid importing the root package (and the cycle that would
// create).
package requestconfig

import (
	"context"
	"net/http"
	"sync"
)

// Credentials is one authentication state: an API key, or a JWT pair
// from the connected-apps device flow. The zero value means anonymous,
// a legitimate mode for the public endpoints.
type Credentials struct {
	APIKey       string
	AccessToken  string
	RefreshToken string
}

// Bearer returns the value for the Authorization header, or "" when
// anonymous. An API key wins over a device-flow pair.
func (c Credentials) Bearer() string {
	if c.APIKey != "" {
		return c.APIKey
	}
	return c.AccessToken
}

// TokenSource supplies credentials for API calls and persists
// rotations.
//
// Token is called before every request. Update is called after a
// successful token refresh: the backend rotates refresh tokens, so the
// old pair is dead the moment Update runs. Implementations that back a
// long-lived login (keyring, file, database) must persist the new pair.
type TokenSource interface {
	Token(ctx context.Context) (Credentials, error)
	Update(ctx context.Context, creds Credentials) error
}

// StaticAPIKey returns a TokenSource for a fixed spoo_... API key.
// Update is a no-op: API keys never rotate client-side.
func StaticAPIKey(key string) TokenSource {
	return staticAPIKey(key)
}

type staticAPIKey string

func (k staticAPIKey) Token(context.Context) (Credentials, error) {
	return Credentials{APIKey: string(k)}, nil
}

func (staticAPIKey) Update(context.Context, Credentials) error { return nil }

// StaticTokens returns a TokenSource seeded with a device-flow JWT
// pair. Rotations are kept in memory only: when the process exits the
// rotated pair is lost and the seed pair is already dead, so use this
// for short-lived processes and implement TokenSource yourself for
// anything that must survive restarts.
func StaticTokens(access, refresh string) TokenSource {
	return &staticTokens{creds: Credentials{AccessToken: access, RefreshToken: refresh}}
}

type staticTokens struct {
	mu    sync.Mutex
	creds Credentials
}

func (s *staticTokens) Token(context.Context) (Credentials, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.creds, nil
}

func (s *staticTokens) Update(_ context.Context, creds Credentials) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.creds = creds
	return nil
}

// Config is the state a client is constructed from. The root package
// seeds the defaults, RequestOptions mutate it, and the client copies
// what it needs.
type Config struct {
	// BaseURL is the deployment the client talks to.
	BaseURL string
	// HTTPClient is the caller-supplied *http.Client, nil for the
	// default.
	HTTPClient *http.Client
	// Tokens supplies credentials; nil means anonymous.
	Tokens TokenSource
	// MaxRetries caps transient-failure retries.
	MaxRetries int
	// ClientTag is the X-Spoo-Client attribution value; empty defers
	// to the SDK default.
	ClientTag string
}

// RequestOption mutates the Config a client is constructed from.
type RequestOption func(*Config)
