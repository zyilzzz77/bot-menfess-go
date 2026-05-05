package utils

import (
	"net/url"
	"regexp"
	"strings"
)

// Platform represents a social media platform
type Platform string

const (
	PlatformTikTok           Platform = "tiktok"
	PlatformInstagram        Platform = "instagram"
	PlatformFacebook         Platform = "facebook"
	PlatformTwitter          Platform = "twitter"
	PlatformSnackVideo       Platform = "snackvideo"
	PlatformSpotify          Platform = "spotify"
	PlatformSpotifyPlaylist  Platform = "spotify_playlist"
	PlatformThreads          Platform = "threads"
	PlatformUnknown          Platform = "unknown"
)

// PlatformInfo holds information about a detected social media link
type PlatformInfo struct {
	Platform Platform
	URL      string
	Label    string
}

// platformPatterns maps regex patterns to social media platforms
var platformPatterns = map[Platform][]*regexp.Regexp{
	PlatformTikTok: {
		regexp.MustCompile(`(?i)https?://(?:www\.)?tiktok\.com/@[^/]+/video/\d+`),
		regexp.MustCompile(`(?i)https?://(?:vm|vt)\.tiktok\.com/\w+`),
		regexp.MustCompile(`(?i)https?://(?:www\.)?tiktok\.com/t/\w+`),
	},
	PlatformInstagram: {
		regexp.MustCompile(`(?i)https?://(?:www\.)?instagram\.com/(?:p|reel|reels|tv)/[\w-]+`),
		regexp.MustCompile(`(?i)https?://(?:www\.)?instagram\.com/stories/[\w.]+/\d+`),
	},
	PlatformFacebook: {
		regexp.MustCompile(`(?i)https?://(?:www\.)?facebook\.com/.+/videos/\d+`),
		regexp.MustCompile(`(?i)https?://(?:www\.)?facebook\.com/watch/?\?v=\d+`),
		regexp.MustCompile(`(?i)https?://(?:www\.)?facebook\.com/reel/\d+`),
		regexp.MustCompile(`(?i)https?://fb\.watch/\w+`),
		regexp.MustCompile(`(?i)https?://(?:www\.)?facebook\.com/share/v/\w+`),
		regexp.MustCompile(`(?i)https?://(?:www\.)?facebook\.com/\d+/videos/\d+`),
	},
	PlatformTwitter: {
		regexp.MustCompile(`(?i)https?://(?:www\.)?(?:twitter|x)\.com/\w+/status/\d+`),
		regexp.MustCompile(`(?i)https?://t\.co/\w+`),
	},
	PlatformSnackVideo: {
		regexp.MustCompile(`(?i)https?://(?:www\.)?snackvideo\.com/[\w/@-]+`),
		regexp.MustCompile(`(?i)https?://sck\.io/p/[\w-]+`),
		regexp.MustCompile(`(?i)https?://sck\.io/[\w-]+`),
	},
	PlatformSpotifyPlaylist: {
		regexp.MustCompile(`(?i)https?://open\.spotify\.com/playlist/[\w]+`),
		regexp.MustCompile(`(?i)https?://open\.spotify\.com/intl-[\w]+/playlist/[\w]+`),
	},
	PlatformSpotify: {
		regexp.MustCompile(`(?i)https?://open\.spotify\.com/track/[\w]+`),
		regexp.MustCompile(`(?i)https?://open\.spotify\.com/intl-[\w]+/track/[\w]+`),
		regexp.MustCompile(`(?i)https?://spotify\.link/[\w]+`),
	},
	PlatformThreads: {
		regexp.MustCompile(`(?i)https?://(?:www\.)?threads\.net/@[\w.]+/post/[\w-]+`),
		regexp.MustCompile(`(?i)https?://(?:www\.)?threads\.net/t/[\w-]+`),
		regexp.MustCompile(`(?i)https?://(?:www\.)?threads\.com/@[\w.]+/post/[\w-]+`),
	},
}

// platformLabels provides human-readable names for platforms
var platformLabels = map[Platform]string{
	PlatformTikTok:          "🎵 TikTok",
	PlatformInstagram:       "📸 Instagram",
	PlatformFacebook:        "📘 Facebook",
	PlatformTwitter:         "🐦 Twitter/X",
	PlatformSnackVideo:      "🎬 Snack Video",
	PlatformSpotify:         "🎧 Spotify",
	PlatformSpotifyPlaylist: "🎧 Spotify Playlist",
	PlatformThreads:         "🧵 Threads",
}

// urlPattern is a general pattern to find URLs in text
var urlPattern = regexp.MustCompile(`https?://[^\s<>"{}|\\^\x60]+`)

// DetectSocialMediaURL checks if a message contains a supported social media URL
// and returns the platform info. Returns nil if no supported URL is found.
func DetectSocialMediaURL(message string) *PlatformInfo {
	// Find all URLs in the message
	urls := urlPattern.FindAllString(message, -1)
	if len(urls) == 0 {
		return nil
	}

	for _, rawURL := range urls {
		// Clean the URL
		rawURL = strings.TrimSpace(rawURL)

		// Validate it's a proper URL
		if _, err := url.ParseRequestURI(rawURL); err != nil {
			continue
		}

		// Check against platform patterns
		for platform, patterns := range platformPatterns {
			for _, pattern := range patterns {
				if pattern.MatchString(rawURL) {
					return &PlatformInfo{
						Platform: platform,
						URL:      rawURL,
						Label:    platformLabels[platform],
					}
				}
			}
		}
	}

	return nil
}

// IsSupportedURL checks if a URL is from a supported platform
func IsSupportedURL(rawURL string) bool {
	return DetectSocialMediaURL(rawURL) != nil
}
