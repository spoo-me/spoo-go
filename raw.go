package spoo

import (
	"context"
	"net/http"
	"net/url"
)

// The raw request methods below are the supported pressure valve for
// API endpoints the SDK has no typed method for yet. They speak
// through the configured client, so auth, token refresh, retries,
// timeout, the attribution tag and *Error mapping all apply; only the
// request and response types are the caller's. path is relative to the
// client's base URL and must start with "/", e.g. "/api/v1/urls".
// Needing one of these means the SDK surface has a gap worth an issue
// at https://github.com/spoo-me/spoo-go/issues.

// Get performs a raw GET request against an API path and decodes the
// JSON response into out (skipped when out is nil). It is the
// supported pressure valve for endpoints the SDK does not cover yet;
// needing it is worth an issue.
func (c *Client) Get(ctx context.Context, path string, query url.Values, out any) error {
	return c.do(ctx, http.MethodGet, path, query, nil, out)
}

// Post performs a raw POST request against an API path, marshaling
// body to JSON (no body when nil) and decoding the JSON response into
// out (skipped when out is nil). It is the supported pressure valve
// for endpoints the SDK does not cover yet; needing it is worth an
// issue.
func (c *Client) Post(ctx context.Context, path string, body, out any) error {
	return c.do(ctx, http.MethodPost, path, nil, body, out)
}

// Put performs a raw PUT request against an API path, marshaling body
// to JSON (no body when nil) and decoding the JSON response into out
// (skipped when out is nil). It is the supported pressure valve for
// endpoints the SDK does not cover yet; needing it is worth an issue.
func (c *Client) Put(ctx context.Context, path string, body, out any) error {
	return c.do(ctx, http.MethodPut, path, nil, body, out)
}

// Patch performs a raw PATCH request against an API path, marshaling
// body to JSON (no body when nil) and decoding the JSON response into
// out (skipped when out is nil). It is the supported pressure valve
// for endpoints the SDK does not cover yet; needing it is worth an
// issue.
func (c *Client) Patch(ctx context.Context, path string, body, out any) error {
	return c.do(ctx, http.MethodPatch, path, nil, body, out)
}

// Delete performs a raw DELETE request against an API path and decodes
// the JSON response into out (skipped when out is nil). It is the
// supported pressure valve for endpoints the SDK does not cover yet;
// needing it is worth an issue.
func (c *Client) Delete(ctx context.Context, path string, out any) error {
	return c.do(ctx, http.MethodDelete, path, nil, nil, out)
}
