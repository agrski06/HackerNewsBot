package telegram

import (
	"fmt"
	"log/slog"
	"time"

	"HackerNewsBot/internal/hackernews"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// Sender sends messages to a Telegram chat
type Sender struct {
	bot            *tgbotapi.BotAPI
	chatID         int64
	silent         bool
	disablePreview bool
}

// NewSender creates a new Telegram sender
func NewSender(token string, chatID int64, silent, disablePreview bool) (*Sender, error) {
	bot, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		return nil, fmt.Errorf("creating telegram bot: %w", err)
	}

	slog.Info("telegram bot authorized", "username", bot.Self.UserName)

	return &Sender{
		bot:            bot,
		chatID:         chatID,
		silent:         silent,
		disablePreview: disablePreview,
	}, nil
}

// SendIndividual sends a single story as its own message.
// telegraphURL is optional — if non-empty, adds an Instant View button.
func (s *Sender) SendIndividual(item *hackernews.Item, telegraphURL string) error {
	text, keyboard := FormatIndividual(item, telegraphURL)

	msg := tgbotapi.NewMessage(s.chatID, text)
	msg.ParseMode = tgbotapi.ModeHTML
	msg.ReplyMarkup = keyboard
	msg.DisableNotification = s.silent
	msg.DisableWebPagePreview = s.disablePreview

	return s.sendWithRetry(msg)
}

// SendDigest sends multiple stories as a single digest message.
// telegraphURLs maps item index to its Telegraph URL.
func (s *Sender) SendDigest(items []*hackernews.Item, telegraphURLs map[int]string) error {
	text, keyboard := FormatDigest(items, telegraphURLs)

	msg := tgbotapi.NewMessage(s.chatID, text)
	msg.ParseMode = tgbotapi.ModeHTML
	msg.ReplyMarkup = keyboard
	msg.DisableNotification = s.silent
	msg.DisableWebPagePreview = true // always disable for digest (multiple URLs)

	return s.sendWithRetry(msg)
}

func (s *Sender) sendWithRetry(msg tgbotapi.MessageConfig) error {
	const maxRetries = 3

	for attempt := range maxRetries {
		_, err := s.bot.Send(msg)
		if err == nil {
			return nil
		}

		// Check for rate limiting (429)
		if apiErr, ok := err.(*tgbotapi.Error); ok && apiErr.Code == 429 {
			wait := time.Duration(apiErr.RetryAfter) * time.Second
			if wait == 0 {
				wait = time.Duration(1<<attempt) * time.Second
			}
			slog.Warn("rate limited by telegram, waiting",
				"attempt", attempt+1,
				"wait", wait,
				"retry_after", apiErr.RetryAfter,
			)
			time.Sleep(wait)
			continue
		}

		return fmt.Errorf("sending telegram message (attempt %d): %w", attempt+1, err)
	}

	return fmt.Errorf("sending telegram message: max retries exceeded")
}
