# spoo.me Go SDK

The official Go SDK for the [spoo.me](https://spoo.me) link management API.

```go
import (
    spoo "github.com/spoo-me/spoo-go"
    "github.com/spoo-me/spoo-go/option"
)

client := spoo.NewClient(option.WithAPIKey(os.Getenv("SPOO_API_KEY")))

link, err := client.Shorten(ctx, spoo.ShortenRequest{
    LongURL: "https://example.com/launch",
})
fmt.Println(link.ShortURL) // https://spoo.me/xyz
```

- Zero dependencies, standard library only
- Typed errors, automatic retries, range-over-func pagination
- Timestamps in and out as `time.Time`, whatever the wire format
- Anonymous, API key, and Sign in with Spoo authentication

## Install

```sh
go get github.com/spoo-me/spoo-go
```

Requires Go 1.24 or newer.

## Authentication

Create an API key from your [spoo.me dashboard](https://spoo.me) and pass it
explicitly:

```go
client := spoo.NewClient(option.WithAPIKey(os.Getenv("SPOO_API_KEY")))
```

Constructing without credentials is valid too: anonymous shortening and the
public endpoints (stats, previews, the emoji set) work without an account.

Self-hosting spoo.me? Point the client at your instance:

```go
client := spoo.NewClient(option.WithBaseURL("https://links.example.com"))
```

Apps built on the SDK should set their own attribution tag:

```go
client := spoo.NewClient(option.WithClientTag("my-app/1.0.0"))
```

## Shorten links

```go
link, err := client.Shorten(ctx, spoo.ShortenRequest{
    LongURL:     "https://example.com/launch",
    Alias:       "launch", // or emoji: "🚀🔥"
    Password:    "optional-password",
    MaxClicks:   10_000,
    ExpireAfter: time.Now().AddDate(0, 1, 0),
})
```

Anonymous creations return a one-time `ClaimToken`. Store it and the link can
be claimed into an account later:

```go
outcome, err := client.ClaimURLs(ctx, []spoo.Claim{
    {URLID: link.ID, Token: link.ClaimToken},
})
```

## Manage links

```go
page, err := client.ListURLs(ctx, spoo.ListURLsOptions{PageSize: 50})

// or iterate everything; pages are fetched lazily
for link, err := range client.ListURLsAll(ctx, spoo.ListURLsOptions{}) {
    if err != nil {
        return err
    }
    fmt.Println(link.Alias, link.TotalClicks)
}
```

Updates distinguish "leave it alone" from "clear it". Omitted fields keep
their current setting, `spoo.Null` clears one, `spoo.Set` replaces it:

```go
updated, err := client.UpdateURL(ctx, link.ID, spoo.UpdateURLParams{
    LongURL:     "https://example.com/moved",
    Password:    spoo.Null[string](),                   // remove protection
    MaxClicks:   spoo.Set(500),                         // replace the limit
    ExpireAfter: spoo.Set(time.Now().AddDate(1, 0, 0)), // replace the expiry
})
```

Bulk operations take up to 100 ids and always answer with per-item results,
never a batch-level failure:

```go
res, err := client.BulkUpdateStatus(ctx, ids, "INACTIVE")
if err != nil {
    return err // request-level failure only
}
for _, row := range res.Results {
    if !row.OK {
        log.Printf("%s: %s (%s)", row.ID, row.Error, row.ErrorCode)
    }
}
```

`BulkDelete`, `BulkUpdateExpiry`, and `BulkMoveDomain` follow the same shape.

## Stats and exports

```go
stats, err := client.Stats(ctx, spoo.StatsQuery{
    StartDate: time.Now().AddDate(0, 0, -30),
    GroupBy:   []string{"time", "browser", "country"},
    Timezone:  "UTC",
})

// one link, addressed by alias
stats, err = client.StatsByAlias(ctx, "launch", "spoo.me", spoo.StatsQuery{})

// anyone's public stats, no auth; the result pairs link facts with stats
public, err := client.PublicStats(ctx, "launch", spoo.PublicStatsQuery{})
fmt.Println(public.Link.Status, public.Stats.Summary.TotalClicks)

// password-protected stats: the password travels in a POST body, never
// the query string
public, err = client.PublicStats(ctx, "secret", spoo.PublicStatsQuery{
    Password: "the-link-password",
})
```

Without explicit dates the API returns only the last 7 days; request up to
`spoo.MaxRangeDays` (90) explicitly for more.

Exports stream and carry the server-suggested filename. The csv format
arrives as a ZIP archive with one CSV per dimension:

```go
file, err := client.Export(ctx, spoo.StatsQuery{}, "xlsx")
if err != nil {
    return err
}
defer file.Body.Close()

out, err := os.Create(file.Filename)
if err != nil {
    return err
}
defer out.Close()
_, err = io.Copy(out, file.Body)
```

## Errors

Failed calls return `*spoo.Error` with the parsed envelope and response
metadata. Retrieve it with `errors.As`:

| What you get | Where |
| --- | --- |
| HTTP status | `err.StatusCode` |
| Machine-readable code | `err.Code` (lowercase snake_case, e.g. `conflict`, `not_found`, `blocked`; the one uppercase outlier is `EMAIL_NOT_VERIFIED`) |
| Human-readable message | `err.Message`, plus `err.Field` on validation errors |
| Request id for support | `err.RequestID` |
| Rate-limit state | `err.RateLimit` (limit, remaining, reset, retry-after) |

Common branches have predicates and sentinels:

| Helper | Meaning |
| --- | --- |
| `spoo.IsNotFound(err)` | 404: no such resource, or not yours |
| `spoo.IsRateLimited(err)` | 429: budget exhausted even after retries |
| `errors.Is(err, spoo.ErrSessionExpired)` | the refresh token no longer works; log in again |
| `errors.Is(err, spoo.ErrLinkPasswordProtected)` | the link's stats need the link password |
| `spoo.IsBlocked(err)` | 451: the link was taken down by the safety pipeline |

## Retries

Idempotent requests (GET, PUT, DELETE) are retried twice by default on
connection errors and 408, 429, 500, 502, 503 and 504 responses, with
exponential backoff and jitter. Requests that are not idempotent are only
retried when the server provably did no work (429 and 503). A `Retry-After`
header is authoritative when the server sends one. Configure with
`option.WithMaxRetries(n)`; 0 disables retries.

## Pagination

`ListURLs` is the manual page: check `HasNext` and bump `Page`. `ListURLsAll`
is the auto-pager, a lazy `iter.Seq2[URLItem, error]` that stops cleanly on
`break` and yields any fetch error once.

## Sign in with Spoo

Connected apps authenticate users through the device flow. The SDK ships the
protocol pieces; your app owns the browser, the callback listener, and token
storage:

```go
verifier, _ := spoo.GenerateCodeVerifier()
state, _ := spoo.GenerateState()

authURL := client.DeviceAuthURL(spoo.DeviceAuthParams{
    AppID:         "my-app",
    RedirectURI:   "http://127.0.0.1:53682/callback",
    State:         state,
    CodeChallenge: spoo.CodeChallengeS256(verifier),
})
// open authURL in a browser; the callback delivers ?code=...&state=...

tokens, err := client.ExchangeDeviceCode(ctx, "my-app", code, verifier)
```

Every app needs its own registration: redirect URIs match exactly against
the registered list, and the app id is a required argument everywhere.

Hand the pair to a client and refresh happens automatically, including under
concurrency (one refresh at a time, rotation persisted through your
`TokenSource`):

```go
client := spoo.NewClient(
    option.WithTokenSource(spoo.StaticTokens(tokens.AccessToken, tokens.RefreshToken)),
)
```

`StaticTokens` keeps rotations in memory only. For anything that outlives the
process, implement the two-method `TokenSource` interface over your keyring,
file, or database, and rotated tokens persist through it.

## API coverage

| Method | Endpoint |
| --- | --- |
| `Shorten`, `CheckAlias` | `POST /api/v1/shorten`, `GET /api/v1/shorten/check-alias` |
| `ListURLs`, `ListURLsAll` | `GET /api/v1/urls` |
| `GetURL`, `ResolveAlias` | `GET /api/v1/urls/{id}`, `GET /api/v1/urls/{domain}/{alias}` |
| `UpdateURL`, `SetURLStatus` | `PATCH /api/v1/urls/{id}`, `PATCH /api/v1/urls/{id}/status` |
| `DeleteURL`, `DeleteURLsByDomain` | `DELETE /api/v1/urls/{id}`, `DELETE /api/v1/urls?domain=` |
| `ClaimURLs` | `POST /api/v1/urls/claim` |
| `BulkDelete`, `BulkUpdateStatus`, `BulkUpdateExpiry`, `BulkMoveDomain` | `POST /api/v1/urls/bulk/*` |
| `Stats`, `LinkStats`, `StatsByAlias` | `GET /api/v1/stats`, `GET /api/v1/stats/links/{id}` |
| `PublicStats`, `PublicPreview` | `GET or POST /api/v1/public/stats/{code}`, `GET /api/v1/public/preview/{code}` |
| `Export`, `ExportLink` | `GET /api/v1/export`, `GET /api/v1/export/links/{id}` |
| `EmojiSet` | `GET /api/v1/emoji-set` (ETag-cached) |
| `Me` | `GET /auth/me` |
| `ExchangeDeviceCode`, `RefreshTokens`, `DeviceAuthURL` | `POST /auth/device/token`, `POST /auth/device/refresh` |

## License

AGPL-3.0. See [LICENSE](LICENSE).
