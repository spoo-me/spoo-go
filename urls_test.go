package spoo

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestListURLsBuildsQueryAndFilter(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("page") != "2" || q.Get("pageSize") != "50" || q.Get("sortBy") != "total_clicks" {
			t.Errorf("unexpected query: %v", q)
		}
		var filter map[string]any
		if err := json.Unmarshal([]byte(q.Get("filter")), &filter); err != nil {
			t.Errorf("filter not JSON: %v", err)
		}
		if filter["search"] != "launch" || filter["status"] != "ACTIVE" {
			t.Errorf("unexpected filter: %v", filter)
		}
		w.Write([]byte(`{"items":[{"id":"a1","alias":"launch","long_url":"https://x.com","created_at":"2026-06-01T10:00:00Z","expire_after":1781524800,"total_clicks":42,"status":"ACTIVE","password_set":false}],"page":2,"pageSize":50,"total":51,"hasNext":false}`))
	}))
	defer srv.Close()

	c := NewClient(WithBaseURL(srv.URL))
	page, err := c.ListURLs(context.Background(), ListURLsOptions{
		Page: 2, PageSize: 50, SortBy: "total_clicks", Search: "launch", Status: "ACTIVE",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.Items[0].TotalClicks != 42 || page.Items[0].LongURL != "https://x.com" {
		t.Fatalf("unexpected page: %+v", page)
	}
	// mixed wire formats normalize: ISO string and Unix seconds
	item := page.Items[0]
	if !item.CreatedAt.Equal(time.Date(2026, 6, 1, 10, 0, 0, 0, time.UTC)) {
		t.Fatalf("CreatedAt = %v", item.CreatedAt)
	}
	if !item.ExpireAfter.Equal(time.Unix(1781524800, 0)) {
		t.Fatalf("ExpireAfter = %v", item.ExpireAfter)
	}
	if !item.LastClick.IsZero() {
		t.Fatalf("LastClick = %v, want zero for absent field", item.LastClick)
	}
}

func TestListURLsAllPagesLazily(t *testing.T) {
	var requests int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		switch r.URL.Query().Get("page") {
		case "1":
			w.Write([]byte(`{"items":[{"id":"a","password_set":false},{"id":"b","password_set":false}],"page":1,"hasNext":true}`))
		case "2":
			w.Write([]byte(`{"items":[{"id":"c","password_set":false}],"page":2,"hasNext":false}`))
		default:
			t.Errorf("unexpected page %q", r.URL.Query().Get("page"))
		}
	}))
	defer srv.Close()

	c := NewClient(WithBaseURL(srv.URL))
	var ids []string
	for item, err := range c.ListURLsAll(context.Background(), ListURLsOptions{}) {
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, item.ID)
	}
	if len(ids) != 3 || ids[0] != "a" || ids[2] != "c" {
		t.Fatalf("ids = %v", ids)
	}
	if requests != 2 {
		t.Fatalf("requests = %d, want 2", requests)
	}
}

func TestListURLsAllStopsOnEarlyBreak(t *testing.T) {
	var requests int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests++
		w.Write([]byte(`{"items":[{"id":"a","password_set":false},{"id":"b","password_set":false}],"page":1,"hasNext":true}`))
	}))
	defer srv.Close()

	c := NewClient(WithBaseURL(srv.URL))
	for item, err := range c.ListURLsAll(context.Background(), ListURLsOptions{}) {
		if err != nil {
			t.Fatal(err)
		}
		if item.ID == "a" {
			break // the second page must never be fetched
		}
	}
	if requests != 1 {
		t.Fatalf("requests = %d, want 1 (lazy fetch)", requests)
	}
}

func TestListURLsAllYieldsFetchError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(`{"error":"nope","code":"AUTHORIZATION_ERROR"}`))
	}))
	defer srv.Close()

	c := NewClient(WithBaseURL(srv.URL))
	var got error
	for _, err := range c.ListURLsAll(context.Background(), ListURLsOptions{}) {
		got = err
	}
	var apiErr *Error
	if !errors.As(got, &apiErr) || apiErr.StatusCode != http.StatusForbidden {
		t.Fatalf("err = %v, want the API error yielded", got)
	}
}

func TestGetURL(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/urls/65f0abc123" || r.Method != http.MethodGet {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.Write([]byte(`{"id":"65f0abc123","alias":"launch","long_url":"https://x.com","status":"ACTIVE","password_set":true}`))
	}))
	defer srv.Close()

	c := NewClient(WithBaseURL(srv.URL))
	u, err := c.GetURL(context.Background(), "65f0abc123")
	if err != nil {
		t.Fatal(err)
	}
	if u.Alias != "launch" || !u.PasswordSet {
		t.Fatalf("unexpected: %+v", u)
	}
}

