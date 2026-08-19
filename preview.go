package spoo

import (
	"context"
	"net/http"
	"net/url"
)

// PreviewDestination describes where a short link points, decomposed
// for display.
type PreviewDestination struct {
	URL     string `json:"url"`
	Domain  string `json:"domain"`
	Path    string `json:"path"`
	IsHTTPS bool   `json:"is_https"`
}

// PreviewGeoDestination is a per-country destination override.
type PreviewGeoDestination struct {
	PreviewDestination
	Countries []string `json:"countries"`
}

// PublicPreview is the anonymous link-preview payload: enough to show
// what a short link is before following it.
type PublicPreview struct {
	Generation string    `json:"generation"` // v1 | v2
	Alias      string    `json:"alias"`
	ShortURL   string    `json:"short_url"`
	Status     string    `json:"status"` // active | inactive | expired | blocked (lowercase, unlike the management surface)
	CreatedAt  Timestamp `json:"created_at"`
	// PasswordProtected reports redirect protection; the destination
	// is still previewable.
	PasswordProtected bool `json:"password_protected"`
	// Destination is nil when the link is not active.
	Destination     *PreviewDestination     `json:"destination"`
	GeoDestinations []PreviewGeoDestination `json:"geo_destinations"`
}

// PublicPreview returns anyone's link preview without auth.
func (c *Client) PublicPreview(ctx context.Context, shortCode string) (*PublicPreview, error) {
	var out PublicPreview
	path := "/api/v1/public/preview/" + url.PathEscape(shortCode)
	if err := c.do(ctx, http.MethodGet, path, nil, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
