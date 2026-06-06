package ai

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"
)

// Config holds AI client configuration.
type Config struct {
	DeepSeekKey     string
	DeepSeekBaseURL string // default: https://api.deepseek.com/v1/chat/completions
	HermesURL       string // optional: Hermes Agent API (http://localhost:8642/v1)
	Model           string
	SystemPrompt    string
	MaxTokens       int
	ReasoningEffort string // "high" or "max" for thinking mode
}

// Client handles all AI API calls (DeepSeek chat, optionally via Hermes Agent).
type Client struct {
	config     Config
	httpClient *http.Client
	apiURL     string // resolved API endpoint
}

// --- DeepSeek Chat Completion Types ---

type thinkingConfig struct {
	Type string `json:"type"` // "enabled" or "disabled"
}

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
	Role    string      `json:"role"`
	Content interface{} `json:"content"` // string for system, []contentPart for user with image
}

type chatRequest struct {
	Model           string         `json:"model"`
	Messages        []chatMessage  `json:"messages"`
	MaxTokens       int            `json:"max_tokens,omitempty"`
	ReasoningEffort string         `json:"reasoning_effort,omitempty"`
	Thinking        *thinkingConfig `json:"thinking,omitempty"`
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
	Role             string `json:"role"`
	Content          string `json:"content"`
	ReasoningContent string `json:"reasoning_content"`
}

// NewClient creates a new AI client with defaults applied.
func NewClient(cfg Config) *Client {
	if cfg.DeepSeekBaseURL == "" {
		cfg.DeepSeekBaseURL = "https://api.deepseek.com/v1/chat/completions"
	}
	if cfg.Model == "" {
		cfg.Model = "deepseek-v4-pro"
	}
	if cfg.SystemPrompt == "" {
		cfg.SystemPrompt = "Kamu adalah asisten AI yang membantu dan ramah. Jawablah dalam bahasa Indonesia kecuali ditanya dalam bahasa lain. Berikan jawaban yang informatif, akurat, dan to the point. Jaga jawaban tetap ringkas untuk WhatsApp."
	}
	if cfg.MaxTokens == 0 {
		cfg.MaxTokens = 4096
	}
	if cfg.ReasoningEffort == "" {
		cfg.ReasoningEffort = "high"
	}

	// Resolve API URL: Hermes Agent (if set) or DeepSeek directly
	apiURL := cfg.DeepSeekBaseURL
	if cfg.HermesURL != "" {
		apiURL = cfg.HermesURL + "/chat/completions"
		fmt.Printf("🧠 AI: Using Hermes Agent at %s\n", cfg.HermesURL)
	} else {
		fmt.Printf("🧠 AI: Using DeepSeek directly (%s)\n", cfg.Model)
	}

	return &Client{
		config: cfg,
		apiURL: apiURL,
		httpClient: &http.Client{
			Timeout: 120 * time.Second,
			Transport: &http.Transport{
				DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
					return (&net.Dialer{
						Timeout:   30 * time.Second,
						KeepAlive: 30 * time.Second,
					}).DialContext(ctx, "tcp4", addr)
				},
			},
		},
	}
}

// Chat sends a text-only query to DeepSeek with thinking mode enabled.
func (c *Client) Chat(ctx context.Context, userText string) (string, error) {
	messages := []chatMessage{
		{Role: "system", Content: c.config.SystemPrompt},
		{Role: "user", Content: userText},
	}
	return c.chatCompletion(ctx, messages)
}

// ChatWithImage sends a multimodal query with a base64-encoded image to DeepSeek.
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

// chatCompletion is the shared DeepSeek API caller with thinking mode.
func (c *Client) chatCompletion(ctx context.Context, messages []chatMessage) (string, error) {
	body := chatRequest{
		Model:           c.config.Model,
		Messages:        messages,
		MaxTokens:       c.config.MaxTokens,
		ReasoningEffort: c.config.ReasoningEffort,
		Thinking:        &thinkingConfig{Type: "enabled"},
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.apiURL, bytes.NewReader(jsonBody))
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

	msg := apiResp.Choices[0].Message

	// Log thinking usage
	if msg.ReasoningContent != "" {
		fmt.Printf("🧠 [AI Thinking] %d chars of reasoning used\n", len(msg.ReasoningContent))
	}

	return strings.TrimSpace(msg.Content), nil
}
