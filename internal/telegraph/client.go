package telegraph

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"
)

const apiBase = "https://api.telegra.ph"

// Client interacts with the Telegraph API.
type Client struct {
	http        *http.Client
	accessToken string
}

// NewClient creates a Telegraph account and returns a ready client.
func NewClient(shortName, authorName string) (*Client, error) {
	c := &Client{
		http: &http.Client{Timeout: 15 * time.Second},
	}

	token, err := c.createAccount(shortName, authorName)
	if err != nil {
		return nil, fmt.Errorf("creating telegraph account: %w", err)
	}

	c.accessToken = token
	slog.Info("telegraph account created", "short_name", shortName)
	return c, nil
}

// Node represents a Telegraph DOM node (used for rich content).
// It can be a text string or a tag element with children.
type Node interface{}

// TextNode is a plain text node.
type TextNode string

// ElementNode is an HTML-like element with tag, attributes, and children.
type ElementNode struct {
	Tag      string            `json:"tag"`
	Attrs    map[string]string `json:"attrs,omitempty"`
	Children []Node            `json:"children,omitempty"`
}

// MarshalJSON implements custom JSON for Node interface.
func (e ElementNode) MarshalJSON() ([]byte, error) {
	type raw struct {
		Tag      string            `json:"tag"`
		Attrs    map[string]string `json:"attrs,omitempty"`
		Children []json.RawMessage `json:"children,omitempty"`
	}

	r := raw{Tag: e.Tag, Attrs: e.Attrs}
	for _, child := range e.Children {
		b, err := marshalNode(child)
		if err != nil {
			return nil, err
		}
		r.Children = append(r.Children, b)
	}
	return json.Marshal(r)
}

func marshalNode(n Node) (json.RawMessage, error) {
	switch v := n.(type) {
	case TextNode:
		return json.Marshal(string(v))
	case ElementNode:
		return json.Marshal(v)
	default:
		return json.Marshal(fmt.Sprintf("%v", v))
	}
}

// MarshalNodes serializes a slice of Node into JSON.
func MarshalNodes(nodes []Node) (json.RawMessage, error) {
	var parts []json.RawMessage
	for _, n := range nodes {
		b, err := marshalNode(n)
		if err != nil {
			return nil, err
		}
		parts = append(parts, b)
	}
	return json.Marshal(parts)
}

// Page represents a created Telegraph page.
type Page struct {
	Path string `json:"path"`
	URL  string `json:"url"`
	// Title string `json:"title"`
}

// CreatePage publishes a new Telegraph page and returns its URL.
func (c *Client) CreatePage(title, authorName string, content []Node) (*Page, error) {
	contentJSON, err := MarshalNodes(content)
	if err != nil {
		return nil, fmt.Errorf("marshaling content: %w", err)
	}

	payload := map[string]interface{}{
		"access_token":  c.accessToken,
		"title":         title,
		"author_name":   authorName,
		"content":       json.RawMessage(contentJSON),
		"return_content": false,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshaling request: %w", err)
	}

	resp, err := c.http.Post(apiBase+"/createPage", "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("creating page: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		OK     bool   `json:"ok"`
		Error  string `json:"error"`
		Result Page   `json:"result"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	if !result.OK {
		return nil, fmt.Errorf("telegraph API error: %s", result.Error)
	}

	return &result.Result, nil
}

func (c *Client) createAccount(shortName, authorName string) (string, error) {
	payload := map[string]string{
		"short_name":  shortName,
		"author_name": authorName,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	resp, err := c.http.Post(apiBase+"/createAccount", "application/json", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var result struct {
		OK     bool   `json:"ok"`
		Error  string `json:"error"`
		Result struct {
			AccessToken string `json:"access_token"`
		} `json:"result"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}

	if !result.OK {
		return "", fmt.Errorf("telegraph: %s", result.Error)
	}

	return result.Result.AccessToken, nil
}

