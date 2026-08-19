package spoo

import (
	"context"
	"net/http"
)

// User is the account behind the current credentials.
type User struct {
	ID            string `json:"id"`
	Email         string `json:"email"`
	EmailVerified bool   `json:"email_verified"`
	Name          string `json:"name"`
	Plan          string `json:"plan"`
}

// Me returns the authenticated user.
func (c *Client) Me(ctx context.Context) (*User, error) {
	var out struct {
		User User `json:"user"`
	}
	if err := c.do(ctx, http.MethodGet, "/auth/me", nil, nil, &out); err != nil {
		return nil, err
	}
	return &out.User, nil
}
