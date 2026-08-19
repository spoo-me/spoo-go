package spoo

import (
	"context"
	"net/http"
	"net/url"
	"time"
)

// ShortenRequest creates a short link. Only LongURL is required;
// zero-valued optionals are omitted from the request.
type ShortenRequest struct {
	LongURL      string    `json:"long_url"`
	Alias        string    `json:"alias,omitempty"`
	Password     string    `json:"password,omitempty"`
	BlockBots    bool      `json:"block_bots,omitempty"`
	MaxClicks    int       `json:"max_clicks,omitempty"`
	ExpireAfter  time.Time `json:"expire_after,omitzero"`
	PrivateStats bool      `json:"private_stats,omitempty"`
	Domain       string    `json:"domain,omitempty"`
}

// ShortURL mirrors UrlResponse (POST /api/v1/shorten).
type ShortURL struct {
	// ID is the identifier the management endpoints address the link by.
	ID       string `json:"id"`
	ShortURL string `json:"short_url"`
	Alias    string `json:"alias"`
	LongURL  string `json:"long_url"`
	// OwnerID is empty for anonymous creations.
	OwnerID   string    `json:"owner_id"`
	CreatedAt Timestamp `json:"created_at"`
	Status    string    `json:"status"`
	// ClaimToken is present only on anonymous creations: the one-time
	// bearer proof of creation. Store it and the link can be claimed
	// into an account later with [Client.ClaimURLs].
	ClaimToken string `json:"claim_token"`
}

// Shorten creates a short link. Anonymous calls work and return a
// one-time ClaimToken; authenticated calls create owned links. An
// empty LongURL fails with [ErrMissingLongURL] before any request
// goes out.
func (c *Client) Shorten(ctx context.Context, req ShortenRequest) (*ShortURL, error) {
	if req.LongURL == "" {
		return nil, ErrMissingLongURL
	}
	var out ShortURL
	if err := c.do(ctx, http.MethodPost, "/api/v1/shorten", nil, req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// AliasCheck reports whether an alias is free to use, with the
// rejection reason when it is not (length, format, reserved, taken,
// or emoji_policy).
type AliasCheck struct {
	Available bool   `json:"available"`
	Reason    string `json:"reason"`
}

// CheckAlias reports whether alias is available, on the given domain
// when one is passed.
func (c *Client) CheckAlias(ctx context.Context, alias, domain string) (*AliasCheck, error) {
	q := url.Values{"alias": {alias}}
	if domain != "" {
		q.Set("domain", domain)
	}
	var out AliasCheck
	if err := c.do(ctx, http.MethodGet, "/api/v1/shorten/check-alias", q, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
