// Package spoo is the official Go client for the spoo.me URL shortener
// API.
//
// The package covers the v1 HTTP API: shortening, link management,
// claiming, bulk operations, stats, exports, public previews, the emoji
// alias policy, and the connected-apps device flow. It has no
// dependencies outside the standard library.
//
// # Quickstart
//
// Construction options live in the option subpackage
// (github.com/spoo-me/spoo-go/option):
//
//	client := spoo.NewClient(
//		option.WithAPIKey(os.Getenv("SPOO_API_KEY")),
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
// Three modes are supported, all optional. Anonymous calls are
// legitimate for the public endpoints (shorten, public stats and
// previews, the emoji set):
//
//   - option.WithAPIKey: a spoo_... API key (CI, servers).
//   - option.WithTokenSource(spoo.StaticTokens(access, refresh)): a
//     JWT pair from the connected-apps device flow. Refresh and
//     rotation are handled automatically; implement your own
//     [TokenSource] to persist rotated tokens (a keyring, a file, a
//     database).
//   - Nothing: anonymous.
//
// # Errors
//
// Failed API calls return *[Error] carrying the backend's error
// envelope, the HTTP status, the request id, and parsed rate-limit
// headers. Use [errors.As], the [IsNotFound] and [IsRateLimited]
// predicates, or the [ErrSessionExpired] and [ErrLinkPasswordProtected]
// sentinels:
//
//	if _, err := client.ResolveAlias(ctx, "launch", "spoo.me"); spoo.IsNotFound(err) {
//		// no such link, or not yours
//	}
//
// # Timestamps
//
// The wire mixes Unix seconds and ISO 8601 strings per endpoint.
// Response timestamps normalize to [Timestamp] (an embedded time.Time),
// and request fields take plain time.Time.
//
// # Retries
//
// Transient failures (connection errors, 408, 429, 5xx) are retried
// twice by default with exponential backoff and jitter, honoring
// Retry-After. Tune with option.WithMaxRetries.
//
// # Pagination
//
// [Client.ListURLs] returns one page with HasNext; [Client.ListURLsAll]
// returns an iter.Seq2 that pages lazily:
//
//	for link, err := range client.ListURLsAll(ctx, spoo.ListURLsOptions{}) {
//		if err != nil {
//			return err
//		}
//		fmt.Println(link.Alias, link.TotalClicks)
//	}
package spoo
