package downloader

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// MediaType represents the type of downloaded media
type MediaType int

const (
	MediaTypeVideo MediaType = iota
	MediaTypeImage
	MediaTypeAudio
)

// MediaItem holds information about a single downloaded media file
type MediaItem struct {
	FilePath  string
	FileName  string
	MediaType MediaType
	FileSize  int64
}

// DownloadResult holds the overall download result (may contain multiple media)
type DownloadResult struct {
	Items []*MediaItem
	Title string
}

// snaptikResponse represents the API response from neoxr snaptik-v2
type snaptikResponse struct {
	Creator string `json:"creator"`
	Status  bool   `json:"status"`
	Msg     string `json:"msg"`
	Data    struct {
		Video   string   `json:"video"`
		VideoHD string   `json:"videoHD"`
		Audio   string   `json:"audio"`
		Photo   []string `json:"photo"`
	} `json:"data"`
}

// Downloader handles downloading media from social media via REST APIs
type Downloader struct {
	APIKey      string
	BaseURL     string
	DownloadDir string
	httpClient  *http.Client
}

// NewDownloader creates a new Downloader instance
func NewDownloader(apiKey string, downloadDir string) *Downloader {
	os.MkdirAll(downloadDir, 0755)

	return &Downloader{
		APIKey:      apiKey,
		BaseURL:     "https://api.neoxr.eu/api",
		DownloadDir: downloadDir,
		httpClient: &http.Client{
			Timeout: 120 * time.Second,
			Transport: &http.Transport{
				Proxy: http.ProxyFromEnvironment,
				DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
					// Force IPv4 to avoid "network is unreachable" on IPv6-only hosts
					return (&net.Dialer{
						Timeout:   30 * time.Second,
						KeepAlive: 30 * time.Second,
					}).DialContext(ctx, "tcp4", addr)
				},
				ForceAttemptHTTP2:     true,
				MaxIdleConns:          100,
				MaxIdleConnsPerHost:   20,
				MaxConnsPerHost:       20,
				IdleConnTimeout:       90 * time.Second,
				TLSHandshakeTimeout:   15 * time.Second,
				ResponseHeaderTimeout: 30 * time.Second,
				ExpectContinueTimeout: 1 * time.Second,
				DisableCompression:    true,
			},
		},
	}
}

