package spoo

import (
	"context"
	"maps"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"
)

// StatsQuery parameterizes the authed stats and export endpoints.
type StatsQuery struct {
	StartDate time.Time
	EndDate   time.Time
	GroupBy   []string // time, browser, os, device, country, city, referrer, utm_source, utm_medium, utm_campaign; account-only: short_code
	Metrics   []string // clicks, unique_clicks (the default when empty)
	Timezone  string   // IANA name
	// Filters narrows results server-side; keys are the filterable
	// dimensions (browser, os, device, country, city, referrer, the
	// utm_* trio) plus the slicing filters short_code and url_id,
	// which restrict the account aggregate to specific owned links.
	Filters map[string][]string
}

func (q StatsQuery) values() url.Values {
	v := url.Values{}
	if !q.StartDate.IsZero() {
		v.Set("start_date", q.StartDate.UTC().Format(time.RFC3339))
	}
	if !q.EndDate.IsZero() {
		v.Set("end_date", q.EndDate.UTC().Format(time.RFC3339))
	}
	if len(q.GroupBy) > 0 {
		v.Set("group_by", strings.Join(q.GroupBy, ","))
	}
	if len(q.Metrics) > 0 {
		v.Set("metrics", strings.Join(q.Metrics, ","))
	}
	if q.Timezone != "" {
		v.Set("timezone", q.Timezone)
	}
	for _, dim := range slices.Sorted(maps.Keys(q.Filters)) {
		if vals := q.Filters[dim]; len(vals) > 0 {
			v.Set(dim, strings.Join(vals, ","))
		}
	}
	return v
}

// StatsSummary is the headline aggregate for a stats window.
type StatsSummary struct {
	TotalClicks        int       `json:"total_clicks"`
	UniqueClicks       int       `json:"unique_clicks"`
	FirstClick         Timestamp `json:"first_click"`
	LastClick          Timestamp `json:"last_click"`
	AvgRedirectionTime float64   `json:"avg_redirection_time"`
}

// StatsTimeRange echoes the window a stats response covers.
type StatsTimeRange struct {
	StartDate Timestamp `json:"start_date"`
	EndDate   Timestamp `json:"end_date"`
}

// StatsResponse keeps Metrics loosely typed: keys are dynamic
// ("clicks_by_browser", "unique_clicks_by_time", ...) and each point
// carries its dimension label under the dimension's own name.
// The wire still carries a legacy "scope" key — tolerated, never read.
type StatsResponse struct {
	URLID           string                      `json:"url_id"` // per-link responses echo the link
	Alias           string                      `json:"alias"`
	Summary         StatsSummary                `json:"summary"`
	TimeRange       StatsTimeRange              `json:"time_range"`
	Metrics         map[string][]map[string]any `json:"metrics"`
	ComputedMetrics map[string]float64          `json:"computed_metrics"`
	GeneratedAt     Timestamp                   `json:"generated_at"`
}

// MaxRangeDays is the widest window the stats endpoint accepts; without
// explicit dates it defaults to only the LAST 7 DAYS, so clients that
// want "all recent activity" should request this window explicitly.
const MaxRangeDays = 90

// MetricPoint is one (label, value) pair extracted from the loosely
// typed metrics payload by [StatsResponse.Points].
type MetricPoint struct {
	Label string
	Value float64
}

// Points extracts (label, value) pairs from the loosely typed metrics
// payload for one dimension/metric pair, e.g. ("browser", "clicks") →
// the "clicks_by_browser" series with labels from the "browser" key.
func (r *StatsResponse) Points(dimension, metric string) []MetricPoint {
	pts := r.Metrics[metric+"_by_"+dimension]
	out := make([]MetricPoint, 0, len(pts))
	for _, p := range pts {
		label, _ := p[dimension].(string)
		value, ok := p[metric].(float64)
		if label == "" || !ok {
			continue
		}
		out = append(out, MetricPoint{Label: label, Value: value})
	}
	return out
}

// Stats aggregates clicks across every link the account owns.
// Auth is required — anonymous stats live on PublicStats.
func (c *Client) Stats(ctx context.Context, q StatsQuery) (*StatsResponse, error) {
	var out StatsResponse
	if err := c.do(ctx, http.MethodGet, "/api/v1/stats", q.values(), nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// LinkStats returns stats for one owned link by its url id (resolve an
// alias with ResolveAlias first, or use StatsByAlias). Unknown and
// foreign ids both 404.
func (c *Client) LinkStats(ctx context.Context, urlID string, q StatsQuery) (*StatsResponse, error) {
	var out StatsResponse
	path := "/api/v1/stats/links/" + url.PathEscape(urlID)
	if err := c.do(ctx, http.MethodGet, path, q.values(), nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// StatsByAlias returns stats for one owned link addressed by alias and
// domain, folding the resolve-then-fetch dance every caller performs
// into one call. Pass "spoo.me" as the domain for links on the default
// namespace.
func (c *Client) StatsByAlias(ctx context.Context, alias, domain string, q StatsQuery) (*StatsResponse, error) {
	item, err := c.ResolveAlias(ctx, alias, domain)
	if err != nil {
		return nil, err
	}
	return c.LinkStats(ctx, item.ID, q)
}

// PublicStatsQuery parameterizes PublicStats. The public endpoint takes
// only a date range and timezone — no group_by — and answers with every
// dimension at once.
type PublicStatsQuery struct {
	StartDate time.Time
	EndDate   time.Time
	Timezone  string // IANA name
}

// PublicStats returns anyone's per-link stats without auth. Private
// links 404 and password-protected ones 401 (ErrLinkPasswordProtected).
// The {generation, link, stats} envelope is unwrapped to the standard
// stats wire.
func (c *Client) PublicStats(ctx context.Context, shortCode string, q PublicStatsQuery) (*StatsResponse, error) {
	v := url.Values{}
	if !q.StartDate.IsZero() {
		v.Set("start_date", q.StartDate.UTC().Format(time.RFC3339))
	}
	if !q.EndDate.IsZero() {
		v.Set("end_date", q.EndDate.UTC().Format(time.RFC3339))
	}
	if q.Timezone != "" {
		v.Set("timezone", q.Timezone)
	}
	var out struct {
		Stats StatsResponse `json:"stats"`
	}
	path := "/api/v1/public/stats/" + url.PathEscape(shortCode)
	if err := c.do(ctx, http.MethodGet, path, v, nil, &out); err != nil {
		return nil, err
	}
	return &out.Stats, nil
}
