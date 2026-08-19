package spoo

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
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

	c := NewClient(WithBaseURL(srv.URL))
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

	c := NewClient(WithBaseURL(srv.URL))
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

	c := NewClient(WithBaseURL(srv.URL))
	res, err := c.StatsByAlias(context.Background(), "launch", "spoo.me", StatsQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if res.Summary.TotalClicks != 42 {
		t.Fatalf("res = %+v", res)
	}
}

func TestPublicStatsUnwrapsEnvelope(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/public/stats/launch" {
			t.Errorf("path = %s", r.URL.Path)
		}
		q := r.URL.Query()
		if q.Get("start_date") != "2026-01-01T00:00:00Z" || q.Get("timezone") != "UTC" {
			t.Errorf("unexpected query: %v", q)
		}
		if q.Has("group_by") || q.Has("scope") {
			t.Errorf("public endpoint takes only a range and timezone: %v", q)
		}
		w.Write([]byte(`{
			"generation": "v2",
			"link": {"alias": "launch", "domain": "spoo.me"},
			"stats": {
				"scope": "anon",
				"summary": {"total_clicks": 9, "unique_clicks": 5},
				"metrics": {"clicks_by_browser": [{"browser": "Chrome", "clicks": 9}]}
			}
		}`))
	}))
	defer srv.Close()

	c := NewClient(WithBaseURL(srv.URL))
	res, err := c.PublicStats(context.Background(), "launch", PublicStatsQuery{
		StartDate: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		Timezone:  "UTC",
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Summary.TotalClicks != 9 || len(res.Metrics["clicks_by_browser"]) != 1 {
		t.Fatalf("res = %+v", res)
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

	c := NewClient(WithBaseURL(srv.URL))
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

// Per-link exports slice the unified endpoint with url_id, not the
// legacy /export/links/{id} path.
func TestExportLinkSlicesUnifiedEndpoint(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/export" {
			t.Errorf("path = %s, want the unified export endpoint", r.URL.Path)
		}
		if r.URL.Query().Get("url_id") != "65f0abc123" {
			t.Errorf("url_id = %q", r.URL.Query().Get("url_id"))
		}
		w.Header().Set("Content-Disposition", `attachment; filename="stats-launch.zip"`)
		w.Write([]byte("FAKEZIP"))
	}))
	defer srv.Close()

	c := NewClient(WithBaseURL(srv.URL))
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

func TestExportErrorMapsEnvelope(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error":"bad format","code":"VALIDATION_ERROR"}`))
	}))
	defer srv.Close()

	c := NewClient(WithBaseURL(srv.URL))
	_, err := c.Export(context.Background(), StatsQuery{}, "bmp")
	var apiErr *Error
	if !errors.As(err, &apiErr) || apiErr.Code != "VALIDATION_ERROR" {
		t.Fatalf("err = %v, want the parsed envelope", err)
	}
}
