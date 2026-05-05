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

// spotifyResponse represents the API response from neoxr spotify endpoint
type spotifyResponse struct {
	Creator string `json:"creator"`
	Status  bool   `json:"status"`
	Data    struct {
		Thumbnail string `json:"thumbnail"`
		Title     string `json:"title"`
		Artist    string `json:"artist"`
		Duration  string `json:"duration"`
		Preview   string `json:"preview"`
		URL       string `json:"url"`
	} `json:"data"`
}

// DownloadSpotify downloads a track from Spotify using the neoxr spotify API
// Sends both the cover art (thumbnail) and the audio file
func (d *Downloader) DownloadSpotify(ctx context.Context, trackURL string) (*DownloadResult, error) {
	// Build API URL
	apiURL := fmt.Sprintf("%s/spotify?url=%s&apikey=%s",
		d.BaseURL,
		url.QueryEscape(trackURL),
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

	var apiResp spotifyResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return nil, fmt.Errorf("failed to parse API response: %w", err)
	}

	if !apiResp.Status {
		return nil, fmt.Errorf("API returned error status")
	}

	// Build title
	title := apiResp.Data.Title
	if apiResp.Data.Artist != "" {
		title = fmt.Sprintf("%s — %s", apiResp.Data.Title, apiResp.Data.Artist)
	}
	if apiResp.Data.Duration != "" {
		title = fmt.Sprintf("%s [%s]", title, apiResp.Data.Duration)
	}

	result := &DownloadResult{
		Title: title,
	}

	prefix := fmt.Sprintf("%d", time.Now().UnixNano())

	// 1. Download thumbnail (cover art)
	if apiResp.Data.Thumbnail != "" {
		thumbName := fmt.Sprintf("%s_spotify_cover.jpg", prefix)
		thumbPath := filepath.Join(d.DownloadDir, thumbName)

		if err := d.downloadFile(ctx, apiResp.Data.Thumbnail, thumbPath); err != nil {
			fmt.Printf("⚠️ Failed to download Spotify cover: %v\n", err)
		} else {
			fileInfo, _ := os.Stat(thumbPath)
			result.Items = append(result.Items, &MediaItem{
				FilePath:  thumbPath,
				FileName:  thumbName,
				MediaType: MediaTypeImage,
				FileSize:  fileInfo.Size(),
			})
		}
	}

	// 2. Download full audio (preferred) or preview
	audioURL := apiResp.Data.URL
	if audioURL == "" {
		audioURL = apiResp.Data.Preview
	}

	if audioURL != "" {
		audioName := fmt.Sprintf("%s_spotify_audio.mp3", prefix)
		audioPath := filepath.Join(d.DownloadDir, audioName)

		if err := d.downloadFile(ctx, audioURL, audioPath); err != nil {
			return nil, fmt.Errorf("failed to download audio: %w", err)
		}

		fileInfo, _ := os.Stat(audioPath)
		result.Items = append(result.Items, &MediaItem{
			FilePath:  audioPath,
			FileName:  audioName,
			MediaType: MediaTypeAudio,
			FileSize:  fileInfo.Size(),
		})
	}

	if len(result.Items) == 0 {
		return nil, fmt.Errorf("no downloadable media found in Spotify API response")
	}

	return result, nil
}
