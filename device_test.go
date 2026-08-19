package spoo

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/spoo-me/spoo-go/option"
)

var verifierRe = regexp.MustCompile(`^[A-Za-z0-9_-]{43}$`)

// RFC 7636 Appendix B test vector.
func TestCodeChallengeS256Vector(t *testing.T) {
	const (
		verifier = "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
		want     = "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM"
	)
	if got := CodeChallengeS256(verifier); got != want {
		t.Fatalf("CodeChallengeS256 = %q, want %q", got, want)
	}
}

func TestGenerateCodeVerifierShape(t *testing.T) {
	a, err := GenerateCodeVerifier()
	if err != nil {
		t.Fatal(err)
	}
	b, err := GenerateCodeVerifier()
	if err != nil {
		t.Fatal(err)
	}
	if !verifierRe.MatchString(a) {
		t.Fatalf("verifier = %q, want 43 base64url chars", a)
	}
	if a == b {
		t.Fatal("verifiers must be random")
	}
}

func TestDeviceAuthURL(t *testing.T) {
	c := NewClient(option.WithBaseURL("https://spoo.example"))
	verifier, err := GenerateCodeVerifier()
	if err != nil {
		t.Fatal(err)
	}
	raw := c.DeviceAuthURL(DeviceAuthParams{
		AppID:         "my-app",
		RedirectURI:   "http://127.0.0.1:53682/callback",
		State:         "st4te",
		CodeChallenge: CodeChallengeS256(verifier),
	})
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if u.Path != "/auth/device/login" {
		t.Errorf("path = %q", u.Path)
	}
	q := u.Query()
	if q.Get("app_id") != "my-app" || q.Get("state") != "st4te" {
		t.Errorf("query = %v", q)
	}
	if q.Get("redirect_uri") != "http://127.0.0.1:53682/callback" {
		t.Errorf("redirect_uri = %q", q.Get("redirect_uri"))
	}
	if q.Get("code_challenge_method") != "S256" {
		t.Errorf("code_challenge_method = %q", q.Get("code_challenge_method"))
	}
	if got := q.Get("code_challenge"); got != CodeChallengeS256(verifier) {
		t.Errorf("code_challenge = %q does not match S256(verifier)", got)
	}
}

// An empty RedirectURI defers to the app's registered default and stays
// off the URL entirely.
func TestDeviceAuthURLOmitsEmptyRedirectURI(t *testing.T) {
	c := NewClient(option.WithBaseURL("https://spoo.example"))
	raw := c.DeviceAuthURL(DeviceAuthParams{
		AppID:         "my-app",
		State:         "st4te",
		CodeChallenge: CodeChallengeS256("dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"),
	})
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if u.Query().Has("redirect_uri") {
		t.Fatalf("redirect_uri must be omitted when empty: %s", raw)
	}
}

// ForceRefresh rotates on demand, persisting through the TokenSource.
func TestForceRefresh(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/auth/device/refresh" {
			t.Errorf("path = %s", r.URL.Path)
		}
		w.Write([]byte(`{"access_token":"newAT","refresh_token":"newRT"}`))
	}))
	defer srv.Close()

	source := StaticTokens("oldAT", "oldRT")
	c := NewClient(option.WithBaseURL(srv.URL), option.WithTokenSource(source))
	creds, err := c.ForceRefresh(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if creds.AccessToken != "newAT" {
		t.Fatalf("creds = %+v", creds)
	}
	persisted, err := source.Token(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if persisted.RefreshToken != "newRT" {
		t.Fatalf("persisted = %+v", persisted)
	}
}

func TestForceRefreshWithoutSource(t *testing.T) {
	c := NewClient(option.WithAPIKey("spoo_key"))
	if _, err := c.ForceRefresh(context.Background()); !errors.Is(err, ErrTokenSourceRequired) {
		t.Fatalf("err = %v, want ErrTokenSourceRequired", err)
	}
}

func TestExchangeDeviceCode(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/auth/device/token" || r.Method != http.MethodPost {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode body: %v", err)
		}
		if body["app_id"] != "my-app" {
			t.Errorf("app_id = %q, want my-app", body["app_id"])
		}
		if body["code"] != "onetimecode" {
			t.Errorf("code = %q, want onetimecode", body["code"])
		}
		if body["code_verifier"] != "theverifier" {
			t.Errorf("code_verifier = %q, want theverifier", body["code_verifier"])
		}
		w.Write([]byte(`{"access_token":"at","refresh_token":"rt","user":{"id":"1","email":"a@b.c","email_verified":true,"name":"A","plan":"free"}}`))
	}))
	defer srv.Close()

	c := NewClient(option.WithBaseURL(srv.URL))
	tok, err := c.ExchangeDeviceCode(context.Background(), "my-app", "onetimecode", "theverifier")
	if err != nil {
		t.Fatal(err)
	}
	if tok.AccessToken != "at" || tok.User.Email != "a@b.c" {
		t.Fatalf("unexpected: %+v", tok)
	}
}

