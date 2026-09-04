package spoo

import (
	"context"
	"net/http"
	"net/url"
)

// Tag is an account tag as the tag endpoints return it, with the number
// of links carrying it. Color is a palette key such as "violet" or
// "teal" and Icon a lucide icon key such as "rocket" ("tag" by
// default); the API docs list the current sets.
type Tag struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Color     string    `json:"color"`
	Icon      string    `json:"icon"`
	LinkCount int       `json:"link_count"`
	CreatedAt Timestamp `json:"created_at"`
	UpdatedAt Timestamp `json:"updated_at"` // zero until the tag is first edited
}

// TagRef is a tag as it appears on a link: enough to render, no counts.
type TagRef struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Color string `json:"color"`
	Icon  string `json:"icon"`
}

// CreateTagParams creates a tag. Only Name is required; it is
// lowercased and trimmed server-side and must be unique per account.
// An empty Color gets the least-used palette color in the account and
// an empty Icon the generic tag glyph.
type CreateTagParams struct {
	Name  string `json:"name"`
	Color string `json:"color,omitempty"`
	Icon  string `json:"icon,omitempty"`
}

// UpdateTagParams patches a tag. Empty fields are omitted and keep
// their current value; links reference tags by id, so a rename shows
// up on every link at once.
type UpdateTagParams struct {
	Name  string `json:"name,omitempty"`
	Color string `json:"color,omitempty"`
	Icon  string `json:"icon,omitempty"`
}

// TagDeletion reports a tag delete: the tag is gone and LinksUpdated
// links had it removed.
type TagDeletion struct {
	Deleted      bool `json:"deleted"`
	LinksUpdated int  `json:"links_updated"`
}

// ListTags returns every tag in the account, oldest first, with link
// counts.
func (c *Client) ListTags(ctx context.Context) ([]Tag, error) {
	var out struct {
		Items []Tag `json:"items"`
	}
	if err := c.do(ctx, http.MethodGet, "/api/v1/tags", nil, nil, &out); err != nil {
		return nil, err
	}
	return out.Items, nil
}

// CreateTag creates a tag. A name the account already has answers 409
// conflict; accounts hold at most 500 tags.
func (c *Client) CreateTag(ctx context.Context, params CreateTagParams) (*Tag, error) {
	var out Tag
	if err := c.do(ctx, http.MethodPost, "/api/v1/tags", nil, params, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// UpdateTag patches one tag by id; see [UpdateTagParams]. Renaming
// onto a name the account already has answers 409 conflict.
func (c *Client) UpdateTag(ctx context.Context, id string, params UpdateTagParams) (*Tag, error) {
	var out Tag
	if err := c.do(ctx, http.MethodPatch, "/api/v1/tags/"+url.PathEscape(id), nil, params, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// DeleteTag deletes one tag by id and removes it from every link that
// carried it; the links are otherwise untouched.
func (c *Client) DeleteTag(ctx context.Context, id string) (*TagDeletion, error) {
	var out TagDeletion
	if err := c.do(ctx, http.MethodDelete, "/api/v1/tags/"+url.PathEscape(id), nil, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
