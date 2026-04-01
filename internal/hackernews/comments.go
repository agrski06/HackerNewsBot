package hackernews

import (
	"context"
	"log/slog"
	"sync"

	"golang.org/x/sync/errgroup"
)

const maxCommentConcurrency = 10

// GetCommentTree fetches the comment tree for a story, up to maxDepth levels
// and maxTopLevel top-level comments. Each top-level comment fetches replies
// up to maxDepth deep.
func (c *Client) GetCommentTree(ctx context.Context, story *Item, maxTopLevel, maxDepth int) ([]*Comment, error) {
	if len(story.Kids) == 0 {
		return nil, nil
	}

	kids := story.Kids
	if len(kids) > maxTopLevel {
		kids = kids[:maxTopLevel]
	}

	// Fetch top-level comments concurrently.
	var (
		mu       sync.Mutex
		comments = make([]*Comment, len(kids))
	)

	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(maxCommentConcurrency)

	for i, kid := range kids {
		g.Go(func() error {
			comment, err := c.fetchComment(ctx, kid, maxDepth, 0)
			if err != nil {
				slog.Warn("failed to fetch comment", "id", kid, "error", err)
				return nil
			}
			mu.Lock()
			comments[i] = comment
			mu.Unlock()
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return nil, err
	}

	// Filter out nils (failed/deleted comments).
	result := make([]*Comment, 0, len(comments))
	for _, c := range comments {
		if c != nil && c.Item != nil && !c.Item.Deleted && !c.Item.Dead {
			result = append(result, c)
		}
	}

	return result, nil
}

func (c *Client) fetchComment(ctx context.Context, id, maxDepth, currentDepth int) (*Comment, error) {
	item, err := c.GetItem(ctx, id)
	if err != nil {
		return nil, err
	}

	if item == nil || item.Deleted || item.Dead {
		return nil, nil
	}

	comment := &Comment{Item: item}

	// Don't recurse beyond maxDepth.
	if currentDepth >= maxDepth || len(item.Kids) == 0 {
		return comment, nil
	}

	// Fetch child comments (sequentially to limit API pressure per thread).
	for _, kidID := range item.Kids {
		child, err := c.fetchComment(ctx, kidID, maxDepth, currentDepth+1)
		if err != nil {
			slog.Debug("skipping failed child comment", "id", kidID, "error", err)
			continue
		}
		if child != nil && child.Item != nil && !child.Item.Deleted && !child.Item.Dead {
			comment.Children = append(comment.Children, child)
		}
	}

	return comment, nil
}

