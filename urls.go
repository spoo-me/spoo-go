package spoo

import (
	"context"
	"encoding/json"
	"errors"
	"iter"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// URLItem is a row from GET /api/v1/urls and the shape of GET
// /api/v1/urls/{url_id}. The list envelope is camelCase (pageSize,
// hasNext) but items are snake_case; expire_after arrives as Unix
// seconds and created_at/last_click as ISO strings — all normalized to
// [Timestamp].
type URLItem struct {
	ID           string    `json:"id"`
	Alias        string    `json:"alias"`
	LongURL      string    `json:"long_url"`
	CreatedAt    Timestamp `json:"created_at"`
	LastClick    Timestamp `json:"last_click"`
	TotalClicks  int       `json:"total_clicks"`
	Status       string    `json:"status"`
	PasswordSet  bool      `json:"password_set"`
	MaxClicks    *int      `json:"max_clicks"`
	ExpireAfter  Timestamp `json:"expire_after"` // zero when the link does not expire
	PrivateStats bool      `json:"private_stats"`
	BlockBots    bool      `json:"block_bots"`
	Domain       string    `json:"domain"`
}

// URLPage is one page of the account's links. HasNext reports whether
// requesting Page+1 yields more.
type URLPage struct {
	Items     []URLItem `json:"items"`
	Page      int       `json:"page"`
	PageSize  int       `json:"pageSize"`
	Total     int       `json:"total"`
	HasNext   bool      `json:"hasNext"`
	SortBy    string    `json:"sortBy"`
	SortOrder string    `json:"sortOrder"`
}

// ListURLsOptions filters, sorts, and pages the account's links.
// Zero values defer to the server defaults.
type ListURLsOptions struct {
	Page      int
	PageSize  int
	SortBy    string // created_at | last_click | total_clicks
	SortOrder string // ascending | descending
	Search    string
	Status    string // ACTIVE | INACTIVE | BLOCKED | EXPIRED
	Domain    string
}

// ListURLs returns one page of the account's links; ListURLsAll
// iterates them all.
func (c *Client) ListURLs(ctx context.Context, opts ListURLsOptions) (*URLPage, error) {
	q := url.Values{}
	if opts.Page > 0 {
		q.Set("page", strconv.Itoa(opts.Page))
	}
	if opts.PageSize > 0 {
		q.Set("pageSize", strconv.Itoa(opts.PageSize))
	}
	if opts.SortBy != "" {
		q.Set("sortBy", opts.SortBy)
	}
	if opts.SortOrder != "" {
		q.Set("sortOrder", opts.SortOrder)
	}
	if opts.Domain != "" {
		q.Set("domain", opts.Domain)
	}
	filter := map[string]any{}
	if opts.Search != "" {
		filter["search"] = opts.Search
	}
	if opts.Status != "" {
		filter["status"] = opts.Status
	}
	if len(filter) > 0 {
		data, err := json.Marshal(filter)
		if err != nil {
			return nil, err
		}
		q.Set("filter", string(data))
	}
	var out URLPage
	if err := c.do(ctx, http.MethodGet, "/api/v1/urls", q, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ListURLsAll pages through every link matching opts, lazily fetching
// pages as the caller ranges. opts.Page picks the starting page (1 when
// zero); on a fetch error the iterator yields it once and stops:
//
//	for link, err := range client.ListURLsAll(ctx, spoo.ListURLsOptions{}) {
//		if err != nil {
//			return err
//		}
//		fmt.Println(link.Alias)
//	}
func (c *Client) ListURLsAll(ctx context.Context, opts ListURLsOptions) iter.Seq2[URLItem, error] {
	return func(yield func(URLItem, error) bool) {
		if opts.Page <= 0 {
			opts.Page = 1
		}
		for {
			page, err := c.ListURLs(ctx, opts)
			if err != nil {
				yield(URLItem{}, err)
				return
			}
			for _, item := range page.Items {
				if !yield(item, nil) {
					return
				}
			}
			if !page.HasNext {
				return
			}
			opts.Page++
		}
	}
}

// GetURL fetches one owned link by its url id. Unknown and foreign ids
// both answer 404 (no ownership oracle).
func (c *Client) GetURL(ctx context.Context, id string) (*URLItem, error) {
	var out URLItem
	if err := c.do(ctx, http.MethodGet, "/api/v1/urls/"+url.PathEscape(id), nil, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

var errDomainRequired = errors.New(`spoo: domain is required; pass "spoo.me" for links on the default domain`)

// ResolveAlias looks up an owned link by alias via GET
// /api/v1/urls/{domain}/{alias}, mainly to obtain its url id for the
// per-link stats and export endpoints. The domain names the namespace
// the alias lives in: "spoo.me" for the default namespace, or one of
// the user's custom domains. Unknown and foreign aliases both answer
// 404 (no ownership oracle).
func (c *Client) ResolveAlias(ctx context.Context, alias, domain string) (*URLItem, error) {
	if domain == "" {
		return nil, errDomainRequired
	}
	path := "/api/v1/urls/" + url.PathEscape(domain) + "/" + url.PathEscape(alias)
	var out URLItem
	if err := c.do(ctx, http.MethodGet, path, nil, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// UpdatedURL mirrors UpdateUrlResponse — unlike the shorten response it
// carries no short_url.
type UpdatedURL struct {
	ID           string    `json:"id"`
	Alias        string    `json:"alias"`
	LongURL      string    `json:"long_url"`
	Status       string    `json:"status"`
	PasswordSet  bool      `json:"password_set"`
	MaxClicks    *int      `json:"max_clicks"`
	ExpireAfter  Timestamp `json:"expire_after"`
	BlockBots    bool      `json:"block_bots"`
	PrivateStats bool      `json:"private_stats"`
	Domain       string    `json:"domain"`
	UpdatedAt    Timestamp `json:"updated_at"`
}

// UpdateURLParams patches a link. Plain fields are sent only when
// non-zero. The [Opt] fields carry the API's tri-state semantics:
// omitted keeps the current setting, [Null] clears it (remove password,
// remove click limit, remove expiry, move back to the default domain),
// [Set] replaces it. BlockBots and PrivateStats use Opt so that
// Set(false) is expressible; null keeps the existing setting there.
type UpdateURLParams struct {
	LongURL      string         `json:"long_url,omitzero"`
	Alias        string         `json:"alias,omitzero"`
	Status       string         `json:"status,omitzero"` // ACTIVE | INACTIVE
	Password     Opt[string]    `json:"password,omitzero"`
	MaxClicks    Opt[int]       `json:"max_clicks,omitzero"`
	ExpireAfter  Opt[time.Time] `json:"expire_after,omitzero"`
	Domain       Opt[string]    `json:"domain,omitzero"`
	BlockBots    Opt[bool]      `json:"block_bots,omitzero"`
	PrivateStats Opt[bool]      `json:"private_stats,omitzero"`
}

// UpdateURL patches one owned link by its url id. See
// [UpdateURLParams] for the tri-state field semantics.
func (c *Client) UpdateURL(ctx context.Context, id string, params UpdateURLParams) (*UpdatedURL, error) {
	var out UpdatedURL
	if err := c.do(ctx, http.MethodPatch, "/api/v1/urls/"+url.PathEscape(id), nil, params, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// DeleteURL permanently deletes one owned link by its url id.
func (c *Client) DeleteURL(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, "/api/v1/urls/"+url.PathEscape(id), nil, nil, nil)
}
