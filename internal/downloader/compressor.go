package downloader

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
)

// ErrFFprobeNotFound is returned when ffprobe is not installed on the system.
var ErrFFprobeNotFound = errors.New("ffprobe not found: install ffmpeg package")

// VideoProbe holds video metadata extracted by ffprobe.
type VideoProbe struct {
	Streams []struct {
		Width     int    `json:"width"`
		Height    int    `json:"height"`
		CodecName string `json:"codec_name"`
		BitRate   string `json:"bit_rate"`
	} `json:"streams"`
	Format struct {
		Duration string `json:"duration"`
		BitRate  string `json:"bit_rate"`
	} `json:"format"`
}

// FormatBitrateKbps returns the overall format bitrate in kbps, or 0 if unparseable.
func (v *VideoProbe) FormatBitrateKbps() int {
	if v.Format.BitRate == "" || v.Format.BitRate == "N/A" {
		return 0
	}
	bps, err := strconv.ParseInt(v.Format.BitRate, 10, 64)
	if err != nil {
		return 0
	}
	return int(bps / 1000)
}

// MaxDimension returns the largest width and height across all video streams.
func (v *VideoProbe) MaxDimension() (maxW, maxH int) {
	for _, s := range v.Streams {
		if s.Width > maxW {
			maxW = s.Width
		}
		if s.Height > maxH {
			maxH = s.Height
		}
	}
	return
}

// ProbeVideo runs ffprobe on the given file and returns parsed metadata.
// Returns an error if ffprobe is not found or fails to parse.
func ProbeVideo(filePath string) (*VideoProbe, error) {
	ffprobe, err := ffprobePath()
	if err != nil {
		return nil, err
	}

	cmd := exec.Command(ffprobe,
		"-v", "error",
		"-show_entries", "format=bit_rate,duration",
		"-show_entries", "stream=width,height,codec_name,bit_rate",
		"-of", "json",
		"--", filePath,
	)

	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("ffprobe failed: %w", err)
	}

	var probe VideoProbe
	if err := json.Unmarshal(output, &probe); err != nil {
		return nil, fmt.Errorf("ffprobe parse: %w", err)
	}

	return &probe, nil
}

// ShouldCompress returns true if the video should be compressed based on
// bitrate or resolution thresholds.
func ShouldCompress(info *VideoProbe) bool {
	// Compress if overall bitrate exceeds 3 Mbps
	if info.FormatBitrateKbps() > 3000 {
		return true
	}

	maxW, maxH := info.MaxDimension()

	// Compress if any dimension exceeds Full HD (1920px) — catches 4K, 2.7K, etc.
	if maxW > 1920 || maxH > 1920 {
		return true
	}

	return false
}

// CompressVideo re-encodes a video with WhatsApp-friendly settings:
// H.264 at 2 Mbps, AAC 128k, scaled to max 1280px, faststart.
// Output is written to outputPath.
func CompressVideo(ctx context.Context, inputPath, outputPath string) error {
	ffmpeg, err := ffmpegPath()
	if err != nil {
		return err
	}

	args := []string{
		"-y",
		"-i", inputPath,
		"-c:v", "libx264",
		"-b:v", "2M",
		"-maxrate", "2.5M",
		"-bufsize", "5M",
		"-vf", "scale='min(1280,iw)':'min(1280,ih)':force_original_aspect_ratio=decrease,format=yuv420p",
		"-c:a", "aac",
		"-b:a", "128k",
		"-preset", "fast",
		"-movflags", "+faststart",
		"--", outputPath,
	}

	cmd := exec.CommandContext(ctx, ffmpeg, args...)
	cmd.Stderr = nil // suppress ffmpeg progress output

	if err := cmd.Run(); err != nil {
		// Clean up partial output on failure
		os.Remove(outputPath)
		return fmt.Errorf("ffmpeg: %w", err)
	}

	return nil
}

// ffprobePath returns the path to ffprobe, or ErrFFprobeNotFound if not installed.
func ffprobePath() (string, error) {
	path, err := exec.LookPath("ffprobe")
	if err != nil {
		return "", ErrFFprobeNotFound
	}
	return path, nil
}

// ffmpegPath returns the path to ffmpeg, or an error if not installed.
func ffmpegPath() (string, error) {
	path, err := exec.LookPath("ffmpeg")
	if err != nil {
		return "", fmt.Errorf("ffmpeg not found: install ffmpeg package")
	}
	return path, nil
}
