package spoo

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/spoo-me/spoo-go/option"
)

func TestShorten(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/shorten" || r.Method != http.MethodPost {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		var req map[string]any
		json.NewDecoder(r.Body).Decode(&req)
		if req["long_url"] != "https://example.com" || req["alias"] != "mylink" {
			t.Errorf("unexpected body: %v", req)
		}
		if _, ok := req["password"]; ok {
			t.Error("empty optional fields must be omitted")
		}
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"id":"x","short_url":"https://spoo.me/mylink","alias":"mylink","long_url":"https://example.com","status":"ACTIVE"}`))
	}))
	defer srv.Close()

	c := NewClient(option.WithBaseURL(srv.URL))
	res, err := c.Shorten(context.Background(), ShortenRequest{LongURL: "https://example.com", Alias: "mylink"})
	if err != nil {
		t.Fatal(err)
	}
	if res.ShortURL != "https://spoo.me/mylink" {
		t.Fatalf("ShortURL = %q", res.ShortURL)
	}
}

// The zero-valued request compiles, so an empty LongURL must fail
// before it becomes {"long_url": ""} on the wire.
func TestShortenRejectsEmptyLongURL(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		t.Errorf("no request should go out, got %s %s", r.Method, r.URL.Path)
	}))
	defer srv.Close()

	c := NewClient(option.WithBaseURL(srv.URL))
	_, err := c.Shorten(context.Background(), ShortenRequest{Alias: "mylink"})
	if !errors.Is(err, ErrMissingLongURL) {
		t.Fatalf("err = %v, want ErrMissingLongURL", err)
	}
}

func TestCheckAlias(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/shorten/check-alias" {
			t.Errorf("path = %s", r.URL.Path)
		}
		if r.URL.Query().Get("alias") != "taken1" {
			t.Errorf("alias = %q", r.URL.Query().Get("alias"))
		}
		w.Write([]byte(`{"available":false,"reason":"taken"}`))
	}))
	defer srv.Close()

	c := NewClient(option.WithBaseURL(srv.URL))
	res, err := c.CheckAlias(context.Background(), "taken1", "")
	if err != nil {
		t.Fatal(err)
	}
	if res.Available || res.Reason != "taken" {
		t.Fatalf("unexpected: %+v", res)
	}
}
