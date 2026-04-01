package bot

import (
	"context"
	"log/slog"
	"time"

	"HackerNewsBot/internal/config"
	"HackerNewsBot/internal/filter"
	"HackerNewsBot/internal/hackernews"
	"HackerNewsBot/internal/store"
	"HackerNewsBot/internal/telegram"
	"HackerNewsBot/internal/telegraph"
)

const (
	// How many top story IDs to consider each run
	topNIDs = 30
	// How old entries can be before pruning.
	pruneAge = 7 * 24 * time.Hour
)

// Bot orchestrates the fetch → filter → send pipeline
type Bot struct {
	cfg       *config.Config
	hn        *hackernews.Client
	store     store.Store
	sender    *telegram.Sender
	telegraph *telegraph.Client // nil if Telegraph is disabled
}

// New creates a new Bot
func New(cfg *config.Config, hn *hackernews.Client, st store.Store, sender *telegram.Sender, tg *telegraph.Client) *Bot {
	return &Bot{
		cfg:       cfg,
		hn:        hn,
		store:     st,
		sender:    sender,
		telegraph: tg,
	}
}

// Run starts the main loop. It blocks until ctx is canceled
func (b *Bot) Run(ctx context.Context) {
	slog.Info("bot starting",
		"interval", b.cfg.HNFetchInterval,
		"score_threshold", b.cfg.ScoreThreshold,
		"max_stories", b.cfg.MaxStoriesPerRun,
		"digest_mode", b.cfg.DigestMode,
	)

	// Run once immediately on startup
	b.tick(ctx)

	ticker := time.NewTicker(b.cfg.HNFetchInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			slog.Info("bot shutting down")
			return
		case <-ticker.C:
			b.tick(ctx)
		}
	}
}

func (b *Bot) tick(ctx context.Context) {
	slog.Info("starting fetch cycle")

	// Fetch top story IDs
	ids, err := b.hn.TopStoryIDs(ctx)
	if err != nil {
		slog.Error("failed to fetch top story IDs", "error", err)
		return
	}

	// Take only the top N
	if len(ids) > topNIDs {
		ids = ids[:topNIDs]
	}

	slog.Info("fetched top story IDs", "count", len(ids))

	// Fetch item details concurrently
	items, err := b.hn.GetItems(ctx, ids)
	if err != nil {
		slog.Error("failed to fetch items", "error", err)
		return
	}

	slog.Info("fetched items", "count", len(items))

	// Filter
	filtered := filter.Filter(items, b.store, b.cfg.ScoreThreshold, b.cfg.MaxStoriesPerRun)
	if len(filtered) == 0 {
		slog.Info("no new stories to send")
		return
	}

	slog.Info("filtered stories", "count", len(filtered))

	// Generate Telegraph discussion pages (if enabled).
	telegraphURLs := b.generateTelegraphPages(ctx, filtered)

	// Send.
	if b.cfg.DigestMode {
		if err := b.sender.SendDigest(filtered, telegraphURLs); err != nil {
			slog.Error("failed to send digest", "error", err)
			return
		}
		slog.Info("sent digest", "stories", len(filtered))
	} else {
		for i, item := range filtered {
			tURL := telegraphURLs[i]
			if err := b.sender.SendIndividual(item, tURL); err != nil {
				slog.Error("failed to send story", "id", item.ID, "title", item.Title, "error", err)
				continue
			}
			slog.Info("sent story", "id", item.ID, "title", item.Title, "score", item.Score, "telegraph", tURL != "")
			// Small delay between individual messages to avoid rate limits.
			time.Sleep(500 * time.Millisecond)
		}
	}

	// Mark as seen
	for _, item := range filtered {
		if err := b.store.MarkSeen(item.ID); err != nil {
			slog.Error("failed to mark item as seen", "id", item.ID, "error", err)
		}
	}

	// Prune old entries
	if err := b.store.Prune(pruneAge); err != nil {
		slog.Error("failed to prune store", "error", err)
	}

	slog.Info("fetch cycle complete")
}

// generateTelegraphPages creates Telegraph Instant View pages for each story's
// discussion. Returns a map of item index → Telegraph URL.
func (b *Bot) generateTelegraphPages(ctx context.Context, items []*hackernews.Item) map[int]string {
	urls := make(map[int]string)

	if b.telegraph == nil || !b.cfg.TelegraphEnabled {
		return urls
	}

	for i, item := range items {
		// Skip stories with no comments.
		if item.Descendants == 0 {
			slog.Debug("skipping telegraph for story with no comments", "id", item.ID)
			continue
		}

		// Fetch comment tree.
		comments, err := b.hn.GetCommentTree(ctx, item, b.cfg.MaxTopComments, b.cfg.MaxCommentDepth)
		if err != nil {
			slog.Warn("failed to fetch comments for telegraph", "id", item.ID, "error", err)
			continue
		}

		if len(comments) == 0 {
			continue
		}

		// Render to Telegraph nodes.
		nodes := telegraph.RenderDiscussion(item, comments)

		// Create Telegraph page.
		page, err := b.telegraph.CreatePage(item.Title+" — HN Discussion", "HackerNewsBot", nodes)
		if err != nil {
			slog.Warn("failed to create telegraph page", "id", item.ID, "error", err)
			continue
		}

		urls[i] = page.URL
		slog.Info("created telegraph page", "id", item.ID, "url", page.URL, "comments", len(comments))
	}

	return urls
}

