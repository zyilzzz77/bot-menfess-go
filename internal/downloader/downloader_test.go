package downloader

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"
)

const testTikTokURL = "https://vt.tiktok.com/ZSQjerA3a/"

// TestTikTokDownloader tests video download from TikTok using the real API.
func TestTikTokDownloader(t *testing.T) {
	apiKey := os.Getenv("API_KEY")
	if apiKey == "" {
		apiKey = "OXlJB9"
	}

	dl := NewDownloader(apiKey, "./test_downloads")
	defer os.RemoveAll("./test_downloads")

	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println("🧪 Testing TikTok Video Downloader")
	fmt.Printf("   API Key : %s***\n", apiKey[:3])
	fmt.Printf("   URL     : %s\n", testTikTokURL)
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	start := time.Now()
	result, err := dl.DownloadTikTok(ctx, testTikTokURL)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("❌ Download FAILED after %.1fs: %v", elapsed.Seconds(), err)
	}

	t.Logf("✅ Download SUCCESS after %.1fs", elapsed.Seconds())
	printResult(t, result)
	dl.CleanupAll(result)
}

// TestTikTokAudioDownloader tests audio-only download from TikTok.
func TestTikTokAudioDownloader(t *testing.T) {
	apiKey := os.Getenv("API_KEY")
	if apiKey == "" {
		apiKey = "OXlJB9"
	}

	dl := NewDownloader(apiKey, "./test_downloads")
	defer os.RemoveAll("./test_downloads")

	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println("🧪 Testing TikTok Audio Downloader")
	fmt.Printf("   URL     : %s\n", testTikTokURL)
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	start := time.Now()
	result, err := dl.DownloadTikTokAudio(ctx, testTikTokURL)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("❌ Audio download FAILED after %.1fs: %v", elapsed.Seconds(), err)
	}

	t.Logf("✅ Audio download SUCCESS after %.1fs", elapsed.Seconds())
	printResult(t, result)
	dl.CleanupAll(result)
}

// TestShouldCompress verifies compression decision logic without needing ffmpeg.
func TestShouldCompress(t *testing.T) {
	tests := []struct {
		name     string
		bitrate  string // format bit_rate in bps
		width    int
		height   int
		expected bool
	}{
		{"low bitrate 720p", "1500000", 1280, 720, false},
		{"high bitrate 720p", "5000000", 1280, 720, true},
		{"low bitrate 1080p", "2000000", 1920, 1080, false},
		{"high bitrate 1080p", "4000000", 1920, 1080, true},
		{"4K video", "1500000", 3840, 2160, true},     // high res triggers compress
		{"vertical 1080x1920 low bitrate", "2000000", 1080, 1920, false}, // 1920 height is at threshold, not over
		{"vertical 720x1280 low bitrate", "2000000", 720, 1280, false},  // under all thresholds
		{"2.7K video low bitrate", "2000000", 2704, 1520, true},         // width > 1920 triggers compress regardless of bitrate
		{"N/A bitrate", "N/A", 1920, 1080, false},      // unparseable bitrate, ok res
		{"empty bitrate", "", 1920, 1080, false},        // missing bitrate, ok res
		{"exactly 3000 kbps", "3000000", 1280, 720, false}, // threshold boundary
		{"3001 kbps", "3001000", 1280, 720, true},       // just over threshold
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			probe := &VideoProbe{}
			probe.Format.BitRate = tt.bitrate
			probe.Streams = []struct {
				Width     int    `json:"width"`
				Height    int    `json:"height"`
				CodecName string `json:"codec_name"`
				BitRate   string `json:"bit_rate"`
			}{
				{Width: tt.width, Height: tt.height, CodecName: "h264"},
			}

			got := ShouldCompress(probe)
			if got != tt.expected {
				t.Errorf("ShouldCompress() = %v, want %v (bitrate=%s, %dx%d)",
					got, tt.expected, tt.bitrate, tt.width, tt.height)
			}
		})
	}
}

func printResult(t *testing.T, result *DownloadResult) {
	t.Logf("   Title : %s", result.Title)
	t.Logf("   Items : %d", len(result.Items))
	for i, item := range result.Items {
		sizeKB := float64(item.FileSize) / 1024.0
		sizeMB := sizeKB / 1024.0
		typeStr := "???"
		switch item.MediaType {
		case MediaTypeVideo:
			typeStr = "VIDEO"
		case MediaTypeImage:
			typeStr = "IMAGE"
		case MediaTypeAudio:
			typeStr = "AUDIO"
		}
		t.Logf("   [%d] %s | %s | %.1f MB (%.0f KB)",
			i+1, typeStr, item.FileName, sizeMB, sizeKB)
	}
}
