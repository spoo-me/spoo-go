package spoo

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestListKeys(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"keys":[{"id":"k1","name":"CI","scopes":["shorten:create"],"token_prefix":"abc12345","revoked":false}]}`))
	}))
	defer srv.Close()

	c := New(srv.URL, nil)
	keys, err := c.ListKeys(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(keys) != 1 || keys[0].TokenPrefix != "abc12345" {
		t.Fatalf("keys = %+v", keys)
	}
}
