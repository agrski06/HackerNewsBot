package commands

import (
	"fmt"
	"log/slog"
	"strconv"
	"strings"

	"HackerNewsBot/internal/scheduler"
	"HackerNewsBot/internal/store"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// Runtime config keys in BoltDB.
const (
	KeySchedule       = "schedule"
	KeyScoreThreshold = "score_threshold"
	KeyMaxStories     = "max_stories"
	KeyDigestMode     = "digest_mode"
	KeyPaused         = "paused"
)

// FetchFunc is called when /fetch is triggered.
type FetchFunc func()

// ScheduleChangedFunc is called when schedule changes so the bot can recalculate timers.
type ScheduleChangedFunc func()

// Handler processes Telegram bot commands from the owner.
type Handler struct {
	bot               *tgbotapi.BotAPI
	ownerID           int64
	store             store.Store
	onFetch           FetchFunc
	onScheduleChanged ScheduleChangedFunc
}

// NewHandler creates a command handler.
func NewHandler(bot *tgbotapi.BotAPI, ownerID int64, st store.Store, onFetch FetchFunc, onScheduleChanged ScheduleChangedFunc) *Handler {
	return &Handler{
		bot:               bot,
		ownerID:           ownerID,
		store:             st,
		onFetch:           onFetch,
		onScheduleChanged: onScheduleChanged,
	}
}

// Handle processes a single incoming message. Returns true if it was handled.
func (h *Handler) Handle(msg *tgbotapi.Message) bool {
	if msg == nil || msg.From == nil {
		return false
	}

	// Owner-only.
	if msg.From.ID != h.ownerID {
		slog.Warn("ignoring command from non-owner", "user_id", msg.From.ID, "text", msg.Text)
		return false
	}

	if !msg.IsCommand() {
		return false
	}

	var reply string

	switch msg.Command() {
	case "start", "help":
		reply = h.cmdHelp()
	case "status":
		reply = h.cmdStatus()
	case "schedule":
		reply = h.cmdSchedule(msg.CommandArguments())
	case "threshold":
		reply = h.cmdThreshold(msg.CommandArguments())
	case "maxstories":
		reply = h.cmdMaxStories(msg.CommandArguments())
	case "digest":
		reply = h.cmdDigest(msg.CommandArguments())
	case "fetch":
		reply = h.cmdFetch()
	case "pause":
		reply = h.cmdPause()
	case "resume":
		reply = h.cmdResume()
	default:
		reply = "❓ Unknown command. Try /help"
	}

	resp := tgbotapi.NewMessage(msg.Chat.ID, reply)
	resp.ParseMode = tgbotapi.ModeHTML
	if _, err := h.bot.Send(resp); err != nil {
		slog.Error("failed to send command response", "error", err)
	}

	return true
}

func (h *Handler) cmdHelp() string {
	return `🔶 <b>HackerNewsBot Commands</b>

<b>Schedule</b>
/schedule — show current schedule
/schedule 09:00,18:00 — set delivery times
/schedule off — switch to interval mode

<b>Tuning</b>
/threshold — show score threshold
/threshold 150 — set minimum score
/maxstories — show max stories per run
/maxstories 10 — set max stories
/digest on|off — toggle digest mode

<b>Control</b>
/fetch — trigger a fetch now
/pause — pause delivery
/resume — resume delivery
/status — show current config`
}

func (h *Handler) cmdStatus() string {
	schedule, _ := h.store.GetConfig(KeySchedule)
	threshold, _ := h.store.GetConfig(KeyScoreThreshold)
	maxStories, _ := h.store.GetConfig(KeyMaxStories)
	digest, _ := h.store.GetConfig(KeyDigestMode)
	paused, _ := h.store.GetConfig(KeyPaused)

	scheduleStr := "not set (using interval)"
	if schedule != "" {
		scheduleStr = schedule
	}

	pausedStr := "▶️ running"
	if paused == "true" {
		pausedStr = "⏸ paused"
	}

	return fmt.Sprintf(`📊 <b>Status</b>

⏰ Schedule: <code>%s</code>
⬆ Score threshold: <code>%s</code>
📰 Max stories/run: <code>%s</code>
📝 Digest mode: <code>%s</code>
%s

<i>Values shown are runtime overrides. Empty = using env default.</i>`,
		scheduleStr,
		valueOrDefault(threshold, "env default"),
		valueOrDefault(maxStories, "env default"),
		valueOrDefault(digest, "env default"),
		pausedStr,
	)
}

func (h *Handler) cmdSchedule(args string) string {
	args = strings.TrimSpace(args)

	// No args — show current.
	if args == "" {
		schedule, _ := h.store.GetConfig(KeySchedule)
		if schedule == "" {
			return "⏰ No schedule set — using interval mode.\n\nUsage: /schedule 09:00,18:00"
		}
		return fmt.Sprintf("⏰ Current schedule: <code>%s</code>\n\nUse /schedule off to disable.", schedule)
	}

	// Disable schedule.
	if args == "off" || args == "clear" {
		if err := h.store.DeleteConfig(KeySchedule); err != nil {
			return "❌ Failed to clear schedule: " + err.Error()
		}
		h.onScheduleChanged()
		return "⏰ Schedule cleared — switched to interval mode."
	}

	// Parse and set.
	slots, err := scheduler.ParseSchedule(args)
	if err != nil {
		return "❌ " + err.Error() + "\n\nFormat: /schedule 09:00,18:00"
	}
	if len(slots) == 0 {
		return "❌ No valid times found.\n\nFormat: /schedule 09:00,18:00"
	}

	formatted := scheduler.FormatSchedule(slots)
	if err := h.store.SetConfig(KeySchedule, formatted); err != nil {
		return "❌ Failed to save schedule: " + err.Error()
	}

	h.onScheduleChanged()

	var lines []string
	for _, s := range slots {
		lines = append(lines, "  • "+s.String())
	}
	return fmt.Sprintf("✅ Schedule set:\n%s\n\nStories will be delivered at these times daily.", strings.Join(lines, "\n"))
}

func (h *Handler) cmdThreshold(args string) string {
	args = strings.TrimSpace(args)
	if args == "" {
		val, _ := h.store.GetConfig(KeyScoreThreshold)
		return fmt.Sprintf("⬆ Score threshold: <code>%s</code>\n\nUsage: /threshold 150", valueOrDefault(val, "env default"))
	}

	n, err := strconv.Atoi(args)
	if err != nil || n < 0 {
		return "❌ Invalid number. Usage: /threshold 150"
	}

	if err := h.store.SetConfig(KeyScoreThreshold, strconv.Itoa(n)); err != nil {
		return "❌ Failed to save: " + err.Error()
	}
	return fmt.Sprintf("✅ Score threshold set to <b>%d</b>", n)
}

func (h *Handler) cmdMaxStories(args string) string {
	args = strings.TrimSpace(args)
	if args == "" {
		val, _ := h.store.GetConfig(KeyMaxStories)
		return fmt.Sprintf("📰 Max stories/run: <code>%s</code>\n\nUsage: /maxstories 10", valueOrDefault(val, "env default"))
	}

	n, err := strconv.Atoi(args)
	if err != nil || n < 1 || n > 30 {
		return "❌ Invalid number (1-30). Usage: /maxstories 10"
	}

	if err := h.store.SetConfig(KeyMaxStories, strconv.Itoa(n)); err != nil {
		return "❌ Failed to save: " + err.Error()
	}
	return fmt.Sprintf("✅ Max stories per run set to <b>%d</b>", n)
}

func (h *Handler) cmdDigest(args string) string {
	args = strings.TrimSpace(strings.ToLower(args))
	if args == "" {
		val, _ := h.store.GetConfig(KeyDigestMode)
		return fmt.Sprintf("📝 Digest mode: <code>%s</code>\n\nUsage: /digest on|off", valueOrDefault(val, "env default"))
	}

	switch args {
	case "on", "true", "yes":
		h.store.SetConfig(KeyDigestMode, "true")
		return "✅ Digest mode <b>enabled</b>"
	case "off", "false", "no":
		h.store.SetConfig(KeyDigestMode, "false")
		return "✅ Digest mode <b>disabled</b>"
	default:
		return "❌ Usage: /digest on|off"
	}
}

func (h *Handler) cmdFetch() string {
	h.onFetch()
	return "🔄 Fetch triggered!"
}

func (h *Handler) cmdPause() string {
	h.store.SetConfig(KeyPaused, "true")
	return "⏸ Bot paused. Stories won't be sent until you /resume."
}

func (h *Handler) cmdResume() string {
	h.store.DeleteConfig(KeyPaused)
	return "▶️ Bot resumed!"
}

func valueOrDefault(val, def string) string {
	if val == "" {
		return def
	}
	return val
}

