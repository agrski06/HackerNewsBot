package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"HackerNewsBot/internal/bot"
	"HackerNewsBot/internal/config"
	"HackerNewsBot/internal/hackernews"
	"HackerNewsBot/internal/store"
	"HackerNewsBot/internal/telegram"
)

func main() {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	// Set up structured logging
	setupLogging(cfg.LogLevel)

	slog.Info("config loaded",
		"fetch_interval", cfg.HNFetchInterval,
		"score_threshold", cfg.ScoreThreshold,
		"max_stories_per_run", cfg.MaxStoriesPerRun,
		"digest_mode", cfg.DigestMode,
		"silent_messages", cfg.SilentMessages,
		"db_path", cfg.DBPath,
	)

	// Open BoltDB store
	st, err := store.NewBoltStore(cfg.DBPath)
	if err != nil {
		slog.Error("failed to open store", "error", err)
		os.Exit(1)
	}
	defer st.Close()

	// Create HN client
	hn := hackernews.NewClient()

	// Create Telegram sender
	sender, err := telegram.NewSender(cfg.TelegramBotToken, cfg.TelegramChatID, cfg.SilentMessages, cfg.DisablePreview)
	if err != nil {
		slog.Error("failed to create telegram sender", "error", err)
		os.Exit(1)
	}

	// Start health check server (for Coolify monitoring)
	go func() {
		mux := http.NewServeMux()
		mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("ok"))
		})
		addr := fmt.Sprintf(":%s", cfg.HealthPort)
		slog.Info("health check server starting", "addr", addr)
		if err := http.ListenAndServe(addr, mux); err != nil {
			slog.Error("health check server failed", "error", err)
		}
	}()

	// Graceful shutdown on SIGINT/SIGTERM
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Create and run bot
	b := bot.New(cfg, hn, st, sender)
	b.Run(ctx)

	slog.Info("shutdown complete")
}

func setupLogging(level string) {
	var l slog.Level
	switch level {
	case "debug":
		l = slog.LevelDebug
	case "warn":
		l = slog.LevelWarn
	case "error":
		l = slog.LevelError
	default:
		l = slog.LevelInfo
	}

	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: l})
	slog.SetDefault(slog.New(handler))
}
