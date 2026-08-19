package spoo

import (
	"context"
	"net/http"
)

// TokenPair is a device-flow JWT pair. The refresh token rotates on
// every refresh: using a pair twice kills the session.
type TokenPair struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
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