func TestResolveAliasEscapesPath(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.EscapedPath()
		w.Write([]byte(`{"id":"65f0abc123","alias":"🚀","long_url":"https://x.com","status":"ACTIVE","password_set":false}`))
	}))
	defer srv.Close()

	c := NewClient(WithBaseURL(srv.URL))
	u, err := c.ResolveAlias(context.Background(), "🚀", "spoo.me")
	if err != nil {
		t.Fatal(err)
	}
	// the emoji alias must arrive percent-encoded
	if gotPath != "/api/v1/urls/spoo.me/%F0%9F%9A%80" {
		t.Fatalf("path = %q", gotPath)
	}
	if u.ID != "65f0abc123" {
		t.Fatalf("id = %q", u.ID)
	}
}

// The domain names the namespace and is never guessed from the base
// URL: a self-hosted client would silently resolve against the wrong
// namespace otherwise.
func TestResolveAliasRequiresDomain(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		t.Error("no request must be sent without a domain")
	}))
	defer srv.Close()

	c := NewClient(WithBaseURL(srv.URL))
	if _, err := c.ResolveAlias(context.Background(), "launch", ""); err == nil {
		t.Fatal("want an error for the missing domain")
	}
}

func TestResolveAliasNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"error":"URL not found","code":"not_found"}`))
	}))
	defer srv.Close()

	c := NewClient(WithBaseURL(srv.URL))
	_, err := c.ResolveAlias(context.Background(), "nope", "spoo.me")
	if !IsNotFound(err) {
		t.Fatalf("err = %v, want IsNotFound", err)
	}
}

// a custom domain replaces the default namespace in the resolve path,
// so links on the user's own domains resolve to their real url ids.
func TestResolveAliasCustomDomain(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.EscapedPath()
		w.Write([]byte(`{"id":"65f0abc123","alias":"promo","long_url":"https://x.com","status":"ACTIVE","password_set":false}`))
	}))
	defer srv.Close()

	c := NewClient(WithBaseURL(srv.URL))
	if _, err := c.ResolveAlias(context.Background(), "promo", "links.example.com"); err != nil {
		t.Fatal(err)
	}
	if gotPath != "/api/v1/urls/links.example.com/promo" {
		t.Fatalf("path = %q, want the custom domain in the path", gotPath)
	}
}

func TestUpdateURLSendsPatch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch || r.URL.Path != "/api/v1/urls/abc123" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		if body["status"] != "INACTIVE" {
			t.Errorf("unexpected body: %v", body)
		}
		w.Write([]byte(`{"id":"abc123","alias":"x","status":"INACTIVE","password_set":false,"updated_at":1781524800}`))
	}))
	defer srv.Close()

	c := NewClient(WithBaseURL(srv.URL))
	res, err := c.UpdateURL(context.Background(), "abc123", UpdateURLParams{Status: "INACTIVE"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != "INACTIVE" {
		t.Fatalf("status = %q", res.Status)
	}
	if !res.UpdatedAt.Equal(time.Unix(1781524800, 0)) {
		t.Fatalf("UpdatedAt = %v", res.UpdatedAt)
	}
}

// The PATCH body must keep the API's tri-state semantics: omitted keeps
// the current setting, null clears it, a value replaces it.
func TestUpdateURLTriState(t *testing.T) {
	var gotBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotBody, _ = io.ReadAll(r.Body)
		w.Write([]byte(`{"id":"abc123","password_set":false,"updated_at":1781524800}`))
	}))
	defer srv.Close()

	c := NewClient(WithBaseURL(srv.URL))
	_, err := c.UpdateURL(context.Background(), "abc123", UpdateURLParams{
		LongURL:     "https://example.com/new",
		Password:    Null[string](),                                   // remove protection
		MaxClicks:   Set(0),                                           // 0 also removes the limit
		ExpireAfter: Set(time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)), // replace
		BlockBots:   Set(false),                                       // false must be expressible
		// Alias, Status, Domain, PrivateStats omitted: keep existing
	})
	if err != nil {
		t.Fatal(err)
	}
	var body map[string]json.RawMessage
	if err := json.Unmarshal(gotBody, &body); err != nil {
		t.Fatalf("body not JSON: %v (%s)", err, gotBody)
	}
	assertRaw := func(key, want string) {
		t.Helper()
		raw, ok := body[key]
		if !ok {
			t.Fatalf("%s missing from body %s", key, gotBody)
		}
		if string(raw) != want {
			t.Fatalf("%s = %s, want %s", key, raw, want)
		}
	}
	assertRaw("long_url", `"https://example.com/new"`)
	assertRaw("password", "null")
	assertRaw("max_clicks", "0")
	assertRaw("expire_after", `"2027-01-01T00:00:00Z"`)
	assertRaw("block_bots", "false")
	for _, key := range []string{"alias", "status", "domain", "private_stats"} {
		if _, ok := body[key]; ok {
			t.Fatalf("%s must be omitted, body = %s", key, gotBody)
		}
	}
}

func TestDeleteURL(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete || r.URL.Path != "/api/v1/urls/abc123" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.Write([]byte(`{"message":"deleted","id":"abc123"}`))
	}))
	defer srv.Close()

	c := NewClient(WithBaseURL(srv.URL))
	if err := c.DeleteURL(context.Background(), "abc123"); err != nil {
		t.Fatal(err)
	}
}
