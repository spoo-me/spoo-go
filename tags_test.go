package spoo

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/spoo-me/spoo-go/option"
)

func TestListTagsUnwrapsItems(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/tags" || r.Method != http.MethodGet {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.Write([]byte(`{"items":[
			{"id":"t1","name":"launch","color":"violet","icon":"rocket","link_count":14,"created_at":"2026-06-01T10:00:00Z","updated_at":null},
			{"id":"t2","name":"q3","color":"teal","icon":"tag","link_count":0,"created_at":"2026-06-02T10:00:00Z","updated_at":"2026-06-03T10:00:00Z"}
		]}`))
	}))
	defer srv.Close()

	c := NewClient(option.WithBaseURL(srv.URL), option.WithAPIKey("spoo_key"))
	tags, err := c.ListTags(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(tags) != 2 || tags[0].Name != "launch" || tags[0].LinkCount != 14 || tags[0].Color != "violet" {
		t.Fatalf("tags = %+v", tags)
	}
	if !tags[0].CreatedAt.Equal(time.Date(2026, 6, 1, 10, 0, 0, 0, time.UTC)) {
		t.Fatalf("CreatedAt = %v", tags[0].CreatedAt)
	}
	if !tags[0].UpdatedAt.IsZero() {
		t.Fatalf("UpdatedAt = %v, want zero for null", tags[0].UpdatedAt)
	}
	if tags[1].UpdatedAt.IsZero() {
		t.Fatalf("UpdatedAt = %v, want the wire value", tags[1].UpdatedAt)
	}
}

// Empty color and icon are omitted so the server picks its defaults.
func TestCreateTagBody(t *testing.T) {
	var bodies []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/tags" || r.Method != http.MethodPost {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		data, _ := io.ReadAll(r.Body)
		bodies = append(bodies, string(data))
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"id":"t1","name":"launch","color":"violet","icon":"rocket","link_count":0,"created_at":"2026-06-01T10:00:00Z"}`))
	}))
	defer srv.Close()

	c := NewClient(option.WithBaseURL(srv.URL))
	tag, err := c.CreateTag(context.Background(), CreateTagParams{Name: "Launch", Color: "violet", Icon: "rocket"})
	if err != nil {
		t.Fatal(err)
	}
	if tag.ID != "t1" || tag.Name != "launch" || tag.Icon != "rocket" {
		t.Fatalf("tag = %+v", tag)
	}
	if _, err := c.CreateTag(context.Background(), CreateTagParams{Name: "q3"}); err != nil {
		t.Fatal(err)
	}
	if bodies[0] != `{"name":"Launch","color":"violet","icon":"rocket"}` {
		t.Fatalf("full body = %s", bodies[0])
	}
	if bodies[1] != `{"name":"q3"}` {
		t.Fatalf("minimal body = %s", bodies[1])
	}
}

// Omitted fields keep their value, so only the ones passed hit the wire.
func TestUpdateTagSendsPatch(t *testing.T) {
	var gotBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/tags/t1" || r.Method != http.MethodPatch {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		gotBody, _ = io.ReadAll(r.Body)
		w.Write([]byte(`{"id":"t1","name":"launch","color":"teal","icon":"rocket","link_count":14,"created_at":"2026-06-01T10:00:00Z","updated_at":"2026-06-03T10:00:00Z"}`))
	}))
	defer srv.Close()

	c := NewClient(option.WithBaseURL(srv.URL))
	tag, err := c.UpdateTag(context.Background(), "t1", UpdateTagParams{Color: "teal"})
	if err != nil {
		t.Fatal(err)
	}
	if string(gotBody) != `{"color":"teal"}` {
		t.Fatalf("body = %s", gotBody)
	}
	if tag.Color != "teal" || tag.UpdatedAt.IsZero() {
		t.Fatalf("tag = %+v", tag)
	}
}

func TestDeleteTagReportsLinksUpdated(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/tags/t1" || r.Method != http.MethodDelete {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.Write([]byte(`{"deleted":true,"links_updated":14}`))
	}))
	defer srv.Close()

	c := NewClient(option.WithBaseURL(srv.URL))
	res, err := c.DeleteTag(context.Background(), "t1")
	if err != nil {
		t.Fatal(err)
	}
	if !res.Deleted || res.LinksUpdated != 14 {
		t.Fatalf("res = %+v", res)
	}
}

func TestDeleteTagNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"error":"Tag not found","code":"not_found"}`))
	}))
	defer srv.Close()

	c := NewClient(option.WithBaseURL(srv.URL))
	if _, err := c.DeleteTag(context.Background(), "nope"); !IsNotFound(err) {
		t.Fatalf("err = %v, want IsNotFound", err)
	}
}
