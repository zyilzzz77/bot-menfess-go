package downloader

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// threadsResponse represents the API response from neoxr threads endpoint
type threadsResponse struct {
	Creator string `json:"creator"`
	Status  bool   `json:"status"`
	Data    []struct {
		Type string `json:"type"`
		URL  string `json:"url"`
	} `json:"data"`
}

// DownloadThreads downloads media from Threads using the neoxr threads API
func (d *Downloader) DownloadThreads(ctx context.Context, postURL string) (*DownloadResult, error) {
	// Build API URL
	apiURL := fmt.Sprintf("%s/threads?url=%s&apikey=%s",
		d.BaseURL,
		url.QueryEscape(postURL),
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

	var apiResp threadsResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return nil, fmt.Errorf("failed to parse API response: %w", err)
	}

	if !apiResp.Status {
		return nil, fmt.Errorf("API returned error status")
	}

	if len(apiResp.Data) == 0 {
		return nil, fmt.Errorf("no media found in API response")
	}

	result := &DownloadResult{
		Title: "Threads Media",
	}

	prefix := fmt.Sprintf("%d", time.Now().UnixNano())

	for i, item := range apiResp.Data {
		if item.URL == "" {
			continue
		}

		mediaType, ext := parseThreadsMediaType(item.Type)
		fileName := fmt.Sprintf("%s_threads_%d%s", prefix, i+1, ext)
		filePath := filepath.Join(d.DownloadDir, fileName)

		if err := d.downloadFile(ctx, item.URL, filePath); err != nil {
			fmt.Printf("⚠️ Failed to download Threads media %d: %v\n", i+1, err)
			continue
		}

		fileInfo, err := os.Stat(filePath)
		if err != nil {
			continue
		}

		result.Items = append(result.Items, &MediaItem{
			FilePath:  filePath,
			FileName:  fileName,
			MediaType: mediaType,
			FileSize:  fileInfo.Size(),
		})
	}

	// Update title based on content
	if len(result.Items) > 1 {
		result.Title = fmt.Sprintf("Threads Carousel (%d media)", len(result.Items))
	} else if len(result.Items) == 1 {
		switch result.Items[0].MediaType {
		case MediaTypeVideo:
			result.Title = "Threads Video"
		case MediaTypeImage:
			result.Title = "Threads Photo"
		default:
			result.Title = "Threads Media"
		}
	}

	if len(result.Items) == 0 {
		return nil, fmt.Errorf("failed to download any media from Threads")
	}

	return result, nil
}

// parseThreadsMediaType determines the MediaType and file extension from the type field
func parseThreadsMediaType(typeStr string) (MediaType, string) {
	t := strings.ToLower(strings.TrimSpace(typeStr))
	switch {
	case t == "mp4" || t == "video":
		return MediaTypeVideo, ".mp4"
	case t == "jpg" || t == "jpeg":
		return MediaTypeImage, ".jpg"
	case t == "png":
		return MediaTypeImage, ".png"
	case t == "webp":
		return MediaTypeImage, ".webp"
	default:
		return MediaTypeImage, ".jpg"
	}
}