// DownloadTikTok downloads media from TikTok using the snaptik-v2 API
func (d *Downloader) DownloadTikTok(ctx context.Context, videoURL string) (*DownloadResult, error) {
	// Build API URL
	apiURL := fmt.Sprintf("%s/snaptik-v2?url=%s&apikey=%s",
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

	var apiResp snaptikResponse
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

	result := &DownloadResult{
		Title: "TikTok Video",
	}

	prefix := fmt.Sprintf("%d", time.Now().UnixNano())

	// Check if it's a photo slideshow
	if len(apiResp.Data.Photo) > 0 {
		result.Title = "TikTok Photos"
		for i, photoURL := range apiResp.Data.Photo {
			if photoURL == "" {
				continue
			}
			fileName := fmt.Sprintf("%s_photo_%d.jpg", prefix, i+1)
			filePath := filepath.Join(d.DownloadDir, fileName)

			if err := d.downloadFile(ctx, photoURL, filePath); err != nil {
				fmt.Printf("⚠️ Failed to download photo %d: %v\n", i+1, err)
				continue
			}

			fileInfo, _ := os.Stat(filePath)
			result.Items = append(result.Items, &MediaItem{
				FilePath:  filePath,
				FileName:  fileName,
				MediaType: MediaTypeImage,
				FileSize:  fileInfo.Size(),
			})
		}
	} else {
		// It's a video — prefer HD version
		videoURL := apiResp.Data.VideoHD
		if videoURL == "" {
			videoURL = apiResp.Data.Video
		}

		if videoURL != "" {
			fileName := fmt.Sprintf("%s_video.mp4", prefix)
			filePath := filepath.Join(d.DownloadDir, fileName)

			if err := d.downloadFile(ctx, videoURL, filePath); err != nil {
				return nil, fmt.Errorf("failed to download video: %w", err)
			}

			fileInfo, _ := os.Stat(filePath)
			result.Items = append(result.Items, &MediaItem{
				FilePath:  filePath,
				FileName:  fileName,
				MediaType: MediaTypeVideo,
				FileSize:  fileInfo.Size(),
			})
		}
	}

	// Also download audio if available and no video was downloaded
	if len(result.Items) == 0 && apiResp.Data.Audio != "" {
		fileName := fmt.Sprintf("%s_audio.mp3", prefix)
		filePath := filepath.Join(d.DownloadDir, fileName)

		if err := d.downloadFile(ctx, apiResp.Data.Audio, filePath); err != nil {
			return nil, fmt.Errorf("failed to download audio: %w", err)
		}

		fileInfo, _ := os.Stat(filePath)
		result.Items = append(result.Items, &MediaItem{
			FilePath:  filePath,
			FileName:  fileName,
			MediaType: MediaTypeAudio,
			FileSize:  fileInfo.Size(),
		})
	}

	if len(result.Items) == 0 {
		return nil, fmt.Errorf("no downloadable media found in API response")
	}

	return result, nil
}

// DownloadTikTokAudio downloads only the audio from a TikTok video (tomp3)
func (d *Downloader) DownloadTikTokAudio(ctx context.Context, videoURL string) (*DownloadResult, error) {
	// Build API URL (same endpoint)
	apiURL := fmt.Sprintf("%s/snaptik-v2?url=%s&apikey=%s",
		d.BaseURL,
		url.QueryEscape(videoURL),
		d.APIKey,
	)

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := d.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	var apiResp snaptikResponse
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

	if apiResp.Data.Audio == "" {
		return nil, fmt.Errorf("audio tidak tersedia untuk video ini")
	}

	prefix := fmt.Sprintf("%d", time.Now().UnixNano())
	fileName := fmt.Sprintf("%s_tiktok_audio.mp3", prefix)
	filePath := filepath.Join(d.DownloadDir, fileName)

	if err := d.downloadFile(ctx, apiResp.Data.Audio, filePath); err != nil {
		return nil, fmt.Errorf("failed to download audio: %w", err)
	}

	fileInfo, _ := os.Stat(filePath)
	return &DownloadResult{
		Title: "TikTok Audio (MP3)",
		Items: []*MediaItem{
			{
				FilePath:  filePath,
				FileName:  fileName,
				MediaType: MediaTypeAudio,
				FileSize:  fileInfo.Size(),
			},
		},
	}, nil
}

// DownloadGeneric attempts to download from any supported platform
// For now, routes to the appropriate API based on platform
func (d *Downloader) DownloadGeneric(ctx context.Context, platform string, mediaURL string) (*DownloadResult, error) {
	switch strings.ToLower(platform) {
	case "tiktok":
		return d.DownloadTikTok(ctx, mediaURL)
	case "instagram":
		return d.DownloadInstagram(ctx, mediaURL)
	case "snackvideo":
		return d.DownloadSnackVideo(ctx, mediaURL)
	case "spotify":
		return d.DownloadSpotify(ctx, mediaURL)
	case "twitter":
		return d.DownloadTwitter(ctx, mediaURL)
	case "threads":
		return d.DownloadThreads(ctx, mediaURL)
	default:
		return nil, fmt.Errorf("platform '%s' belum didukung via API. Silakan tunggu update berikutnya", platform)
	}
}

// downloadFile downloads a file from a URL and saves it to the given path
func (d *Downloader) downloadFile(ctx context.Context, fileURL string, destPath string) error {
	req, err := http.NewRequestWithContext(ctx, "GET", fileURL, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	// Set headers to mimic a browser
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
	req.Header.Set("Referer", fileURL)

	resp, err := d.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("download request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed with status %d", resp.StatusCode)
	}

	outFile, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("create file: %w", err)
	}
	defer outFile.Close()

	_, err = io.CopyBuffer(outFile, resp.Body, make([]byte, 256*1024))
	if err != nil {
		os.Remove(destPath)
		return fmt.Errorf("write file: %w", err)
	}

	return nil
}

// CleanupAll removes all files from a download result
func (d *Downloader) CleanupAll(result *DownloadResult) {
	for _, item := range result.Items {
		os.Remove(item.FilePath)
	}
}
