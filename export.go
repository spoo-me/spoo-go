package spoo

import (
	"context"
	"io"
	"mime"
	"net/http"
	"net/url"
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

// exportFilename extracts the filename from a Content-Disposition
// header. mime.ParseMediaType decodes RFC 2231/5987 extended params, so
// filename* surfaces under the "filename" key already decoded.
func exportFilename(disposition, format string) string {
	if _, params, err := mime.ParseMediaType(disposition); err == nil {
		if name := params["filename"]; name != "" {
			return name
		}
	}
	if format == "csv" {
		return "spoo-export.zip" // csv arrives zipped, one CSV per dimension
	}
	return "spoo-export." + format
}
