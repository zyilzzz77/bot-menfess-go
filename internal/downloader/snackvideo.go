package downloader

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"time"
)

// snackvidResponse represents the API response from neoxr snackvid endpoint
type snackvidResponse struct {
	Creator string `json:"creator"`
	Status  bool   `json:"status"`
	Data    struct {
		Thumbnail []string `json:"thumbnail"`
		Author    string   `json:"author"`
		Caption   string   `json:"caption"`
		URL       string   `json:"url"`
	} `json:"data"`
}

// DownloadSnackVideo downloads media from Snack Video using the neoxr snackvid API
func (d *Downloader) DownloadSnackVideo(ctx context.Context, videoURL string) (*DownloadResult, error) {
	// Build API URL
	apiURL := fmt.Sprintf("%s/snackvid?url=%s&apikey=%s",
		d.BaseURL,
		url.QueryEscape(videoURL),
		d.APIKey,
	)

	// Call API
	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := d.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	var apiResp snackvidResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return nil, fmt.Errorf("failed to parse API response: %w", err)
	}

	if !apiResp.Status {
		return nil, fmt.Errorf("API returned error status")
	}

	if apiResp.Data.URL == "" {
		return nil, fmt.Errorf("no video URL found in API response")
	}

	// Build title from author and caption
	title := "Snack Video"
	if apiResp.Data.Author != "" {
		title = fmt.Sprintf("Snack Video — %s", apiResp.Data.Author)
	}

	prefix := fmt.Sprintf("%d", time.Now().UnixNano())
	fileName := fmt.Sprintf("%s_snackvid.mp4", prefix)
	filePath := filepath.Join(d.DownloadDir, fileName)

	// Download the video
	if err := d.downloadFile(ctx, apiResp.Data.URL, filePath); err != nil {
		return nil, fmt.Errorf("failed to download video: %w", err)
	}

	fileInfo, err := os.Stat(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to stat file: %w", err)
	}

	result := &DownloadResult{
		Title: title,
		Items: []*MediaItem{
			{
				FilePath:  filePath,
				FileName:  fileName,
				MediaType: MediaTypeVideo,
				FileSize:  fileInfo.Size(),
			},
		},
	}

	return result, nil
}
