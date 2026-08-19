package spoo

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWithAPIKeySendsBearer(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Write([]byte(`{"user":{"id":"1"}}`))
	}))
	defer srv.Close()

	c := NewClient(WithBaseURL(srv.URL), WithAPIKey("spoo_testkey"))
	if _, err := c.Me(context.Background()); err != nil {
		t.Fatal(err)
	}
	if gotAuth != "Bearer spoo_testkey" {
		t.Fatalf("Authorization = %q, want Bearer spoo_testkey", gotAuth)
	}
}

func TestEmptyAPIKeyStaysAnonymous(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	c := NewClient(WithBaseURL(srv.URL), WithAPIKey(""))
	if err := c.do(context.Background(), http.MethodGet, "/api/v1/shorten/check-alias", nil, nil, nil); err != nil {
		t.Fatal(err)
	}
	if gotAuth != "" {
		t.Fatalf("Authorization = %q, want none (anonymous)", gotAuth)
	}
}

func TestTokenSourceErrorAborts(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		t.Error("request must not be sent when the token source fails")
	}))
	defer srv.Close()

	wantErr := errors.New("keyring locked")
	c := NewClient(WithBaseURL(srv.URL), WithTokenSource(failingSource{err: wantErr}))
	if _, err := c.Me(context.Background()); !errors.Is(err, wantErr) {
		t.Fatalf("err = %v, want the token-source error", err)
	}
}

type failingSource struct{ err error }

func (f failingSource) Token(context.Context) (Credentials, error) {
	return Credentials{}, f.err
}

func (f failingSource) Update(context.Context, Credentials) error { return f.err }

func TestWithClientTagOverridesDefault(t *testing.T) {
	var gotClient string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotClient = r.Header.Get("X-Spoo-Client")
		w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	c := NewClient(WithBaseURL(srv.URL), WithClientTag("cli/1.4.0"))
	if err := c.do(context.Background(), http.MethodGet, "/auth/me", nil, nil, nil); err != nil {
		t.Fatal(err)
	}
	if gotClient != "cli/1.4.0" {
		t.Fatalf("X-Spoo-Client = %q, want cli/1.4.0", gotClient)
	}
}

// A caller-supplied http.Client must still get the cross-host
// X-Spoo-Client strip, and its own CheckRedirect must keep running.
func TestWithHTTPClientKeepsRedirectStrip(t *testing.T) {
	gotClient := "unset"
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotClient = r.Header.Get("X-Spoo-Client")
		w.Write([]byte(`{}`))
	}))
	defer target.Close()

	redirector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL+"/final", http.StatusFound)
	}))
	defer redirector.Close()

	var customRan bool
	hc := &http.Client{
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			customRan = true
			return nil
		},
	}
	c := NewClient(WithBaseURL(redirector.URL), WithHTTPClient(hc))
	if err := c.do(context.Background(), http.MethodGet, "/start", nil, nil, nil); err != nil {
		t.Fatal(err)
	}
	if gotClient != "" {
		t.Fatalf("X-Spoo-Client forwarded cross-origin = %q, want empty", gotClient)
	}
	if !customRan {
		t.Fatal("the caller's CheckRedirect was not chained")
	}
	if hc.CheckRedirect == nil {
		t.Fatal("caller's client was mutated") // we must copy, not overwrite
	}
}
