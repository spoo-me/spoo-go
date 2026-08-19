package spoo

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/spoo-me/spoo-go/option"
)

func TestStatsQueryAndDecode(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("group_by") != "time,browser" || q.Get("timezone") != "UTC" {
			t.Errorf("unexpected query: %v", q)
		}
		if q.Get("start_date") != "2026-06-01T00:00:00Z" {
			t.Errorf("start_date = %q", q.Get("start_date"))
		}
		if q.Get("metrics") != "clicks,unique_clicks" {
			t.Errorf("metrics = %q", q.Get("metrics"))
		}
		if q.Has("scope") {
			t.Errorf("scope param must not be sent: %v", q)
		}
		w.Write([]byte(`{
			"scope": "all",
			"summary": {"total_clicks": 100, "unique_clicks": 60, "first_click": "2026-06-01T09:30:00Z", "avg_redirection_time": 0.12},
			"metrics": {
				"clicks_by_browser": [{"browser": "Chrome", "clicks": 70, "clicks_percentage": 70.0}],
				"clicks_by_time": [{"date": "2026-06-01", "clicks": 10}]
			},
			"computed_metrics": {"unique_click_rate": 0.6},
			"generated_at": "2026-06-02T00:00:00Z"
		}`))
	}))
	defer srv.Close()

	c := NewClient(option.WithBaseURL(srv.URL))
	res, err := c.Stats(context.Background(), StatsQuery{
		StartDate: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
		GroupBy:   []string{"time", "browser"},
		Metrics:   []string{"clicks", "unique_clicks"},
		Timezone:  "UTC",
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Summary.TotalClicks != 100 || res.Summary.UniqueClicks != 60 {
		t.Fatalf("summary = %+v", res.Summary)
	}
	if !res.Summary.FirstClick.Equal(time.Date(2026, 6, 1, 9, 30, 0, 0, time.UTC)) {
		t.Fatalf("FirstClick = %v", res.Summary.FirstClick)
	}
	points := res.Metrics["clicks_by_browser"]
	if len(points) != 1 || points[0]["browser"] != "Chrome" {
		t.Fatalf("metrics = %+v", res.Metrics)
	}
	if res.ComputedMetrics["unique_click_rate"] != 0.6 {
		t.Fatalf("computed = %+v", res.ComputedMetrics)
	}
}

func TestLinkStatsHitsPerLinkPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/stats/links/65f0abc123" {
			t.Errorf("path = %s", r.URL.Path)
		}
		if q := r.URL.Query(); q.Has("scope") || q.Has("short_code") {
			t.Errorf("legacy params must not be sent: %v", q)
		}
		w.Write([]byte(`{
			"url_id": "65f0abc123", "alias": "launch", "scope": "all",
			"summary": {"total_clicks": 42, "unique_clicks": 30},
			"metrics": {}
		}`))
	}))
	defer srv.Close()

	c := NewClient(option.WithBaseURL(srv.URL))
	res, err := c.LinkStats(context.Background(), "65f0abc123", StatsQuery{GroupBy: []string{"time"}})
	if err != nil {
		t.Fatal(err)
	}
	if res.URLID != "65f0abc123" || res.Alias != "launch" || res.Summary.TotalClicks != 42 {
		t.Fatalf("res = %+v", res)
	}
}

