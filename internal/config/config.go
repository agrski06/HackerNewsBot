package config

import (
	"fmt"
	"time"

	"github.com/caarlos0/env/v11"
)

type Config struct {
	TelegramBotToken string        `env:"HNB_TELEGRAM_BOT_TOKEN,required"`
	TelegramChatID   int64         `env:"HNB_TELEGRAM_CHAT_ID,required"`
	OwnerUserID      int64         `env:"HNB_OWNER_USER_ID,required"` // your Telegram user ID (for bot commands)
	HNFetchInterval  time.Duration `env:"HNB_FETCH_INTERVAL" envDefault:"30m"`
	Schedule         string        `env:"HNB_SCHEDULE" envDefault:""` // e.g. "09:00,18:00" — overrides interval when set
	Timezone         string        `env:"HNB_TIMEZONE" envDefault:"UTC"`
	ScoreThreshold   int           `env:"HNB_SCORE_THRESHOLD" envDefault:"100"`
	MaxStoriesPerRun int           `env:"HNB_MAX_STORIES_PER_RUN" envDefault:"5"`
	DBPath           string        `env:"HNB_DB_PATH" envDefault:"seen.db"`
	LogLevel         string        `env:"HNB_LOG_LEVEL" envDefault:"info"`
	DigestMode       bool          `env:"HNB_DIGEST_MODE" envDefault:"false"`
	DisablePreview   bool          `env:"HNB_DISABLE_PREVIEW" envDefault:"false"`
	SilentMessages   bool          `env:"HNB_SILENT_MESSAGES" envDefault:"true"`
	HealthPort       string        `env:"HNB_HEALTH_PORT" envDefault:"8080"`

	// Telegraph (Instant View for HN discussions)
	TelegraphEnabled bool `env:"HNB_TELEGRAPH_ENABLED" envDefault:"true"`
	MaxTopComments   int  `env:"HNB_MAX_TOP_COMMENTS" envDefault:"15"`
	MaxCommentDepth  int  `env:"HNB_MAX_COMMENT_DEPTH" envDefault:"3"`
}

func Load() (*Config, error) {
	cfg, err := env.ParseAs[Config]()
	if err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}
	return &cfg, nil
}

