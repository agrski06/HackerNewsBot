# 🔶 HackerNewsBot

A lightweight Go service that posts top Hacker News stories to a private Telegram channel — with **Telegraph Instant View** for reading discussions directly inside Telegram.

## Features

- **Curated feed** — filters stories by score threshold (default ≥100)
- **Two display modes** — individual messages or batched digest
- **Inline buttons** — tap-friendly links to article, HN discussion, and Instant View
- **📝 Telegraph Instant View** — read HN comment threads natively in Telegram, no browser needed
- **Deduplication** — BoltDB-backed store ensures no story is sent twice
- **Silent by default** — no push notifications; browse when you want
- **Story type badges** — 🟠 Ask HN · 🟢 Show HN · 🚀 Launch HN

## Quick Start

1. Create a Telegram bot via [@BotFather](https://t.me/BotFather) and get the token
2. Create a private channel, add the bot as admin
3. Get the channel's chat ID (see [PLAN.md](PLAN.md#telegram-setup-one-time))

```bash
export HNB_TELEGRAM_BOT_TOKEN="your-token"
export HNB_TELEGRAM_CHAT_ID="-100123456789"

go run ./cmd/bot
```

## Configuration

All configuration is via environment variables (defaults can be overridden at runtime via bot commands):

| Variable | Default | Description |
|---|---|---|
| `HNB_TELEGRAM_BOT_TOKEN` | *required* | Telegram bot token |
| `HNB_TELEGRAM_CHAT_ID` | *required* | Target channel ID |
| `HNB_OWNER_USER_ID` | *required* | Your Telegram user ID (for bot commands) |
| `HNB_SCHEDULE` | *(empty)* | Delivery times, e.g. `09:00,18:00` |
| `HNB_TIMEZONE` | `UTC` | Timezone for schedule, e.g. `Europe/Berlin` |
| `HNB_FETCH_INTERVAL` | `30m` | Polling interval (used when no schedule set) |
| `HNB_SCORE_THRESHOLD` | `100` | Minimum story score |
| `HNB_MAX_STORIES_PER_RUN` | `5` | Max stories per cycle |
| `HNB_DIGEST_MODE` | `false` | Batch stories into one message |
| `HNB_SILENT_MESSAGES` | `true` | Send without notification |
| `HNB_TELEGRAPH_ENABLED` | `true` | Generate Instant View discussion pages |
| `HNB_MAX_TOP_COMMENTS` | `15` | Comments per Telegraph page |
| `HNB_MAX_COMMENT_DEPTH` | `3` | Reply nesting depth |

See [`.env.example`](.env.example) for the full list.

## Bot Commands

DM the bot directly on Telegram to configure it at runtime. Commands are owner-only.

| Command | Description |
|---|---|
| `/schedule 09:00,18:00` | Set delivery times (daily) |
| `/schedule off` | Switch back to interval mode |
| `/threshold 150` | Set minimum story score |
| `/maxstories 10` | Set max stories per run |
| `/digest on\|off` | Toggle digest mode |
| `/fetch` | Trigger a fetch right now |
| `/pause` / `/resume` | Pause/resume delivery |
| `/status` | Show current config |

Schedule changes take effect immediately — no restart needed. Settings persist in BoltDB across restarts.

## Deploy with Docker

```bash
docker compose up -d
```

Designed for [Coolify](https://coolify.io) — just point it at the repo and set env vars in the UI. See [PLAN.md](PLAN.md#coolify-deployment) for detailed setup steps.

## Architecture

```
Ticker → HN API → Filter (score + dedup) → Telegraph page → Telegram channel
```

```
cmd/bot/main.go          — entrypoint, health check, graceful shutdown
internal/config/         — env var loading
internal/hackernews/     — HN Firebase API client + comment tree fetcher
internal/filter/         — score threshold, dedup, sorting
internal/telegram/       — message formatting + Telegram Bot API sender
internal/telegraph/      — Telegraph page generation (Instant View)
internal/store/          — BoltDB persistence for dedup
internal/bot/            — orchestrator (fetch → filter → send loop)
```

## License

MIT

