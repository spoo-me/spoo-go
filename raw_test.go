package spoo

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/spoo-me/spoo-go/option"
)

// The raw methods must ride the same machinery as every typed call:
// auth header, client tag, query encoding, body marshaling, decoding.
func TestRawMethodsReuseClientMachinery(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer spoo_key123" {
			t.Errorf("Authorization = %q", r.Header.Get("Authorization"))
		}
		if r.Header.Get("X-Spoo-Client") != "raw-test/1.0" {
			t.Errorf("X-Spoo-Client = %q", r.Header.Get("X-Spoo-Client"))
		}
		switch r.Method {
		case http.MethodGet:
			if r.URL.Path != "/api/v1/future" || r.URL.Query().Get("page") != "2" {
				t.Errorf("unexpected GET: %s %v", r.URL.Path, r.URL.Query())
			}
		case http.MethodPost, http.MethodPut, http.MethodPatch:
			var body map[string]any
			json.NewDecoder(r.Body).Decode(&body)
			if body["name"] != "x" {
				t.Errorf("%s body = %v", r.Method, body)
			}
		case http.MethodDelete:
			if r.URL.Path != "/api/v1/future/abc" {
				t.Errorf("unexpected DELETE path: %s", r.URL.Path)
			}
		}
		w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	c := NewClient(
		option.WithBaseURL(srv.URL),
		option.WithAPIKey("spoo_key123"),
		option.WithClientTag("raw-test/1.0"),
	)
	ctx := context.Background()
	body := map[string]string{"name": "x"}

	var out struct {
		OK bool `json:"ok"`
	}
	if err := c.Get(ctx, "/api/v1/future", url.Values{"page": {"2"}}, &out); err != nil || !out.OK {
		t.Fatalf("Get: err=%v out=%+v", err, out)
	}
	if err := c.Post(ctx, "/api/v1/future", body, nil); err != nil {
		t.Fatalf("Post: %v", err)
	}
	if err := c.Put(ctx, "/api/v1/future", body, nil); err != nil {
		t.Fatalf("Put: %v", err)
	}
	if err := c.Patch(ctx, "/api/v1/future", body, nil); err != nil {
		t.Fatalf("Patch: %v", err)
	}
	if err := c.Delete(ctx, "/api/v1/future/abc", nil); err != nil {
		t.Fatalf("Delete: %v", err)
	}
}

// Raw calls surface API failures as the same *Error every typed
// method returns.
func TestRawMethodsMapErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"error":"no such thing","code":"not_found"}`))
	}))
	defer srv.Close()

	c := NewClient(option.WithBaseURL(srv.URL))
	err := c.Get(context.Background(), "/api/v1/future", nil, nil)
	var apiErr *Error
	if !errors.As(err, &apiErr) || apiErr.Code != "not_found" {
		t.Fatalf("err = %v, want *Error with code not_found", err)
	}
	if !IsNotFound(err) {
		t.Fatalf("IsNotFound(%v) = false", err)
	}
}
