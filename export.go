package spoo

import (
	"context"
	"io"
	"net/http"
	"net/url"

	"github.com/spoo-me/spoo-go/internal/transport"
)

// ExportFile is a streamed export download. The caller owns Body and
// must Close it.
type ExportFile struct {
	// Filename is the server-suggested name from Content-Disposition
	// (RFC 5987 filename* preferred, plain filename fallback), reduced
	// to a bare filename so it is safe to hand to os.Create, or a
	// spoo-export.<ext> default when the header is absent or its name
	// is path-shaped.
	Filename string
	// ContentType is the response media type.
	ContentType string
	// Body streams the file contents. csv exports arrive as a ZIP
	// archive (one CSV per dimension).
	Body io.ReadCloser
}

// Export downloads account-wide stats in the given format (json, csv,
// xlsx, xml). Auth is required — anonymous export no longer exists.
// Slice to specific links with the short_code / url_id filters on
// [StatsQuery]; note the aggregate route reports a generic filename
// regardless of slicing, so single-link exports belong on ExportLink.
func (c *Client) Export(ctx context.Context, q StatsQuery, format string) (*ExportFile, error) {
	return c.export(ctx, "/api/v1/export", q.values(), format)
}

// ExportLink downloads one owned link's stats by its url id via the
// per-link route, whose server-suggested filename carries the link's
// identity (the aggregate route names every download the same, so
// saved files would silently overwrite each other). Resolve an alias
// with ResolveAlias first; unknown and foreign ids both 404. The
// short_code / url_id slicing filters are aggregate-only here too.
func (c *Client) ExportLink(ctx context.Context, urlID string, q StatsQuery, format string) (*ExportFile, error) {
	if err := q.validatePerLink(); err != nil {
		return nil, err
	}
	path := "/api/v1/export/links/" + url.PathEscape(urlID)
	return c.export(ctx, path, q.values(), format)
}

func (c *Client) export(ctx context.Context, path string, v url.Values, format string) (*ExportFile, error) {
	v.Set("format", format)
	resp, err := c.request(ctx, http.MethodGet, path, v, nil)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		defer resp.Body.Close()
		return nil, newError(resp)
	}
	return &ExportFile{
		Filename:    exportFilename(resp.Header.Get("Content-Disposition"), format),
		ContentType: resp.Header.Get("Content-Type"),
		Body:        resp.Body,
	}, nil
}

// exportFilename resolves the download name: the server-suggested
// Content-Disposition filename when present and sanitized to a bare
// name, else a synthesized spoo-export name.
func exportFilename(disposition, format string) string {
	if name, ok := transport.ContentDispositionFilename(disposition); ok {
		return name
	}
	if format == "csv" {
		return "spoo-export.zip" // csv arrives zipped, one CSV per dimension
	}
	return "spoo-export." + format
}
