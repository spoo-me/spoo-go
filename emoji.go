package spoo

import (
	"context"
	"encoding/json"
	"net/http"
)

// EmojiEntry is one emoji a user may choose for an alias.
type EmojiEntry struct {
	// Char is the raw canonical emoji character (no U+FE0F variation
	// selector), matching how aliases are stored and echoed.
	Char string `json:"c"`
	// Name is the lowercased human-readable name, the primary search
	// key (e.g. "rocket").
	Name string `json:"n"`
	// Group is the Unicode category display name (e.g. "Smileys &
	// Emotion").
	Group string `json:"g"`
	// Generated reports whether the emoji is in the auto-generation
	// pool.
	Generated bool `json:"gen"`
	// Keywords are extra search aliases, when the source lists any.
	Keywords []string `json:"k"`
}

// EmojiSet is the alias emoji policy plus every choosable emoji.
type EmojiSet struct {
	// AcceptMaxVersion is the newest Unicode emoji version a custom
	// alias may use.
	AcceptMaxVersion float64 `json:"accept_max_version"`
	// GenerateMaxVersion caps auto-generated aliases (lower, for older
	// platform coverage).
	GenerateMaxVersion float64 `json:"generate_max_version"`
	// MaxGraphemes is the most emoji graphemes allowed in one alias.
	MaxGraphemes int          `json:"max_graphemes"`
	Emoji        []EmojiEntry `json:"emoji"`
}

// EmojiSet fetches the emoji alias policy and pool. The payload is
// large and changes rarely, so the client revalidates with
// If-None-Match and serves its cached copy on 304 responses.
func (c *Client) EmojiSet(ctx context.Context) (*EmojiSet, error) {
	c.emojiMu.Lock()
	etag, cached := c.emojiETag, c.emojiCache
	c.emojiMu.Unlock()

	var extra http.Header
	if etag != "" && cached != nil {
		extra = http.Header{"If-None-Match": {etag}}
	}
	creds, err := c.credentials(ctx)
	if err != nil {
		return nil, err
	}
	resp, err := c.send(ctx, http.MethodGet, "/api/v1/emoji-set", nil, nil, creds, extra)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotModified {
		return cached, nil
	}
	if resp.StatusCode >= 400 {
		return nil, newError(resp)
	}
	var out EmojiSet
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	if newTag := resp.Header.Get("ETag"); newTag != "" {
		c.emojiMu.Lock()
		c.emojiETag, c.emojiCache = newTag, &out
		c.emojiMu.Unlock()
	}
	return &out, nil
}
