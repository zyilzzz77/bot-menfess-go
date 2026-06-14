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

// igResponse represents the API response from neoxr ig endpoint
type igResponse struct {
	Creator string `json:"creator"`
	Status  bool   `json:"status"`
	Msg     string `json:"msg"`
	Data    []struct {
		Type string `json:"type"`
		URL  string `json:"url"`
	} `json:"data"`
}

// DownloadInstagram downloads media from Instagram using the neoxr ig API
func (d *Downloader) DownloadInstagram(ctx context.Context, postURL string) (*DownloadResult, error) {
	// Build API URL
	apiURL := fmt.Sprintf("%s/ig?url=%s&apikey=%s",
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

	var apiResp igResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return nil, fmt.Errorf("failed to parse API response: %w", err)
	}

	if !apiResp.Status {
		msg := apiResp.Msg
		if msg == "" {
			msg = "unknown error"
		}
		return nil, fmt.Errorf("API error: %s", msg)
	}

	if len(apiResp.Data) == 0 {
		return nil, fmt.Errorf("no media found in API response")
	}

	result := &DownloadResult{
		Title: "Instagram Media",
	}

	prefix := fmt.Sprintf("%d", time.Now().UnixNano())

	for i, item := range apiResp.Data {
		if item.URL == "" {
			continue
		}

		// Determine media type and extension from the type field
		mediaType, ext := parseIGMediaType(item.Type)
		fileName := fmt.Sprintf("%s_ig_%d%s", prefix, i+1, ext)
		filePath := filepath.Join(d.DownloadDir, fileName)

		if err := d.downloadFile(ctx, item.URL, filePath); err != nil {
			fmt.Printf("⚠️ Failed to download IG media %d: %v\n", i+1, err)
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
		result.Title = fmt.Sprintf("Instagram Carousel (%d media)", len(result.Items))
	} else if len(result.Items) == 1 {
		switch result.Items[0].MediaType {
		case MediaTypeVideo:
			result.Title = "Instagram Video"
		case MediaTypeImage:
			result.Title = "Instagram Photo"
		default:
			result.Title = "Instagram Media"
		}
	}

	if len(result.Items) == 0 {
		return nil, fmt.Errorf("failed to download any media from Instagram")
	}

	return result, nil
}

// parseIGMediaType determines the MediaType and file extension from the IG API type field
func parseIGMediaType(typeStr string) (MediaType, string) {
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
	case t == "mp3" || t == "audio":
		return MediaTypeAudio, ".mp3"
	default:
		// Default to video for unknown types (most IG content is video)
		if strings.Contains(t, "video") || strings.Contains(t, "mp4") {
			return MediaTypeVideo, ".mp4"
		}
		return MediaTypeImage, ".jpg"
	}
}