func TestDeviceCallsRequireAppID(t *testing.T) {
	c := NewClient()
	if _, err := c.ExchangeDeviceCode(context.Background(), "", "code", "verifier"); err == nil {
		t.Fatal("ExchangeDeviceCode must reject an empty app_id")
	}
	if _, err := c.RefreshTokens(context.Background(), "", "rt"); err == nil {
		t.Fatal("RefreshTokens must reject an empty app_id")
	}
}

func TestRefreshTokensSendsAppIDAndRotates(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/auth/device/refresh" || r.Method != http.MethodPost {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if auth := r.Header.Get("Authorization"); auth != "" {
			t.Errorf("Authorization = %q, want none (the refresh token is the credential)", auth)
		}
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode body: %v", err)
		}
		if body["app_id"] != "my-app" || body["refresh_token"] != "rt1" {
			t.Errorf("body = %v", body)
		}
		w.Write([]byte(`{"access_token":"at2","refresh_token":"rt2"}`))
	}))
	defer srv.Close()

	c := NewClient(option.WithBaseURL(srv.URL), option.WithTokenSource(StaticTokens("at1", "rt1")))
	pair, err := c.RefreshTokens(context.Background(), "my-app", "rt1")
	if err != nil {
		t.Fatal(err)
	}
	if pair.AccessToken != "at2" || pair.RefreshToken != "rt2" {
		t.Fatalf("pair = %+v", pair)
	}
}

// The CLI had a concurrent-refresh race that could spend a rotating
// refresh token twice and persist a dead pair. Under a stampede of
// 401s, exactly one refresh may reach the wire.
func TestConcurrent401sRefreshOnce(t *testing.T) {
	var refreshCalls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/auth/device/refresh":
			var body map[string]string
			_ = json.NewDecoder(r.Body).Decode(&body)
			if refreshCalls.Add(1) > 1 || body["refresh_token"] != "oldRT" {
				// A second spend of a rotated token is exactly the
				// bug: fail the way the backend would.
				w.WriteHeader(http.StatusUnauthorized)
				w.Write([]byte(`{"error":"invalid refresh token","code":"authentication_error"}`))
				return
			}
			time.Sleep(50 * time.Millisecond) // widen the race window
			w.Write([]byte(`{"access_token":"newAT","refresh_token":"newRT"}`))
		case "/auth/me":
			if r.Header.Get("Authorization") == "Bearer newAT" {
				w.Write([]byte(`{"user":{"id":"1"}}`))
				return
			}
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`{"error":"token expired","code":"authentication_error"}`))
		}
	}))
	defer srv.Close()

	source := StaticTokens("staleAT", "oldRT")
	c := NewClient(option.WithBaseURL(srv.URL), option.WithTokenSource(source))

	const goroutines = 10
	errs := make([]error, goroutines)
	var wg sync.WaitGroup
	for i := range goroutines {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, errs[i] = c.Me(context.Background())
		}()
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("goroutine %d: %v", i, err)
		}
	}
	if refreshCalls.Load() != 1 {
		t.Fatalf("refresh reached the wire %d times, want 1", refreshCalls.Load())
	}
	got, err := source.Token(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got.AccessToken != "newAT" || got.RefreshToken != "newRT" {
		t.Fatalf("persisted pair = %+v, want the rotated one", got)
	}
}
