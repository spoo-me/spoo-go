package spoo

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"net/http"
	"net/url"
)

// Device flow (Sign in with Spoo, client half). The SDK ships the
// protocol: PKCE primitives, the authorization URL, the code exchange,
// and refresh. Driving a browser, listening on a loopback callback, and
// storing secrets stay in the app — they are platform concerns.
//
// Every consumer needs its own app registration: the backend validates
// redirect_uri by EXACT match against the app's registered URIs, so one
// app's registration cannot be borrowed by another.

// TokenPair is a device-flow JWT pair. The refresh token rotates on
// every refresh: using a pair twice kills the session.
type TokenPair struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
}

// DeviceTokens is the result of the device-code exchange: a token pair
// plus the user who granted it.
type DeviceTokens struct {
	TokenPair
	User User `json:"user"`
}

var errAppIDRequired = errors.New("spoo: app_id is required — every connected app has its own registration")

// GenerateCodeVerifier returns a PKCE code verifier: 32 random bytes
// encoded as unpadded base64url, always 43 characters (RFC 7636 §4.1).
func GenerateCodeVerifier() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// CodeChallengeS256 derives the S256 challenge for a verifier:
// BASE64URL(SHA256(verifier)) without padding (RFC 7636 §4.2).
func CodeChallengeS256(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

// GenerateState returns a random state value binding an authorization
// callback to the flow that started it (CSRF protection). The app must
// reject callbacks whose state does not match.
func GenerateState() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// DeviceAuthParams parameterizes the browser leg of the device flow.
// All fields are required.
type DeviceAuthParams struct {
	// AppID is the app's identifier in the backend's connected-apps
	// registry.
	AppID string
	// RedirectURI receives the one-time code. It must EXACTLY match a
	// redirect URI registered for the app.
	RedirectURI string
	// State from GenerateState, echoed back on the callback.
	State string
	// CodeChallenge from CodeChallengeS256; keep the verifier for
	// ExchangeDeviceCode.
	CodeChallenge string
}

// DeviceAuthURL builds the consent URL the user's browser must visit.
// The callback delivers ?code=...&state=...; trade the code with
// [Client.ExchangeDeviceCode].
func (c *Client) DeviceAuthURL(p DeviceAuthParams) string {
	q := url.Values{
		"app_id":                {p.AppID},
		"redirect_uri":          {p.RedirectURI},
		"state":                 {p.State},
		"code_challenge":        {p.CodeChallenge},
		"code_challenge_method": {"S256"},
	}
	return c.base + "/auth/device/login?" + q.Encode()
}

// ExchangeDeviceCode trades a one-time device-auth code for a JWT pair.
// The code is the credential — no prior auth is required. The verifier
// is the PKCE code verifier whose S256 challenge was sent on the login
// URL, and appID must be the registration the flow started under.
func (c *Client) ExchangeDeviceCode(ctx context.Context, appID, code, verifier string) (*DeviceTokens, error) {
	if appID == "" {
		return nil, errAppIDRequired
	}
	body := map[string]string{"app_id": appID, "code": code, "code_verifier": verifier}
	var out DeviceTokens
	if err := c.do(ctx, http.MethodPost, "/auth/device/token", nil, body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// RefreshTokens exchanges a refresh token for a new pair. The old pair
// is dead the moment this succeeds (rotation) — persist the result
// before using it. Clients with a refresh-capable [TokenSource] get
// this automatically on 401; call it directly only when driving the
// protocol yourself.
func (c *Client) RefreshTokens(ctx context.Context, appID, refreshToken string) (*TokenPair, error) {
	if appID == "" {
		return nil, errAppIDRequired
	}
	return c.deviceRefresh(ctx, appID, refreshToken)
}

// deviceRefresh posts the refresh exchange. It deliberately bypasses
// do/request: the refresh call must never recurse into the 401-refresh
// path (refreshMu is held) and needs no Authorization header — the
// refresh token in the body is the credential.
func (c *Client) deviceRefresh(ctx context.Context, appID, refreshToken string) (*TokenPair, error) {
	body := map[string]string{"refresh_token": refreshToken}
	if appID != "" {
		body["app_id"] = appID
	}
	resp, err := c.send(ctx, http.MethodPost, "/auth/device/refresh", nil, body, Credentials{})
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var out TokenPair
	if err := decode(resp, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
