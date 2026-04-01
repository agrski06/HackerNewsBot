package bot

import (
	"context"
	"log/slog"
	"strconv"
	"sync"
	"time"

	"HackerNewsBot/internal/commands"
	"HackerNewsBot/internal/config"
	"HackerNewsBot/internal/filter"
	"HackerNewsBot/internal/hackernews"
	"HackerNewsBot/internal/scheduler"
	"HackerNewsBot/internal/store"
	"HackerNewsBot/internal/telegram"
	"HackerNewsBot/internal/telegraph"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

const (
	// How many top story IDs to consider each run.
	topNIDs = 30
	// How old entries can be before pruning.
	pruneAge = 7 * 24 * time.Hour
)

// Bot orchestrates the fetch → filter → send pipeline,
// listens for Telegram commands, and supports scheduled delivery.
type Bot struct {
	cfg       *config.Config
	hn        *hackernews.Client
	store     store.Store
	sender    *telegram.Sender
	telegraph *telegraph.Client

	cmdHandler *commands.Handler
	loc        *time.Location

	// scheduleChanged is signaled when /schedule command updates the schedule.
	scheduleChanged chan struct{}

	// fetchNow is signaled when /fetch command is used.
	fetchNow chan struct{}

	mu sync.Mutex // protects runtime config reads during tick
}

// New creates a new Bot.
func New(cfg *config.Config, hn *hackernews.Client, st store.Store, sender *telegram.Sender, tg *telegraph.Client) *Bot {
	loc, err := time.LoadLocation(cfg.Timezone)
	if err != nil {
		slog.Warn("invalid timezone, falling back to UTC", "timezone", cfg.Timezone, "error", err)
		loc = time.UTC
	}

	b := &Bot{
		cfg:             cfg,
		hn:              hn,
		store:           st,
		sender:          sender,
		telegraph:       tg,
		loc:             loc,
		scheduleChanged: make(chan struct{}, 1),
		fetchNow:        make(chan struct{}, 1),
	}

	// Set up command handler.
	b.cmdHandler = commands.NewHandler(
		sender.BotAPI(),
		cfg.OwnerUserID,
		st,
		func() {
			// /fetch — signal the main loop.
			select {
			case b.fetchNow <- struct{}{}:
			default:
			}
		},
		func() {
			// /schedule changed — signal the main loop to recalculate timer.
			select {
			case b.scheduleChanged <- struct{}{}:
			default:
			}
		},
	)

	// Seed schedule from env if not already stored.
	if cfg.Schedule != "" {
		if _, found := st.GetConfig(commands.KeySchedule); !found {
			if _, err := scheduler.ParseSchedule(cfg.Schedule); err == nil {
				st.SetConfig(commands.KeySchedule, cfg.Schedule)
			}
		}
	}

	return b
}

// Run starts the main loop. It blocks until ctx is cancelled.
func (b *Bot) Run(ctx context.Context) {
	slog.Info("bot starting",
		"score_threshold", b.getScoreThreshold(),
		"max_stories", b.getMaxStories(),
		"digest_mode", b.getDigestMode(),
		"timezone", b.loc.String(),
	)

	// Start command listener in background.
	go b.listenCommands(ctx)

	// Main scheduling loop.
	for {
		slots := b.getScheduleSlots()

		if len(slots) > 0 {
			// Schedule mode: wait until next fire time.
			next := scheduler.NextFire(time.Now(), slots, b.loc)
			wait := time.Until(next)
			slog.Info("schedule mode: next delivery", "at", next.In(b.loc).Format("15:04"), "in", wait.Round(time.Second))

			timer := time.NewTimer(wait)
			select {
			case <-ctx.Done():
				timer.Stop()
				slog.Info("bot shutting down")
				return
			case <-timer.C:
				if !b.isPaused() {
					b.tick(ctx)
				} else {
					slog.Info("skipping scheduled tick — bot is paused")
				}
			case <-b.fetchNow:
				timer.Stop()
				slog.Info("manual fetch triggered")
				b.tick(ctx)
			case <-b.scheduleChanged:
				timer.Stop()
				slog.Info("schedule changed, recalculating")
				// Loop will recalculate.
			}
		} else {
			// Interval mode: use fixed ticker.
			slog.Info("interval mode", "interval", b.cfg.HNFetchInterval)

			// Run once immediately.
			if !b.isPaused() {
				b.tick(ctx)
			}

			ticker := time.NewTicker(b.cfg.HNFetchInterval)
			intervalLoop:
			for {
				select {
				case <-ctx.Done():
					ticker.Stop()
					slog.Info("bot shutting down")
					return
				case <-ticker.C:
					if !b.isPaused() {
						b.tick(ctx)
					}
				case <-b.fetchNow:
					slog.Info("manual fetch triggered")
					b.tick(ctx)
				case <-b.scheduleChanged:
					ticker.Stop()
					slog.Info("schedule changed, switching modes")
					break intervalLoop
				}
			}
		}
	}
}

// listenCommands starts long polling for incoming Telegram messages.
func (b *Bot) listenCommands(ctx context.Context) {
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 30

	updates := b.sender.BotAPI().GetUpdatesChan(u)

	for {
		select {
		case <-ctx.Done():
			return
		case update := <-updates:
			if update.Message != nil {
				b.cmdHandler.Handle(update.Message)
			}
		}
	}
}

func (b *Bot) tick(ctx context.Context) {
	slog.Info("starting fetch cycle")

	// Fetch top story IDs.
	ids, err := b.hn.TopStoryIDs(ctx)
	if err != nil {
		slog.Error("failed to fetch top story IDs", "error", err)
		return
	}

	if len(ids) > topNIDs {
		ids = ids[:topNIDs]
	}

	slog.Info("fetched top story IDs", "count", len(ids))

	// Fetch item details concurrently.
	items, err := b.hn.GetItems(ctx, ids)
	if err != nil {
		slog.Error("failed to fetch items", "error", err)
		return
	}

	slog.Info("fetched items", "count", len(items))

	// Filter with runtime overrides.
	scoreThreshold := b.getScoreThreshold()
	maxStories := b.getMaxStories()
	filtered := filter.Filter(items, b.store, scoreThreshold, maxStories)
	if len(filtered) == 0 {
		slog.Info("no new stories to send")
		return
	}

	slog.Info("filtered stories", "count", len(filtered))

	// Generate Telegraph discussion pages (if enabled).
	telegraphURLs := b.generateTelegraphPages(ctx, filtered)

	// Send.
	digestMode := b.getDigestMode()
	if digestMode {
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
			time.Sleep(500 * time.Millisecond)
		}
	}

	// Mark as seen.
	for _, item := range filtered {
		if err := b.store.MarkSeen(item.ID); err != nil {
			slog.Error("failed to mark item as seen", "id", item.ID, "error", err)
		}
	}

	// Prune old entries.
	if err := b.store.Prune(pruneAge); err != nil {
		slog.Error("failed to prune store", "error", err)
	}

	slog.Info("fetch cycle complete")
}

