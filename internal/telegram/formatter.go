package telegram

import (
	"fmt"
	"html"
	"strings"
	"time"

	"HackerNewsBot/internal/hackernews"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

var numberEmojis = []string{"1️⃣", "2️⃣", "3️⃣", "4️⃣", "5️⃣", "6️⃣", "7️⃣", "8️⃣", "9️⃣", "🔟"}

// FormatIndividual builds the HTML message text and inline keyboard for a single story.
// If telegraphURL is non-empty, a "📝 Discussion" Instant View button is added.
func FormatIndividual(item *hackernews.Item, telegraphURL string) (string, tgbotapi.InlineKeyboardMarkup) {
	var sb strings.Builder

	// Story type badge + title
	badge := storyBadge(item)
	title := html.EscapeString(item.Title)
	articleURL := effectiveURL(item)

	sb.WriteString(fmt.Sprintf("%s <a href=\"%s\"><b>%s</b></a>\n\n", badge, articleURL, title))

	// Stats line
	sb.WriteString(fmt.Sprintf("⬆ %d points  ·  💬 %d comments  ·  👤 %s\n",
		item.Score, item.Descendants, html.EscapeString(item.By)))

	// Time ago
	sb.WriteString(fmt.Sprintf("⏰ %s", formatAge(item.Age())))

	// Keyboard
	keyboard := buildIndividualKeyboard(item, telegraphURL)

	return sb.String(), keyboard
}

// FormatDigest builds the HTML message text and inline keyboard for a batch of stories.
// telegraphURLs maps item index to its Telegraph page URL (may be empty for some).
func FormatDigest(items []*hackernews.Item, telegraphURLs map[int]string) (string, tgbotapi.InlineKeyboardMarkup) {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("📰 <b>Hacker News Digest — %s</b>\n", time.Now().Format("Jan 2, 2006")))

	for i, item := range items {
		emoji := "🔶"
		if i < len(numberEmojis) {
			emoji = numberEmojis[i]
		}

		badge := storyBadge(item)
		title := html.EscapeString(item.Title)
		articleURL := effectiveURL(item)

		sb.WriteString(fmt.Sprintf("\n%s  %s<a href=\"%s\"><b>%s</b></a>\n",
			emoji, badge, articleURL, title))
		sb.WriteString(fmt.Sprintf("    ⬆ %d pts · 💬 %d · 👤 %s\n",
			item.Score, item.Descendants, html.EscapeString(item.By)))
	}

	keyboard := buildDigestKeyboard(items, telegraphURLs)

	return sb.String(), keyboard
}

func storyBadge(item *hackernews.Item) string {
	switch {
	case item.IsAskHN():
		return "🟠"
	case item.IsShowHN():
		return "🟢"
	case item.IsLaunchHN():
		return "🚀"
	default:
		return "🔶"
	}
}

func effectiveURL(item *hackernews.Item) string {
	if item.URL != "" {
		return item.URL
	}
	return item.HNURL()
}

func buildIndividualKeyboard(item *hackernews.Item, telegraphURL string) tgbotapi.InlineKeyboardMarkup {
	hnURL := item.HNURL()

	var rows [][]tgbotapi.InlineKeyboardButton

	// If no external URL (e.g., Ask HN text post), show only HN button
	if item.URL == "" {
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonURL("💬 Read on HN", hnURL),
		))
	} else {
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonURL("📖 Read Article", item.URL),
			tgbotapi.NewInlineKeyboardButtonURL("💬 HN Discussion", hnURL),
		))
	}

	// Add Telegraph "Discussion" button if URL is available.
	if telegraphURL != "" {
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonURL("📝 Read Discussion", telegraphURL),
		))
	}

	return tgbotapi.InlineKeyboardMarkup{InlineKeyboard: rows}
}

func buildDigestKeyboard(items []*hackernews.Item, telegraphURLs map[int]string) tgbotapi.InlineKeyboardMarkup {
	var rows [][]tgbotapi.InlineKeyboardButton

	for i, item := range items {
		num := i + 1
		hnURL := item.HNURL()

		if item.URL == "" {
			// Ask HN-style: single button
			rows = append(rows, tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonURL(fmt.Sprintf("💬 #%d on HN", num), hnURL),
			))
		} else {
			rows = append(rows, tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonURL(fmt.Sprintf("📖 Read #%d", num), item.URL),
				tgbotapi.NewInlineKeyboardButtonURL(fmt.Sprintf("💬 HN #%d", num), hnURL),
			))
		}

		// Add Telegraph button for this story if available.
		if tURL, ok := telegraphURLs[i]; ok && tURL != "" {
			rows = append(rows, tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonURL(fmt.Sprintf("📝 Discussion #%d", num), tURL),
			))
		}
	}

	return tgbotapi.InlineKeyboardMarkup{InlineKeyboard: rows}
}

func formatAge(d time.Duration) string {
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		m := int(d.Minutes())
		if m == 1 {
			return "1 minute ago"
		}
		return fmt.Sprintf("%d minutes ago", m)
	case d < 24*time.Hour:
		h := int(d.Hours())
		if h == 1 {
			return "1 hour ago"
		}
		return fmt.Sprintf("%d hours ago", h)
	default:
		days := int(d.Hours() / 24)
		if days == 1 {
			return "1 day ago"
		}
		return fmt.Sprintf("%d days ago", days)
	}
}
