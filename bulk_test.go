package spoo

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// Bulk ops answer HTTP 200 even when every item fails; outcomes are
// data with a summary and per-item error codes.
func TestBulkDeleteAllFailuresIsNotAnError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/urls/bulk/delete" || r.Method != http.MethodPost {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.Write([]byte(`{
			"summary": {"total": 2, "succeeded": 0, "failed": 2},
			"results": [
				{"id": "a", "alias": null, "ok": false, "error_code": "not_found", "error": "URL not found"},
				{"id": "b", "alias": null, "ok": false, "error_code": "forbidden", "error": "not yours"}
			]
		}`))
	}))
	defer srv.Close()

	c := NewClient(WithBaseURL(srv.URL), WithAPIKey("spoo_key"))
	res, err := c.BulkDelete(context.Background(), []string{"a", "b"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Summary.Failed != 2 || res.Summary.Succeeded != 0 {
		t.Fatalf("summary = %+v", res.Summary)
	}
	if res.Results[0].ErrorCode != "not_found" || res.Results[1].OK {
		t.Fatalf("results = %+v", res.Results)
	}
}

func TestBulkUpdateStatusBody(t *testing.T) {
	var gotBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotBody, _ = io.ReadAll(r.Body)
		w.Write([]byte(`{"summary":{"total":1,"succeeded":1,"failed":0},"results":[{"id":"a","alias":"x","ok":true}]}`))
	}))
	defer srv.Close()

	c := NewClient(WithBaseURL(srv.URL))
	res, err := c.BulkUpdateStatus(context.Background(), []string{"a"}, "INACTIVE")
	if err != nil {
		t.Fatal(err)
	}
	if string(gotBody) != `{"ids":["a"],"status":"INACTIVE"}` {
		t.Fatalf("body = %s", gotBody)
	}
	if !res.Results[0].OK || res.Results[0].Alias != "x" {
		t.Fatalf("results = %+v", res.Results)
	}
}

// expire_after is a required, nullable field: a time serializes as
// RFC 3339 and the zero time as null (clear expiry).
func TestBulkUpdateExpiryWire(t *testing.T) {
	var bodies []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, _ := io.ReadAll(r.Body)
		bodies = append(bodies, string(data))
		w.Write([]byte(`{"summary":{"total":1,"succeeded":1,"failed":0},"results":[{"id":"a","ok":true}]}`))
	}))
	defer srv.Close()

	c := NewClient(WithBaseURL(srv.URL))
	when := time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)
	if _, err := c.BulkUpdateExpiry(context.Background(), []string{"a"}, when); err != nil {
		t.Fatal(err)
	}
	if _, err := c.BulkUpdateExpiry(context.Background(), []string{"a"}, time.Time{}); err != nil {
		t.Fatal(err)
	}
	if bodies[0] != `{"ids":["a"],"expire_after":"2027-01-01T00:00:00Z"}` {
		t.Fatalf("set body = %s", bodies[0])
	}
	if bodies[1] != `{"ids":["a"],"expire_after":null}` {
		t.Fatalf("clear body = %s", bodies[1])
	}
}

// domain is a required, nullable field: "" means back to the system
// default and serializes as null.
func TestBulkMoveDomainWire(t *testing.T) {
	var bodies []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, _ := io.ReadAll(r.Body)
		bodies = append(bodies, string(data))
		w.Write([]byte(`{"summary":{"total":1,"succeeded":1,"failed":0},"results":[{"id":"a","ok":true}]}`))
	}))
	defer srv.Close()

	c := NewClient(WithBaseURL(srv.URL))
	if _, err := c.BulkMoveDomain(context.Background(), []string{"a"}, "links.example.com"); err != nil {
		t.Fatal(err)
	}
	if _, err := c.BulkMoveDomain(context.Background(), []string{"a"}, ""); err != nil {
		t.Fatal(err)
	}
	var move, cleared map[string]json.RawMessage
	if err := json.Unmarshal([]byte(bodies[0]), &move); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(bodies[1]), &cleared); err != nil {
		t.Fatal(err)
	}
	if string(move["domain"]) != `"links.example.com"` {
		t.Fatalf("move body = %s", bodies[0])
	}
	if string(cleared["domain"]) != "null" {
		t.Fatalf("clear body = %s", bodies[1])
	}
}
