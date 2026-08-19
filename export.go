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
	// (RFC 5987 filename* preferred, plain filename fallback), or a
	// spoo-export.<ext> default when the header is absent.
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
// [StatsQuery].
func (c *Client) Export(ctx context.Context, q StatsQuery, format string) (*ExportFile, error) {
	return c.export(ctx, q.values(), format)
}

// ExportLink downloads one owned link's stats by its url id (resolve
// an alias with ResolveAlias first). Unknown and foreign ids yield an
// empty slice of that link, consistent with the slicing filters.
func (c *Client) ExportLink(ctx context.Context, urlID string, q StatsQuery, format string) (*ExportFile, error) {
	v := q.values()
	v.Set("url_id", urlID)
	return c.export(ctx, v, format)
}

func (c *Client) export(ctx context.Context, v url.Values, format string) (*ExportFile, error) {
	v.Set("format", format)
	resp, err := c.request(ctx, http.MethodGet, "/api/v1/export", v, nil)
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
// Content-Disposition filename when present, else a synthesized
// spoo-export name.
func exportFilename(disposition, format string) string {
	if name, ok := transport.ContentDispositionFilename(disposition); ok {
		return name
	}
	if format == "csv" {
		return "spoo-export.zip" // csv arrives zipped, one CSV per dimension
	}
	return "spoo-export." + format
}