// ── Runtime config helpers (BoltDB overrides > env defaults) ──

func (b *Bot) getScheduleSlots() []scheduler.TimeSlot {
	raw, _ := b.store.GetConfig(commands.KeySchedule)
	if raw == "" {
		return nil
	}
	slots, err := scheduler.ParseSchedule(raw)
	if err != nil {
		slog.Warn("invalid stored schedule, ignoring", "value", raw, "error", err)
		return nil
	}
	return slots
}

func (b *Bot) getScoreThreshold() int {
	if val, ok := b.store.GetConfig(commands.KeyScoreThreshold); ok {
		if n, err := strconv.Atoi(val); err == nil {
			return n
		}
	}
	return b.cfg.ScoreThreshold
}

func (b *Bot) getMaxStories() int {
	if val, ok := b.store.GetConfig(commands.KeyMaxStories); ok {
		if n, err := strconv.Atoi(val); err == nil {
			return n
		}
	}
	return b.cfg.MaxStoriesPerRun
}

func (b *Bot) getDigestMode() bool {
	if val, ok := b.store.GetConfig(commands.KeyDigestMode); ok {
		return val == "true"
	}
	return b.cfg.DigestMode
}

func (b *Bot) isPaused() bool {
	val, _ := b.store.GetConfig(commands.KeyPaused)
	return val == "true"
}

// generateTelegraphPages creates Telegraph Instant View pages for each story's
// discussion. Returns a map of item index → Telegraph URL.
func (b *Bot) generateTelegraphPages(ctx context.Context, items []*hackernews.Item) map[int]string {
	urls := make(map[int]string)

	if b.telegraph == nil || !b.cfg.TelegraphEnabled {
		return urls
	}

	for i, item := range items {
		if item.Descendants == 0 {
			slog.Debug("skipping telegraph for story with no comments", "id", item.ID)
			continue
		}

		comments, err := b.hn.GetCommentTree(ctx, item, b.cfg.MaxTopComments, b.cfg.MaxCommentDepth)
		if err != nil {
			slog.Warn("failed to fetch comments for telegraph", "id", item.ID, "error", err)
			continue
		}

		if len(comments) == 0 {
			continue
		}

		nodes := telegraph.RenderDiscussion(item, comments)

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

