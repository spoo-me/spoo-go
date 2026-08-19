package spoo

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPublicPreview(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.EscapedPath() != "/api/v1/public/preview/%F0%9F%9A%80" {
			t.Errorf("path = %s", r.URL.EscapedPath())
		}
		if r.Header.Get("Authorization") != "" {
			t.Error("preview must work anonymously")
		}
		w.Write([]byte(`{
			"generation": "v2",
			"alias": "🚀",
			"short_url": "https://spoo.me/🚀",
			"status": "active",
			"created_at": "2026-06-01T10:00:00Z",
			"password_protected": true,
			"destination": {"url": "https://example.com/x", "domain": "example.com", "path": "/x", "is_https": true},
			"geo_destinations": null
		}`))
	}))
	defer srv.Close()

	c := NewClient(WithBaseURL(srv.URL))
	p, err := c.PublicPreview(context.Background(), "🚀")
	if err != nil {
		t.Fatal(err)
	}
	if p.Status != "active" || !p.PasswordProtected {
		t.Fatalf("preview = %+v", p)
	}
	if p.Destination == nil || p.Destination.Domain != "example.com" || !p.Destination.IsHTTPS {
		t.Fatalf("destination = %+v", p.Destination)
	}
}

// A non-active link previews with a nil destination.
func TestPublicPreviewWithheldDestination(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{
			"generation": "v1",
			"alias": "gone",
			"short_url": "https://spoo.me/gone",
			"status": "expired",
			"created_at": null,
			"password_protected": false,
			"destination": null,
			"geo_destinations": null
		}`))
	}))
	defer srv.Close()

	c := NewClient(WithBaseURL(srv.URL))
	p, err := c.PublicPreview(context.Background(), "gone")
	if err != nil {
		t.Fatal(err)
	}
	if p.Destination != nil || p.Status != "expired" || !p.CreatedAt.IsZero() {
		t.Fatalf("preview = %+v", p)
	}
}
