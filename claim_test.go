package spoo

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// Claims resolve independently: partial success is data, not an error.
func TestClaimURLsPartialSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/urls/claim" || r.Method != http.MethodPost {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		var body struct {
			Claims []map[string]string `json:"claims"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode: %v", err)
		}
		if len(body.Claims) != 2 || body.Claims[0]["url_id"] != "65f0abc123" || body.Claims[0]["token"] == "" {
			t.Errorf("claims = %v", body.Claims)
		}
		w.Write([]byte(`{
			"results": [
				{"url_id": "65f0abc123", "status": "claimed"},
				{"url_id": "65f0abc124", "status": "invalid"}
			],
			"claimed": 1
		}`))
	}))
	defer srv.Close()

	c := NewClient(WithBaseURL(srv.URL), WithAPIKey("spoo_key"))
	out, err := c.ClaimURLs(context.Background(), []Claim{
		{URLID: "65f0abc123", Token: "claimtok-claimtok-claimtok-claimtok-claimtok"},
		{URLID: "65f0abc124", Token: "wrong-token-wrong-token-wrong-token-wrong-tok"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.Claimed != 1 || len(out.Results) != 2 {
		t.Fatalf("out = %+v", out)
	}
	if out.Results[0].Status != ClaimStatusClaimed || out.Results[1].Status != ClaimStatusInvalid {
		t.Fatalf("results = %+v", out.Results)
	}
}
