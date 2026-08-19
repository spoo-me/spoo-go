package spoo

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/spoo-me/spoo-go/option"
)

// The emoji set is served from the client cache on 304: the second call
// revalidates with If-None-Match and never re-downloads the payload.
func TestEmojiSetETagCache(t *testing.T) {
	var fullResponses atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/emoji-set" {
			t.Errorf("path = %s", r.URL.Path)
		}
		if r.Header.Get("If-None-Match") == `"v42"` {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		fullResponses.Add(1)
		w.Header().Set("ETag", `"v42"`)
		w.Write([]byte(`{
			"accept_max_version": 15.1,
			"generate_max_version": 14.0,
			"max_graphemes": 15,
			"emoji": [{"c": "🚀", "n": "rocket", "g": "Travel & Places", "gen": true, "k": ["launch"]}]
		}`))
	}))
	defer srv.Close()

	c := NewClient(option.WithBaseURL(srv.URL))
	first, err := c.EmojiSet(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Emoji) != 1 || first.Emoji[0].Name != "rocket" || first.MaxGraphemes != 15 {
		t.Fatalf("set = %+v", first)
	}
	second, err := c.EmojiSet(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if second != first {
		t.Fatal("304 must serve the cached copy")
	}
	if fullResponses.Load() != 1 {
		t.Fatalf("full payload served %d times, want 1", fullResponses.Load())
	}
}

// A changed ETag replaces the cache instead of serving stale data.
func TestEmojiSetCacheInvalidation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("ETag", `"v2"`)
		w.Write([]byte(`{"accept_max_version": 16.0, "generate_max_version": 14.0, "max_graphemes": 15, "emoji": []}`))
	}))
	defer srv.Close()

	c := NewClient(option.WithBaseURL(srv.URL))
	c.emojiETag, c.emojiCache = `"v1"`, &EmojiSet{AcceptMaxVersion: 15.1}
	set, err := c.EmojiSet(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if set.AcceptMaxVersion != 16.0 {
		t.Fatalf("set = %+v, want the fresh payload", set)
	}
	if c.emojiETag != `"v2"` {
		t.Fatalf("etag = %q, want the rotated one", c.emojiETag)
	}
}
