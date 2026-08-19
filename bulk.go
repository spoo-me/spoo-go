package spoo

import (
	"context"
	"net/http"
	"time"
)

// BulkSummary counts a bulk operation's outcomes after id dedupe.
type BulkSummary struct {
	Total     int `json:"total"`
	Succeeded int `json:"succeeded"`
	Failed    int `json:"failed"`
}

// BulkRow is one id's outcome, in request order.
type BulkRow struct {
	ID string `json:"id"`
	// Alias is echoed when the id resolved to a URL the caller owns.
	Alias string `json:"alias"`
	OK    bool   `json:"ok"`
	// ErrorCode is set when OK is false: not_found, forbidden,
	// conflict, validation_error, internal, or not_attempted.
	ErrorCode string `json:"error_code"`
	// Error is the human-readable failure message.
	Error string `json:"error"`
}

// BulkResult is a bulk operation's outcome. Bulk endpoints answer
// HTTP 200 even when every item fails — per-item outcomes are data,
// not an error. Check Summary.Failed and the per-row ErrorCode.
type BulkResult struct {
	Summary BulkSummary `json:"summary"`
	Results []BulkRow   `json:"results"`
}

// BulkDelete deletes up to 100 owned links by url id. Duplicates are
// deduplicated server-side.
func (c *Client) BulkDelete(ctx context.Context, ids []string) (*BulkResult, error) {
	body := struct {
		IDs []string `json:"ids"`
	}{IDs: ids}
	return c.bulk(ctx, "/api/v1/urls/bulk/delete", body)
}

// BulkUpdateStatus applies status (ACTIVE or INACTIVE) to up to 100
// owned links by url id.
func (c *Client) BulkUpdateStatus(ctx context.Context, ids []string, status string) (*BulkResult, error) {
	body := struct {
		IDs    []string `json:"ids"`
		Status string   `json:"status"`
	}{IDs: ids, Status: status}
	return c.bulk(ctx, "/api/v1/urls/bulk/status", body)
}

// BulkUpdateExpiry applies an expiration to up to 100 owned links by
// url id. The time must be in the future; the zero time clears expiry
// (JSON null on the wire).
func (c *Client) BulkUpdateExpiry(ctx context.Context, ids []string, expireAfter time.Time) (*BulkResult, error) {
	body := struct {
		IDs         []string  `json:"ids"`
		ExpireAfter Timestamp `json:"expire_after"`
	}{IDs: ids, ExpireAfter: Timestamp{expireAfter}}
	return c.bulk(ctx, "/api/v1/urls/bulk/expiry", body)
}

// BulkMoveDomain moves up to 100 owned links by url id to a domain
// namespace: an owned ACTIVE custom-domain fqdn, or "" to move back to
// the system default (JSON null on the wire).
func (c *Client) BulkMoveDomain(ctx context.Context, ids []string, domain string) (*BulkResult, error) {
	var target *string
	if domain != "" {
		target = &domain
	}
	body := struct {
		IDs    []string `json:"ids"`
		Domain *string  `json:"domain"`
	}{IDs: ids, Domain: target}
	return c.bulk(ctx, "/api/v1/urls/bulk/domain", body)
}

func (c *Client) bulk(ctx context.Context, path string, body any) (*BulkResult, error) {
	var out BulkResult
	if err := c.do(ctx, http.MethodPost, path, nil, body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
