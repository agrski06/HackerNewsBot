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
)

const (
	// How many top story IDs to consider each run
	topNIDs = 30
	// How old entries can be before pruning.
	pruneAge = 7 * 24 * time.Hour
)

// Bot orchestrates the fetch → filter → send pipeline
type Bot struct {
	cfg    *config.Config
	hn     *hackernews.Client
	store  store.Store
	sender *telegram.Sender
}

// New creates a new Bot
func New(cfg *config.Config, hn *hackernews.Client, st store.Store, sender *telegram.Sender) *Bot {
	return &Bot{
		cfg:    cfg,
		hn:     hn,
		store:  st,
		sender: sender,
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

	// Send
	if b.cfg.DigestMode {
		if err := b.sender.SendDigest(filtered); err != nil {
			slog.Error("failed to send digest", "error", err)
			return
		}
		slog.Info("sent digest", "stories", len(filtered))
	} else {
		for _, item := range filtered {
			if err := b.sender.SendIndividual(item); err != nil {
				slog.Error("failed to send story", "id", item.ID, "title", item.Title, "error", err)
				continue
			}
			slog.Info("sent story", "id", item.ID, "title", item.Title, "score", item.Score)
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
