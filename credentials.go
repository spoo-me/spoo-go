package spoo

import "github.com/spoo-me/spoo-go/internal/requestconfig"

// Credentials is one authentication state: an API key, or a JWT pair
// from the connected-apps device flow. The zero value means anonymous,
// a legitimate mode for the public endpoints.
type Credentials = requestconfig.Credentials

// TokenSource supplies credentials for API calls and persists
// rotations.
//
// Token is called before every request. Update is called after a
// successful token refresh: the backend rotates refresh tokens, so the
// old pair is dead the moment Update runs. Implementations that back a
// long-lived login (keyring, file, database) must persist the new pair.
type TokenSource = requestconfig.TokenSource

// StaticAPIKey returns a TokenSource for a fixed spoo_... API key.
// Update is a no-op: API keys never rotate client-side.
func StaticAPIKey(key string) TokenSource {
	return requestconfig.StaticAPIKey(key)
}

// StaticTokens returns a TokenSource seeded with a device-flow JWT
// pair. Rotations are kept in memory only: when the process exits the
// rotated pair is lost and the seed pair is already dead, so use this
// for short-lived processes and implement TokenSource yourself for
// anything that must survive restarts.
func StaticTokens(access, refresh string) TokenSource {
	return requestconfig.StaticTokens(access, refresh)
}
