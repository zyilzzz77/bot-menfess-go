package ai

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Config holds AI client configuration.
type Config struct {
	DeepSeekKey    string
	DeepSeekBaseURL string // default: https://api.deepseek.com/v1/chat/completions
	Model          string
	SystemPrompt   string
	MaxTokens      int
	Temperature    float64
}

// Client handles all AI API calls (DeepSeek chat, web search).
type Client struct {
	config     Config
	httpClient *http.Client
}

// --- DeepSeek Chat Completion Types ---

type contentPart struct {
	Type     string    `json:"type"`
	Text     string    `json:"text,omitempty"`
	ImageURL *imageURL `json:"image_url,omitempty"`
}

type imageURL struct {
	URL    string `json:"url"`
	Detail string `json:"detail,omitempty"`
}

type chatMessage struct {
	Role    string        `json:"role"`
	Content interface{}   `json:"content"` // string for system, []contentPart for user with image
}

type chatRequest struct {
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	MaxTokens   int           `json:"max_tokens,omitempty"`
	Temperature float64       `json:"temperature,omitempty"`
}

type chatResponse struct {
	ID      string       `json:"id"`
	Choices []chatChoice `json:"choices"`
	Error   *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

type chatChoice struct {
	Index   int         `json:"index"`
	Message chatRespMsg `json:"message"`
}

type chatRespMsg struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// --- Web Search Types ---

// SearchResult holds a single web search result.
type SearchResult struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Snippet string `json:"snippet"`
}

// NewClient creates a new AI client with defaults applied.
func NewClient(cfg Config) *Client {
	if cfg.DeepSeekBaseURL == "" {
		cfg.DeepSeekBaseURL = "https://api.deepseek.com/v1/chat/completions"
	}
	if cfg.Model == "" {
		cfg.Model = "deepseek-chat"
	}
	if cfg.SystemPrompt == "" {
		cfg.SystemPrompt = "Kamu adalah asisten AI yang membantu dan ramah. Jawablah dalam bahasa Indonesia kecuali ditanya dalam bahasa lain. Berikan jawaban yang informatif, akurat, dan to the point. Jaga jawaban tetap ringkas untuk WhatsApp."
	}
	if cfg.MaxTokens == 0 {
		cfg.MaxTokens = 2048
	}
	if cfg.Temperature == 0 {
		cfg.Temperature = 0.7
	}

	return &Client{
		config: cfg,
		httpClient: &http.Client{
			Timeout: 90 * time.Second,
		},
	}
}

// Chat sends a text-only query to DeepSeek and returns the response.
func (c *Client) Chat(ctx context.Context, userText string) (string, error) {
	messages := []chatMessage{
		{Role: "system", Content: c.config.SystemPrompt},
		{Role: "user", Content: userText},
	}
	return c.chatCompletion(ctx, messages)
}

// ChatWithImage sends a multimodal query with a base64-encoded image to DeepSeek.
// imageData is the raw image bytes; mimeType should be "image/jpeg", "image/png", etc.
func (c *Client) ChatWithImage(ctx context.Context, userText string, imageData []byte, mimeType string) (string, error) {
	if mimeType == "" {
		mimeType = "image/jpeg"
	}

	b64 := base64.StdEncoding.EncodeToString(imageData)
	dataURL := fmt.Sprintf("data:%s;base64,%s", mimeType, b64)

	parts := []contentPart{
		{
			Type: "image_url",
			ImageURL: &imageURL{
				URL:    dataURL,
				Detail: "auto",
			},
		},
	}

	if userText != "" {
		parts = append([]contentPart{{Type: "text", Text: userText}}, parts...)
	} else {
		parts = append([]contentPart{{Type: "text", Text: "Jelaskan apa yang kamu lihat di gambar ini dalam bahasa Indonesia."}}, parts...)
	}

	messages := []chatMessage{
		{Role: "system", Content: c.config.SystemPrompt},
		{Role: "user", Content: parts},
	}
	return c.chatCompletion(ctx, messages)
}

// chatCompletion is the shared DeepSeek API caller.
func (c *Client) chatCompletion(ctx context.Context, messages []chatMessage) (string, error) {
	body := chatRequest{
		Model:       c.config.Model,
		Messages:    messages,
		MaxTokens:   c.config.MaxTokens,
		Temperature: c.config.Temperature,
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.config.DeepSeekBaseURL, bytes.NewReader(jsonBody))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.config.DeepSeekKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	var apiResp chatResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return "", fmt.Errorf("parse response: %w", err)
	}

	if apiResp.Error != nil {
		return "", fmt.Errorf("API error: %s", apiResp.Error.Message)
	}

	if len(apiResp.Choices) == 0 {
		return "", fmt.Errorf("no response from AI (empty choices)")
	}

	return strings.TrimSpace(apiResp.Choices[0].Message.Content), nil
}

