package spoo

import (
	"context"
	"net/http"
)

// Claim pairs an anonymously created URL with its one-time claim token,
// the bearer proof of creation returned by the anonymous shorten call.
type Claim struct {
	URLID string `json:"url_id"`
	Token string `json:"token"`
}

// Claim result statuses. The batch never hard-fails per item — check
// each result's Status.
const (
	ClaimStatusClaimed      = "claimed"       // ownership transferred, token burned
	ClaimStatusAlreadyYours = "already_yours" // idempotent repeat
	ClaimStatusInvalid      = "invalid"       // unknown id, wrong token, or not claimable
)

// ClaimResult is one item's outcome, in request order.
type ClaimResult struct {
	URLID  string `json:"url_id"`
	Status string `json:"status"`
}

// ClaimOutcome is the whole batch's result: one row per submitted item
// plus a convenience count of claimed rows.
type ClaimOutcome struct {
	Results []ClaimResult `json:"results"`
	Claimed int           `json:"claimed"`
}

// ClaimURLs claims anonymously created URLs into the authenticated
// account, up to 16 per call. Items resolve independently and partial
// success is normal: the call errors only on request-level failures,
// per-item outcomes are data.
func (c *Client) ClaimURLs(ctx context.Context, claims []Claim) (*ClaimOutcome, error) {
	body := struct {
		Claims []Claim `json:"claims"`
	}{Claims: claims}
	var out ClaimOutcome
	if err := c.do(ctx, http.MethodPost, "/api/v1/urls/claim", nil, body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
