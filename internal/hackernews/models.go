package hackernews

import (
	"strconv"
	"time"
)

// Item represents a Hacker News item (story, comment, job, poll, etc.)
type Item struct {
	ID          int    `json:"id"`
	Type        string `json:"type"`
	By          string `json:"by"`
	Time        int64  `json:"time"`
	Title       string `json:"title"`
	URL         string `json:"url"`
	Score       int    `json:"score"`
	Descendants int    `json:"descendants"` // comment count
}

// Age returns the duration since the item was posted
func (i *Item) Age() time.Duration {
	return time.Since(time.Unix(i.Time, 0))
}

// HNURL returns the Hacker News discussion URL for this item
func (i *Item) HNURL() string {
	return "https://news.ycombinator.com/item?id=" + strconv.Itoa(i.ID)
}

// IsAskHN returns true if this is an Ask HN post
func (i *Item) IsAskHN() bool {
	return len(i.Title) >= 7 && i.Title[:7] == "Ask HN:"
}

// IsShowHN returns true if this is a Show HN post
func (i *Item) IsShowHN() bool {
	return len(i.Title) >= 8 && i.Title[:8] == "Show HN:"
}

// IsLaunchHN returns true if this is a Launch HN post
func (i *Item) IsLaunchHN() bool {
	return len(i.Title) >= 10 && i.Title[:10] == "Launch HN:"
}
