// Package spoo is the official Go client for the spoo.me URL shortener API.
//
// The package covers the v1 HTTP API: shortening, link management, stats,
// exports, API keys, and the connected-apps device flow. It has no
// dependencies outside the standard library.
//
// # Quickstart
//
//	client := spoo.NewClient(
//		spoo.WithAPIKey(os.Getenv("SPOO_API_KEY")),
//	)
//
//	link, err := client.Shorten(ctx, spoo.ShortenRequest{
//		LongURL: "https://example.com/very/long/url",
//	})
//	if err != nil {
//		log.Fatal(err)
//	}
//	fmt.Println(link.ShortURL)
//
// # Authentication
//
// Three modes are supported, all optional — anonymous calls are legitimate
// for the public endpoints (shorten, public stats):
//
//   - WithAPIKey: a spoo_... API key (CI, servers).
//   - WithTokenSource(StaticTokens(access, refresh)): a JWT pair from the
//     connected-apps device flow. Refresh and rotation are handled
//     automatically; implement your own [TokenSource] to persist rotated
//     tokens (a keyring, a file, a database).
//   - Nothing: anonymous.
//
// # Errors
//
// Failed API calls return *[Error] carrying the backend's error envelope,
// the HTTP status, the request id, and parsed rate-limit headers. Use
// [errors.As], the [IsNotFound] and [IsRateLimited] predicates, or the
// [ErrSessionExpired] and [ErrLinkPasswordProtected] sentinels:
//
//	if _, err := client.ResolveAlias(ctx, "launch", ""); spoo.IsNotFound(err) {
//		// no such link, or not yours
//	}
//
// # Retries
//
// Transient failures (connection errors, 408, 429, 5xx) are retried twice
// by default with exponential backoff and jitter, honoring Retry-After.
// Tune with [WithMaxRetries].
package spoo
