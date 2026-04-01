package telegraph

import (
	"fmt"
	"strings"
	"time"

	"golang.org/x/net/html"

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
		nodes = append(nodes, ElementNode{
			Tag: "p",
			Children: []Node{
				ElementNode{Tag: "b", Children: []Node{TextNode("↳ " + authorText)}},
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

// parseHTMLToNodes properly parses HN comment HTML into Telegraph nodes.
//
// HN comments contain HTML entities (&#x27; &#x2F; &amp; &quot;) and inline
// tags (<a>, <i>, <b>, <code>, <pre>). We use a real HTML parser so that
// entities are decoded and tags become proper Telegraph ElementNodes.
func parseHTMLToNodes(htmlText string) []Node {
	if htmlText == "" {
		return nil
	}

	// HN uses bare <p> (no </p>) to separate paragraphs.
	// Wrap in a root element so the parser can handle it.
	root := "<div>" + htmlText + "</div>"

	doc, err := html.Parse(strings.NewReader(root))
	if err != nil {
		// Fallback: decode entities and return as plain text.
		return []Node{ElementNode{
			Tag:      "p",
			Children: []Node{TextNode(html.UnescapeString(htmlText))},
		}}
	}

	// Find our <div> wrapper and convert its children.
	var divNode *html.Node
	var findDiv func(*html.Node)
	findDiv = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "div" {
			divNode = n
			return
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			findDiv(c)
			if divNode != nil {
				return
			}
		}
	}
	findDiv(doc)

	if divNode == nil {
		return []Node{ElementNode{
			Tag:      "p",
			Children: []Node{TextNode(html.UnescapeString(htmlText))},
		}}
	}

	return convertChildren(divNode)
}

// convertChildren converts the children of an html.Node into Telegraph nodes.
// It groups inline content into <p> paragraphs and preserves block-level elements.
func convertChildren(parent *html.Node) []Node {
	var nodes []Node
	var inlineBuf []Node // accumulates inline content for a paragraph

	flushInline := func() {
		if len(inlineBuf) == 0 {
			return
		}
		// Trim leading/trailing whitespace-only text nodes.
		nodes = append(nodes, ElementNode{Tag: "p", Children: inlineBuf})
		inlineBuf = nil
	}

	for c := parent.FirstChild; c != nil; c = c.NextSibling {
		switch {
		case c.Type == html.TextNode:
			text := c.Data
			if strings.TrimSpace(text) != "" {
				inlineBuf = append(inlineBuf, TextNode(text))
			} else if len(inlineBuf) > 0 {
				// Preserve meaningful whitespace between inline elements.
				inlineBuf = append(inlineBuf, TextNode(" "))
			}

		case c.Type == html.ElementNode:
			switch c.Data {
			case "p":
				// <p> starts a new paragraph.
				flushInline()
				children := convertInlineChildren(c)
				if len(children) > 0 {
					nodes = append(nodes, ElementNode{Tag: "p", Children: children})
				}

			case "pre":
				// <pre> block — extract code content.
				flushInline()
				var code string
				if c.FirstChild != nil && c.FirstChild.Type == html.ElementNode && c.FirstChild.Data == "code" {
					code = extractText(c.FirstChild)
				} else {
					code = extractText(c)
				}
				nodes = append(nodes, ElementNode{
					Tag: "pre",
					Children: []Node{
						ElementNode{Tag: "code", Children: []Node{TextNode(code)}},
					},
				})

			case "a", "i", "b", "em", "strong", "code", "s", "u":
				// Inline elements — accumulate in current paragraph.
				inlineBuf = append(inlineBuf, convertInlineElement(c))

			case "br":
				inlineBuf = append(inlineBuf, TextNode("\n"))

			case "blockquote":
				flushInline()
				children := convertChildren(c)
				nodes = append(nodes, ElementNode{Tag: "blockquote", Children: children})

			default:
				// Unknown tag — convert children inline.
				inlineBuf = append(inlineBuf, convertInlineChildren(c)...)
			}

		default:
			// Skip comments, doctypes, etc.
		}
	}

	flushInline()
	return nodes
}

// convertInlineChildren converts all children of a node into inline Telegraph nodes.
func convertInlineChildren(n *html.Node) []Node {
	var nodes []Node
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		switch {
		case c.Type == html.TextNode:
			if c.Data != "" {
				nodes = append(nodes, TextNode(c.Data))
			}
		case c.Type == html.ElementNode:
			nodes = append(nodes, convertInlineElement(c))
		}
	}
	return nodes
}

// convertInlineElement converts an inline HTML element to a Telegraph node.
func convertInlineElement(n *html.Node) Node {
	switch n.Data {
	case "a":
		href := getAttr(n, "href")
		children := convertInlineChildren(n)
		if len(children) == 0 {
			children = []Node{TextNode(href)}
		}
		return ElementNode{
			Tag:      "a",
			Attrs:    map[string]string{"href": href},
			Children: children,
		}
	case "i", "em":
		return ElementNode{Tag: "em", Children: convertInlineChildren(n)}
	case "b", "strong":
		return ElementNode{Tag: "b", Children: convertInlineChildren(n)}
	case "code":
		return ElementNode{Tag: "code", Children: []Node{TextNode(extractText(n))}}
	case "s":
		return ElementNode{Tag: "s", Children: convertInlineChildren(n)}
	case "u":
		return ElementNode{Tag: "u", Children: convertInlineChildren(n)}
	case "br":
		return TextNode("\n")
	default:
		// Unknown inline tag — just extract children.
		children := convertInlineChildren(n)
		if len(children) == 1 {
			return children[0]
		}
		return ElementNode{Tag: "em", Children: children} // fallback wrapper
	}
}

// extractText recursively extracts all text content from a node tree.
func extractText(n *html.Node) string {
	if n.Type == html.TextNode {
		return n.Data
	}
	var sb strings.Builder
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		sb.WriteString(extractText(c))
	}
	return sb.String()
}

// getAttr returns the value of the named attribute, or empty string.
func getAttr(n *html.Node, name string) string {
	for _, a := range n.Attr {
		if a.Key == name {
			return a.Val
		}
	}
	return ""
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