// WebSearch performs a web search using DuckDuckGo Lite and returns results.
// No API key required — uses DuckDuckGo's HTML (non-JS) version.
func (c *Client) WebSearch(ctx context.Context, query string) ([]SearchResult, error) {
	searchURL := fmt.Sprintf("https://lite.duckduckgo.com/lite/?%s",
		url.Values{"q": {query}}.Encode(),
	)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, searchURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create search request: %w", err)
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("search request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 512*1024)) // 512KB limit
	if err != nil {
		return nil, fmt.Errorf("read search response: %w", err)
	}

	return parseDuckDuckGoLite(body), nil
}

// parseDuckDuckGoLite extracts search results from DuckDuckGo Lite HTML.
func parseDuckDuckGoLite(html []byte) []SearchResult {
	var results []SearchResult
	content := string(html)

	// DuckDuckGo Lite uses simple HTML structure:
	// <a rel="nofollow" href="URL" class="result-link">Title</a>
	// <span class="result-snippet">Snippet</span>

	// Split by result sections
	lines := strings.Split(content, "\n")
	var currentTitle, currentURL, currentSnippet string

	for _, line := range lines {
		line = strings.TrimSpace(line)

		// Extract link: <a rel="nofollow" href="URL" class="result-link">Title</a>
		if strings.Contains(line, "class=\"result-link\"") || strings.Contains(line, "class='result-link'") {
			// Save previous result if exists
			if currentURL != "" && currentTitle != "" {
				results = append(results, SearchResult{
					Title:   cleanHTML(currentTitle),
					URL:     currentURL,
					Snippet: cleanHTML(currentSnippet),
				})
			}

			// Extract href
			hrefStart := strings.Index(line, "href=\"")
			if hrefStart < 0 {
				hrefStart = strings.Index(line, "href='")
				if hrefStart < 0 {
					continue
				}
				hrefStart += 6
				hrefEnd := strings.Index(line[hrefStart:], "'")
				if hrefEnd < 0 {
					continue
				}
				currentURL = line[hrefStart : hrefStart+hrefEnd]
			} else {
				hrefStart += 6
				hrefEnd := strings.Index(line[hrefStart:], "\"")
				if hrefEnd < 0 {
					continue
				}
				currentURL = line[hrefStart : hrefStart+hrefEnd]
			}

			// Extract title text between > and </a>
			titleStart := strings.LastIndex(line, ">")
			if titleStart >= 0 {
				titleEnd := strings.Index(line[titleStart:], "</a>")
				if titleEnd >= 0 {
					currentTitle = line[titleStart+1 : titleStart+titleEnd]
				}
			}
			currentSnippet = ""
		}

		// Extract snippet: <span class="result-snippet">Snippet</span>
		if strings.Contains(line, "class=\"result-snippet\"") || strings.Contains(line, "class='result-snippet'") {
			snipStart := strings.Index(line, ">")
			if snipStart >= 0 {
				snipEnd := strings.Index(line[snipStart:], "</span>")
				if snipEnd >= 0 {
					currentSnippet = line[snipStart+1 : snipStart+snipEnd]
				}
			}
		}
	}

	// Save last result
	if currentURL != "" && currentTitle != "" {
		results = append(results, SearchResult{
			Title:   cleanHTML(currentTitle),
			URL:     currentURL,
			Snippet: cleanHTML(currentSnippet),
		})
	}

	// Limit to 5 results
	if len(results) > 5 {
		results = results[:5]
	}

	return results
}

// cleanHTML strips basic HTML tags and decodes common entities.
func cleanHTML(s string) string {
	s = strings.ReplaceAll(s, "&amp;", "&")
	s = strings.ReplaceAll(s, "&lt;", "<")
	s = strings.ReplaceAll(s, "&gt;", ">")
	s = strings.ReplaceAll(s, "&quot;", "\"")
	s = strings.ReplaceAll(s, "&#39;", "'")
	s = strings.ReplaceAll(s, "&apos;", "'")

	// Remove HTML tags
	for {
		start := strings.Index(s, "<")
		if start < 0 {
			break
		}
		end := strings.Index(s[start:], ">")
		if end < 0 {
			break
		}
		s = s[:start] + s[start+end+1:]
	}

	return strings.TrimSpace(s)
}

// ChatWithWebSearch searches the web first, then sends results + query to DeepSeek for a summarized answer.
func (c *Client) ChatWithWebSearch(ctx context.Context, query string) (string, []SearchResult, error) {
	// Step 1: Search the web
	results, err := c.WebSearch(ctx, query)
	if err != nil {
		// Fallback: just chat without search
		answer, _ := c.Chat(ctx, query)
		return answer, nil, fmt.Errorf("web search failed (answering without search): %w", err)
	}

	if len(results) == 0 {
		answer, err := c.Chat(ctx, query)
		return answer, nil, err
	}

	// Step 2: Build a prompt with search results
	var searchContext strings.Builder
	searchContext.WriteString("Berikut adalah hasil pencarian web terbaru:\n\n")
	for i, r := range results {
		searchContext.WriteString(fmt.Sprintf("%d. %s\n   URL: %s\n   %s\n\n", i+1, r.Title, r.URL, r.Snippet))
	}
	searchContext.WriteString("\nBerdasarkan hasil pencarian di atas, jawab pertanyaan berikut:\n")
	searchContext.WriteString(query)

	// Step 3: Send to DeepSeek with search context
	messages := []chatMessage{
		{Role: "system", Content: c.config.SystemPrompt + "\n\nKamu memiliki akses ke hasil pencarian web terbaru. Gunakan informasi dari hasil pencarian untuk memberikan jawaban yang akurat dan up-to-date. Selalu sebutkan sumber informasi jika relevan."},
		{Role: "user", Content: searchContext.String()},
	}

	answer, err := c.chatCompletion(ctx, messages)
	if err != nil {
		return "", results, err
	}

	return answer, results, nil
}
