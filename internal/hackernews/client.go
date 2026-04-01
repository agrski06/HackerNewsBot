package hackernews

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"
)

const (
	baseURL        = "https://hacker-news.firebaseio.com/v0"
	maxConcurrency = 10
)

// Client fetches data from the Hacker News Firebase API
type Client struct {
	http *http.Client
}

// NewClient creates a new HN API client
func NewClient() *Client {
	return &Client{
		http: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

// TopStoryIDs returns the current top story IDs
func (c *Client) TopStoryIDs(ctx context.Context) ([]int, error) {
	url := baseURL + "/topstories.json"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching top stories: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d for top stories", resp.StatusCode)
	}

	var ids []int
	if err := json.NewDecoder(resp.Body).Decode(&ids); err != nil {
		return nil, fmt.Errorf("decoding top stories: %w", err)
	}

	return ids, nil
}

// GetItem fetches a single item by ID
func (c *Client) GetItem(ctx context.Context, id int) (*Item, error) {
	url := fmt.Sprintf("%s/item/%d.json", baseURL, id)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request for item %d: %w", id, err)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching item %d: %w", id, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d for item %d", resp.StatusCode, id)
	}

	var item Item
	if err := json.NewDecoder(resp.Body).Decode(&item); err != nil {
		return nil, fmt.Errorf("decoding item %d: %w", id, err)
	}

	return &item, nil
}

// GetItems fetches multiple items concurrently
func (c *Client) GetItems(ctx context.Context, ids []int) ([]*Item, error) {
	var (
		mu    sync.Mutex
		items = make([]*Item, 0, len(ids))
	)

	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(maxConcurrency)

	for _, id := range ids {
		g.Go(func() error {
			item, err := c.GetItem(ctx, id)
			if err != nil {
				slog.Warn("failed to fetch item", "id", id, "error", err)
				return nil // don't fail the whole batch for one item
			}
			mu.Lock()
			items = append(items, item)
			mu.Unlock()
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return nil, err
	}

	return items, nil
}
