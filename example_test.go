package spoo_test

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"time"

	spoo "github.com/spoo-me/spoo-go"
)

// Construct a client for the hosted API with an API key. All options
// are optional: an empty option list gives an anonymous client for the
// public endpoints, and WithBaseURL points at a self-hosted instance.
func ExampleNewClient() {
	client := spoo.NewClient(
		spoo.WithAPIKey(os.Getenv("SPOO_API_KEY")),
		spoo.WithMaxRetries(3),
	)
	_ = client
}

// Shorten a link with an alias, a password, and an expiry.
func ExampleClient_Shorten() {
	client := spoo.NewClient(spoo.WithAPIKey(os.Getenv("SPOO_API_KEY")))

	link, err := client.Shorten(context.Background(), spoo.ShortenRequest{
		LongURL:     "https://example.com/launch",
		Alias:       "launch",
		MaxClicks:   10000,
		ExpireAfter: time.Now().AddDate(0, 1, 0),
	})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(link.ShortURL)
}

// Iterate every link in the account; pages are fetched lazily as the
// loop advances.
func ExampleClient_ListURLsAll() {
	client := spoo.NewClient(spoo.WithAPIKey(os.Getenv("SPOO_API_KEY")))

	for link, err := range client.ListURLsAll(context.Background(), spoo.ListURLsOptions{
		SortBy: "total_clicks",
	}) {
		if err != nil {
			log.Fatal(err)
		}
		fmt.Println(link.Alias, link.TotalClicks)
	}
}

// Patch a link. Omitted fields keep their current setting, spoo.Null
// clears one, and spoo.Set replaces it.
func ExampleClient_UpdateURL() {
	client := spoo.NewClient(spoo.WithAPIKey(os.Getenv("SPOO_API_KEY")))

	updated, err := client.UpdateURL(context.Background(), "507f1f77bcf86cd799439011", spoo.UpdateURLParams{
		LongURL:     "https://example.com/moved",
		Password:    spoo.Null[string](), // remove password protection
		ExpireAfter: spoo.Set(time.Now().AddDate(1, 0, 0)),
	})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(updated.Status)
}

// Branch on API errors with errors.As and the typed predicates.
func Example_errorHandling() {
	client := spoo.NewClient(spoo.WithAPIKey(os.Getenv("SPOO_API_KEY")))

	_, err := client.ResolveAlias(context.Background(), "launch", "spoo.me")
	switch {
	case err == nil:
		// found
	case spoo.IsNotFound(err):
		fmt.Println("no such link, or not yours")
	case spoo.IsRateLimited(err):
		var apiErr *spoo.Error
		errors.As(err, &apiErr)
		fmt.Println("retry after", apiErr.RateLimit.RetryAfter)
	default:
		var apiErr *spoo.Error
		if errors.As(err, &apiErr) {
			fmt.Println(apiErr.Code, apiErr.Message, apiErr.RequestID)
		}
	}
}

// The device flow (Sign in with Spoo) in an app: the SDK provides the
// protocol pieces and the app owns the browser, the callback listener,
// and token storage. Register your app to get an app id; redirect URIs
// match exactly against that registration.
func Example_deviceFlow() {
	client := spoo.NewClient()

	verifier, err := spoo.GenerateCodeVerifier()
	if err != nil {
		log.Fatal(err)
	}
	state, err := spoo.GenerateState()
	if err != nil {
		log.Fatal(err)
	}
	authURL := client.DeviceAuthURL(spoo.DeviceAuthParams{
		AppID:         "my-app",
		RedirectURI:   "http://127.0.0.1:53682/callback",
		State:         state,
		CodeChallenge: spoo.CodeChallengeS256(verifier),
	})
	fmt.Println("open in a browser:", authURL)

	// The app drives the browser and receives ?code=...&state=... on
	// its callback listener, verifying the state matches.
	code := "one-time-code-from-the-callback"

	tokens, err := client.ExchangeDeviceCode(context.Background(), "my-app", code, verifier)
	if err != nil {
		log.Fatal(err)
	}

	// Hand the pair to a client via a TokenSource; refresh and
	// rotation persistence happen automatically from here.
	authed := spoo.NewClient(
		spoo.WithTokenSource(spoo.StaticTokens(tokens.AccessToken, tokens.RefreshToken)),
	)
	_ = authed
}
