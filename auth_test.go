package spoo

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/spoo-me/spoo-go/option"
)

func TestMe(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"user":{"id":"1","email":"a@b.c","email_verified":true,"name":"A","plan":"free"}}`))
	}))
	defer srv.Close()

	c := NewClient(option.WithBaseURL(srv.URL))
	u, err := c.Me(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if u.Email != "a@b.c" || !u.EmailVerified {
		t.Fatalf("unexpected user: %+v", u)
	}
}
