//go:build smoke

package spoo_test

// Scheduled production smoke: create a link, read its stats, fetch the emoji
// catalogue, delete the link. Runs only with -tags smoke and a dedicated
// low-scope key in SPOO_SMOKE_API_KEY; it is not part of the offline suite.

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	spoo "github.com/spoo-me/spoo-go"
	"github.com/spoo-me/spoo-go/option"
)

func TestProdSmoke(t *testing.T) {
	key := os.Getenv("SPOO_SMOKE_API_KEY")
	if key == "" {
		t.Skip("SPOO_SMOKE_API_KEY not set")
	}
	client := spoo.NewClient(
		option.WithAPIKey(key),
		option.WithClientTag("sdk-go-smoke"),
	)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	link, err := client.Shorten(ctx, spoo.ShortenRequest{
		LongURL: fmt.Sprintf("https://example.com/?smoke=%d", time.Now().UnixNano()),
	})
	if err != nil {
		t.Fatalf("shorten: %v", err)
	}
	defer func() {
		if err := client.DeleteURL(ctx, link.ID); err != nil {
			t.Errorf("cleanup delete: %v", err)
		}
	}()
	if link.ShortURL == "" {
		t.Fatal("shorten: empty short_url")
	}

	stats, err := client.LinkStats(ctx, link.ID, spoo.StatsQuery{GroupBy: []string{"time"}})
	if err != nil {
		t.Fatalf("link stats: %v", err)
	}
	if stats.Summary.TotalClicks != 0 {
		t.Fatalf("link stats: expected 0 clicks on a fresh link, got %d", stats.Summary.TotalClicks)
	}

	set, err := client.EmojiSet(ctx)
	if err != nil {
		t.Fatalf("emoji set: %v", err)
	}
	if len(set.Emoji) == 0 {
		t.Fatal("emoji set: empty catalogue")
	}
}
