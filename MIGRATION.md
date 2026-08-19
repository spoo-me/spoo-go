# Migration guide

## v0.2 to v0.3

Client construction options moved from the root package to the new
`option` package, matching the layout of the major Go API SDKs. This is
the only change; everything else keeps its `spoo.*` name.

```go
import (
    spoo "github.com/spoo-me/spoo-go"
    "github.com/spoo-me/spoo-go/option"
)

client := spoo.NewClient(
    option.WithAPIKey(os.Getenv("SPOO_API_KEY")),
    option.WithMaxRetries(3),
)
```

Rename table:

| v0.2 | v0.3 |
| --- | --- |
| `spoo.WithAPIKey` | `option.WithAPIKey` |
| `spoo.WithTokenSource` | `option.WithTokenSource` |
| `spoo.WithBaseURL` | `option.WithBaseURL` |
| `spoo.WithHTTPClient` | `option.WithHTTPClient` |
| `spoo.WithMaxRetries` | `option.WithMaxRetries` |
| `spoo.WithClientTag` | `option.WithClientTag` |
| `spoo.Option` | `option.RequestOption` |

Unchanged: every resource method and response type, `spoo.Error` and
the sentinels and predicates, `spoo.Opt`, `spoo.Timestamp`,
`spoo.TokenSource` with `spoo.StaticAPIKey` and `spoo.StaticTokens`,
and the device-flow helpers. Custom `TokenSource` implementations keep
working as-is and are passed with `option.WithTokenSource(store)`.

## v0.1 to v0.2

`PublicStats` returns the full envelope instead of just the stats half:
the new `PublicStatsResult` pairs `Link` (alias, destination, status,
password protection) with `Stats`. Change `res.Summary` to
`res.Stats.Summary` and read the new link facts from `res.Link`.
