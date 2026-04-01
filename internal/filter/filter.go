package filter

import (
	"sort"

	"HackerNewsBot/internal/hackernews"
	"HackerNewsBot/internal/store"
)

// Filter returns items that pass the score threshold and haven't been seen,
// sorted by score descending and capped to maxItems
func Filter(items []*hackernews.Item, st store.Store, scoreThreshold, maxItems int) []*hackernews.Item {
	var filtered []*hackernews.Item

	for _, item := range items {
		if item == nil {
			continue
		}
		// Skip non-story types
		if item.Type != "" && item.Type != "story" {
			continue
		}
		// Skip jobs and polls
		if item.Title == "" {
			continue
		}
		// Score threshold
		if item.Score < scoreThreshold {
			continue
		}
		// Deduplication
		if st.HasSeen(item.ID) {
			continue
		}
		filtered = append(filtered, item)
	}

	// Sort by score descending
	sort.Slice(filtered, func(i, j int) bool {
		return filtered[i].Score > filtered[j].Score
	})

	// Cap to maxItems
	if len(filtered) > maxItems {
		filtered = filtered[:maxItems]
	}

	return filtered
}