// StatsByAlias folds the resolve-then-fetch dance into one call.
func TestStatsByAlias(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/urls/spoo.me/launch":
			w.Write([]byte(`{"id":"65f0abc123","alias":"launch","password_set":false}`))
		case "/api/v1/stats/links/65f0abc123":
			w.Write([]byte(`{"url_id":"65f0abc123","alias":"launch","summary":{"total_clicks":42,"unique_clicks":30},"metrics":{}}`))
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	c := NewClient(option.WithBaseURL(srv.URL))
	res, err := c.StatsByAlias(context.Background(), "launch", "spoo.me", StatsQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if res.Summary.TotalClicks != 42 {
		t.Fatalf("res = %+v", res)
	}
}

func TestPublicStatsReturnsEnvelope(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/public/stats/launch" {
			t.Errorf("path = %s", r.URL.Path)
		}
		if r.Method != http.MethodGet {
			t.Errorf("method = %s, want GET without a password", r.Method)
		}
		q := r.URL.Query()
		if q.Get("start_date") != "2026-01-01T00:00:00Z" || q.Get("timezone") != "UTC" {
			t.Errorf("unexpected query: %v", q)
		}
		if q.Has("group_by") || q.Has("scope") || q.Has("password") {
			t.Errorf("public endpoint takes only a range and timezone: %v", q)
		}
		w.Write([]byte(`{
			"generation": "v2",
			"link": {"alias": "launch", "short_url": "https://spoo.me/launch", "long_url": "https://example.com/x", "status": "active", "password_protected": false, "block_bots": true},
			"stats": {
				"scope": "anon",
				"summary": {"total_clicks": 9, "unique_clicks": 5},
				"metrics": {"clicks_by_browser": [{"browser": "Chrome", "clicks": 9}]}
			}
		}`))
	}))
	defer srv.Close()

	c := NewClient(option.WithBaseURL(srv.URL))
	res, err := c.PublicStats(context.Background(), "launch", PublicStatsQuery{
		StartDate: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		Timezone:  "UTC",
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Stats.Summary.TotalClicks != 9 || len(res.Stats.Metrics["clicks_by_browser"]) != 1 {
		t.Fatalf("stats = %+v", res.Stats)
	}
	link := res.Link
	if link.Alias != "launch" || link.ShortURL != "https://spoo.me/launch" || link.Status != "active" || !link.BlockBots {
		t.Fatalf("link facts = %+v", link)
	}
	if res.Generation != "v2" {
		t.Fatalf("generation = %q", res.Generation)
	}
}

// A password rides in a POST body, never the query string (the API
// ignores query-string passwords so they cannot land in URLs or logs).
func TestPublicStatsPasswordGoesInPOSTBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST with a password", r.Method)
		}
		if r.URL.Query().Has("password") {
			t.Errorf("password leaked into the query string: %v", r.URL.Query())
		}
		if r.URL.Query().Get("timezone") != "UTC" {
			t.Errorf("query params must still ride the URL: %v", r.URL.Query())
		}
		body, _ := io.ReadAll(r.Body)
		if string(body) != `{"password":"hunter22"}` {
			t.Errorf("body = %s", body)
		}
		w.Write([]byte(`{
			"generation": "v2",
			"link": {"alias": "secret", "short_url": "https://spoo.me/secret", "status": "active", "password_protected": true, "block_bots": false},
			"stats": {"summary": {"total_clicks": 3, "unique_clicks": 2}, "metrics": {}}
		}`))
	}))
	defer srv.Close()

	c := NewClient(option.WithBaseURL(srv.URL))
	res, err := c.PublicStats(context.Background(), "secret", PublicStatsQuery{
		Timezone: "UTC",
		Password: "hunter22",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Link.PasswordProtected || res.Stats.Summary.TotalClicks != 3 {
		t.Fatalf("res = %+v", res)
	}
}

// A wrong password keeps the typed 401 semantics.
func TestPublicStatsWrongPassword(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-Error-Code", "invalid_password")
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":"Invalid password","code":"invalid_password"}`))
	}))
	defer srv.Close()

	c := NewClient(option.WithBaseURL(srv.URL))
	_, err := c.PublicStats(context.Background(), "secret", PublicStatsQuery{Password: "wrong"})
	if !errors.Is(err, ErrLinkPasswordProtected) {
		t.Fatalf("err = %v, want ErrLinkPasswordProtected", err)
	}
}

func TestExportStreamsFilenameAndBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/export" || r.URL.Query().Get("format") != "xlsx" {
			t.Errorf("unexpected request: %s %v", r.URL.Path, r.URL.Query())
		}
		if r.URL.Query().Has("scope") {
			t.Errorf("scope param must not be sent: %v", r.URL.Query())
		}
		w.Header().Set("Content-Disposition", `attachment; filename="stats-launch.xlsx"`)
		w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
		w.Write([]byte("FAKEXLSX"))
	}))
	defer srv.Close()

	c := NewClient(option.WithBaseURL(srv.URL))
	file, err := c.Export(context.Background(), StatsQuery{}, "xlsx")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Body.Close()
	data, err := io.ReadAll(file.Body)
	if err != nil {
		t.Fatal(err)
	}
	if file.Filename != "stats-launch.xlsx" || string(data) != "FAKEXLSX" {
		t.Fatalf("name=%q data=%q", file.Filename, data)
	}
	if file.ContentType != "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet" {
		t.Fatalf("ContentType = %q", file.ContentType)
	}
}

// RFC 5987 extended filenames decode; the plain parameter is the
// fallback and a bare header falls back to a synthesized name.
func TestExportFilenameParsing(t *testing.T) {
	for disposition, want := range map[string]string{
		`attachment; filename*=UTF-8''sp%C3%B6%C3%B6-export.zip`: "spöö-export.zip",
		`attachment; filename="plain.json"`:                      "plain.json",
		``:                                                       "spoo-export.zip",
	} {
		if got := exportFilename(disposition, "csv"); got != want {
			t.Errorf("exportFilename(%q) = %q, want %q", disposition, got, want)
		}
	}
	if got := exportFilename("", "json"); got != "spoo-export.json" {
		t.Errorf("json fallback = %q", got)
	}
}

// Per-link exports hit the per-link route: only that route names the
// download after the link, so aggregate exports of different links
// would silently overwrite each other on disk.
func TestExportLinkHitsPerLinkRoute(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/export/links/65f0abc123" {
			t.Errorf("path = %s, want the per-link export route", r.URL.Path)
		}
		if r.URL.Query().Has("url_id") {
			t.Errorf("url_id must not ride the query: %v", r.URL.Query())
		}
		w.Header().Set("Content-Disposition", `attachment; filename="stats-launch.zip"`)
		w.Write([]byte("FAKEZIP"))
	}))
	defer srv.Close()

	c := NewClient(option.WithBaseURL(srv.URL))
	file, err := c.ExportLink(context.Background(), "65f0abc123", StatsQuery{}, "csv")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Body.Close()
	data, _ := io.ReadAll(file.Body)
	if file.Filename != "stats-launch.zip" || string(data) != "FAKEZIP" {
		t.Fatalf("name=%q data=%q", file.Filename, data)
	}
}

// The per-link endpoints 422 on the aggregate slicing filters, so the
// SDK rejects them before any request goes out.
func TestPerLinkCallsRejectSlicingFilters(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		t.Error("no request must be sent with aggregate-only filters")
	}))
	defer srv.Close()

	c := NewClient(option.WithBaseURL(srv.URL))
	sliced := StatsQuery{Filters: map[string][]string{"short_code": {"launch"}}}
	if _, err := c.LinkStats(context.Background(), "65f0abc123", sliced); err == nil {
		t.Fatal("LinkStats must reject short_code filters")
	}
	byID := StatsQuery{Filters: map[string][]string{"url_id": {"65f0abc123"}}}
	if _, err := c.ExportLink(context.Background(), "65f0abc123", byID, "json"); err == nil {
		t.Fatal("ExportLink must reject url_id filters")
	}
}

func TestExportErrorMapsEnvelope(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error":"bad format","code":"validation_error"}`))
	}))
	defer srv.Close()

	c := NewClient(option.WithBaseURL(srv.URL))
	_, err := c.Export(context.Background(), StatsQuery{}, "bmp")
	var apiErr *Error
	if !errors.As(err, &apiErr) || apiErr.Code != "validation_error" {
		t.Fatalf("err = %v, want the parsed envelope", err)
	}
}
