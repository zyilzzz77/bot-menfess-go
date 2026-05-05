package downloader

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

// bratResponse represents the API response from neoxr brat endpoint.
type bratResponse struct {
	Creator string `json:"creator"`
	Status  bool   `json:"status"`
	Data    struct {
		Filename string `json:"filename"`
		Size     string `json:"size"`
		Expired  string `json:"expired"`
		URL      string `json:"url"`
	} `json:"data"`
}

// GenerateBratImage creates a brat sticker image from text and returns the image bytes.
func (d *Downloader) GenerateBratImage(ctx context.Context, text string) ([]byte, error) {
	apiURL := fmt.Sprintf("%s/brat?text=%s&apikey=%s",
		d.BaseURL,
		url.QueryEscape(text),
		d.APIKey,
	)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := d.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	var apiResp bratResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return nil, fmt.Errorf("failed to parse API response: %w", err)
	}

	if !apiResp.Status {
		return nil, fmt.Errorf("API returned error status")
	}

	if apiResp.Data.URL == "" {
		return nil, fmt.Errorf("no brat image URL found in API response")
	}

	return d.downloadBytes(ctx, apiResp.Data.URL)
}

// downloadBytes downloads a URL response into memory.
func (d *Downloader) downloadBytes(ctx context.Context, fileURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fileURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
	req.Header.Set("Referer", fileURL)

	resp, err := d.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("download request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download failed with status %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read bytes: %w", err)
	}

	return data, nil
}
