package telegraph

import (
	"fmt"
	"strings"
	"time"

	"HackerNewsBot/internal/hackernews"
)

// RenderDiscussion converts a story + comment tree into Telegraph Nodes
// for a beautiful Instant View page.
func RenderDiscussion(story *hackernews.Item, comments []*hackernews.Comment) []Node {
	var nodes []Node

	// ── Header: story metadata ──
	nodes = append(nodes, ElementNode{
		Tag: "p",
		Children: []Node{
			ElementNode{
				Tag:   "a",
				Attrs: map[string]string{"href": effectiveURL(story)},
				Children: []Node{
					ElementNode{Tag: "b", Children: []Node{TextNode(story.Title)}},
				},
			},
		},
	})

	// Stats line.
	statsText := fmt.Sprintf("⬆ %d points · 💬 %d comments · 👤 %s · ⏰ %s",
		story.Score, story.Descendants, story.By, formatTimestamp(story.Time))
	nodes = append(nodes, ElementNode{
		Tag:      "p",
		Children: []Node{ElementNode{Tag: "em", Children: []Node{TextNode(statsText)}}},
	})

	// Link to original article + HN page.
	nodes = append(nodes, ElementNode{
		Tag: "p",
		Children: []Node{
			TextNode("📖 "),
			ElementNode{
				Tag:      "a",
				Attrs:    map[string]string{"href": effectiveURL(story)},
				Children: []Node{TextNode("Read Article")},
			},
			TextNode("  ·  🔶 "),
			ElementNode{
				Tag:      "a",
				Attrs:    map[string]string{"href": story.HNURL()},
				Children: []Node{TextNode("View on Hacker News")},
			},
		},
	})

	nodes = append(nodes, ElementNode{Tag: "hr"})

	// ── Comments ──
	if len(comments) == 0 {
		nodes = append(nodes, ElementNode{
			Tag:      "p",
			Children: []Node{ElementNode{Tag: "em", Children: []Node{TextNode("No comments yet.")}}},
		})
		return nodes
	}

	for i, comment := range comments {
		nodes = append(nodes, renderComment(comment, 0)...)

		// Add a divider between top-level comments (except after last).
		if i < len(comments)-1 {
			nodes = append(nodes, ElementNode{Tag: "hr"})
		}
	}

	return nodes
}

func renderComment(c *hackernews.Comment, depth int) []Node {
	if c == nil || c.Item == nil {
		return nil
	}

	var nodes []Node

	// Author + timestamp header.
	// Use h4 for top-level, bold paragraph for nested.
	authorText := fmt.Sprintf("%s · %s", c.Item.By, formatTimestamp(c.Item.Time))

	if depth == 0 {
		nodes = append(nodes, ElementNode{
			Tag:      "h4",
			Children: []Node{TextNode(authorText)},
		})
	} else {
		// Indent indicator for nested comments.
		prefix := strings.Repeat("↳ ", min(depth, 3))
		nodes = append(nodes, ElementNode{
			Tag: "p",
			Children: []Node{
				ElementNode{Tag: "b", Children: []Node{TextNode(prefix + authorText)}},
			},
		})
	}

	// Comment body. HN stores comments as HTML — we need to convert to Telegraph nodes.
	// Telegraph accepts a subset of HTML, so we parse the HN HTML into nodes.
	bodyNodes := parseHTMLToNodes(c.Item.Text)
	if depth > 0 {
		// Wrap nested comments in blockquote for visual indentation.
		nodes = append(nodes, ElementNode{
			Tag:      "blockquote",
			Children: bodyNodes,
		})
	} else {
		nodes = append(nodes, bodyNodes...)
	}

	// Render children recursively.
	for _, child := range c.Children {
		nodes = append(nodes, renderComment(child, depth+1)...)
	}

	return nodes
}

// parseHTMLToNodes converts HN comment HTML into Telegraph-compatible nodes.
// HN comments use <p>, <a>, <pre><code>, <i> tags — all supported by Telegraph.
// We wrap the raw HTML in a <p> tag. Telegraph's API accepts HTML strings as text
// content within nodes, but we need to handle it carefully.
func parseHTMLToNodes(htmlText string) []Node {
	if htmlText == "" {
		return nil
	}

	// HN uses <p> to separate paragraphs. Split on <p> and create separate paragraph nodes.
	// First, normalize: replace common HN patterns.
	text := strings.ReplaceAll(htmlText, "<p>", "\n<p>")
	paragraphs := strings.Split(text, "\n")

	var nodes []Node
	for _, p := range paragraphs {
		p = strings.TrimSpace(p)
		if p == "" || p == "<p>" {
			continue
		}

		// Strip leading <p> tag if present (we'll wrap in our own).
		p = strings.TrimPrefix(p, "<p>")
		p = strings.TrimSuffix(p, "</p>")
		p = strings.TrimSpace(p)

		if p == "" {
			continue
		}

		// Check if it's a code block.
		if strings.HasPrefix(p, "<pre><code>") {
			code := strings.TrimPrefix(p, "<pre><code>")
			code = strings.TrimSuffix(code, "</code></pre>")
			nodes = append(nodes, ElementNode{
				Tag: "pre",
				Children: []Node{
					ElementNode{Tag: "code", Children: []Node{TextNode(code)}},
				},
			})
			continue
		}

		// For regular text (may contain <a>, <i>, <b> inline tags),
		// wrap in a <p> and use the raw text as a TextNode.
		// Telegraph will handle the inline HTML.
		nodes = append(nodes, ElementNode{
			Tag:      "p",
			Children: []Node{TextNode(p)},
		})
	}

	if len(nodes) == 0 {
		nodes = append(nodes, ElementNode{
			Tag:      "p",
			Children: []Node{TextNode(htmlText)},
		})
	}

	return nodes
}

func effectiveURL(item *hackernews.Item) string {
	if item.URL != "" {
		return item.URL
	}
	return item.HNURL()
}

func formatTimestamp(unix int64) string {
	t := time.Unix(unix, 0)
	d := time.Since(t)

	switch {
	case d < time.Hour:
		m := int(d.Minutes())
		if m <= 1 {
			return "just now"
		}
		return fmt.Sprintf("%dm ago", m)
	case d < 24*time.Hour:
		h := int(d.Hours())
		if h == 1 {
			return "1h ago"
		}
		return fmt.Sprintf("%dh ago", h)
	default:
		days := int(d.Hours() / 24)
		if days == 1 {
			return "1d ago"
		}
		return fmt.Sprintf("%dd ago", days)
	}
}

