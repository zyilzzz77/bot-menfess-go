package bot

import (
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"bot-wa/internal/ai"
	"bot-wa/internal/downloader"
	"bot-wa/internal/utils"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types/events"
)

// sendDelay is the delay between sending multiple media items to avoid ban
const sendDelay = 3 * time.Second

// progressInterval is how often the progress message gets updated
const progressInterval = 2 * time.Second

// duplicateCacheExpiry is how long a URL is remembered to prevent duplicate downloads
const duplicateCacheExpiry = 30 * time.Minute

// tomp3Pattern matches the tomp3(url) command
var tomp3Pattern = regexp.MustCompile(`(?i)^tomp3\s*\(\s*(https?://[^\s)]+)\s*\)$`)

// pendingPlaylist stores a playlist waiting for user selection
type pendingPlaylist struct {
	Playlist  *downloader.SpotifyPlaylistResult
	URL       string
	CreatedAt time.Time
}

// cachedURL stores info about a recently downloaded URL
type cachedURL struct {
	URL       string
	Platform  string
	CreatedAt time.Time
}

// Handler processes incoming WhatsApp messages
type Handler struct {
	client           *whatsmeow.Client
	downloader       *downloader.Downloader
	pendingPlaylists map[string]*pendingPlaylist // key: chat JID string
	playlistMu       sync.RWMutex

	menfessThreads      map[string]*menfessThread // key: bot message ID -> thread
	menfessChatMessages map[string]string         // key: chat JID -> latest bot message ID
	menfessMu           sync.RWMutex

	// Anti-spam: track recently downloaded URLs per chat
	recentURLs map[string]map[string]*cachedURL // key: chatJID -> url -> info
	urlCacheMu sync.RWMutex

	// Sticker: track users waiting to send image for sticker
	pendingStickers map[string]time.Time // key: chatJID -> when requested
	stickerMu       sync.RWMutex

	// AI: DeepSeek-powered chat assistant
	ai             *ai.Client           // nil if not configured
	pendingAIAudio map[string]time.Time // key: chatJID -> when /ai was sent empty
	aiAudioMu      sync.RWMutex
	aiRateLimit    map[string]time.Time // key: chatJID -> last AI query time
	aiRateLimitMu  sync.RWMutex

	// TikTok audio button: tracks which button maps to which TikTok URL
	tiktokAudioButtons map[string]string // key: buttonID (ta_<ts>) -> tiktokURL, cleaned after 10min
	tiktokAudioMu      sync.RWMutex
}

// NewHandler creates a new message handler
func NewHandler(client *whatsmeow.Client, dl *downloader.Downloader, aiClient *ai.Client) *Handler {
	return &Handler{
		client:              client,
		downloader:          dl,
		ai:                  aiClient,
		pendingPlaylists:    make(map[string]*pendingPlaylist),
		menfessThreads:      make(map[string]*menfessThread),
		menfessChatMessages: make(map[string]string),
		recentURLs:          make(map[string]map[string]*cachedURL),
		pendingStickers:     make(map[string]time.Time),
		pendingAIAudio:      make(map[string]time.Time),
		aiRateLimit:         make(map[string]time.Time),
		tiktokAudioButtons:  make(map[string]string),
	}
}

// HandleMessage processes a single incoming message
func (h *Handler) HandleMessage(evt *events.Message) {
	if evt.Info.IsFromMe {
		return
	}

	if evt.Info.Chat.Server == "broadcast" {
		return
	}

	chat := evt.Info.Chat
	chatKey := chat.String()

	if h.handleMenfessReply(evt) {
		return
	}

	// Check for TikTok audio button tap
	if h.handleTikTokAudioButton(evt) {
		return
	}

	imgMsg := evt.Message.GetImageMessage()
	if imgMsg != nil {
		caption := strings.TrimSpace(imgMsg.GetCaption())

		// AI image query: caption starts with /ai or .ai
		if h.ai != nil {
			if aiText, ok := parseAICommand(caption); ok {
				go h.handleAIImageCommand(evt, imgMsg, aiText)
				return
			}
		}

		captionLower := strings.ToLower(caption)
		if captionLower == "s" || captionLower == "sticker" {
			go h.handleStickerFromImage(evt, imgMsg)
			return
		}

		h.stickerMu.RLock()
		stickerTime, hasPendingSticker := h.pendingStickers[chatKey]
		h.stickerMu.RUnlock()
		if hasPendingSticker && time.Since(stickerTime) < 2*time.Minute {
			h.stickerMu.Lock()
			delete(h.pendingStickers, chatKey)
			h.stickerMu.Unlock()
			go h.handleStickerFromImage(evt, imgMsg)
			return
		}
		return
	}

	// AI audio query: voice note after /ai (empty) command
	if h.ai != nil {
		audioMsg := evt.Message.GetAudioMessage()
		if audioMsg != nil {
			h.aiAudioMu.RLock()
			_, hasPending := h.pendingAIAudio[chatKey]
			h.aiAudioMu.RUnlock()
			if hasPending {
				h.aiAudioMu.Lock()
				delete(h.pendingAIAudio, chatKey)
				h.aiAudioMu.Unlock()
				go h.handleAIAudioCommand(evt, audioMsg)
				return
			}
		}
	}

	msg := ""
	if evt.Message.GetConversation() != "" {
		msg = evt.Message.GetConversation()
	} else if evt.Message.GetExtendedTextMessage() != nil {
		msg = evt.Message.GetExtendedTextMessage().GetText()
	}

	if msg == "" {
		return
	}

	msgLower := strings.TrimSpace(strings.ToLower(msg))
	switch {
	case msgLower == "/help" || msgLower == "/start" || msgLower == "!help":
		h.sendHelp(evt)
		return
	case msgLower == "/ping":
		h.sendText(chat, "🏓 Pong! Bot is alive.")
		return
	case msgLower == "s" || msgLower == "sticker":
		h.stickerMu.Lock()
		h.pendingStickers[chatKey] = time.Now()
		h.stickerMu.Unlock()
		return
	}

	trimmedMsg := strings.TrimSpace(msg)
	if targetJID, menfessBody, err := parseMenfessCommand(trimmedMsg); err == nil {
		go h.handleMenfessCommand(evt, targetJID, menfessBody)
		return
	} else if strings.HasPrefix(strings.ToLower(trimmedMsg), "menfess") {
		h.sendText(chat, fmt.Sprintf("❌ %s\n\nFormat: menfess 6281234567890 pesan kamu", err.Error()))
		return
	}

	if bratText, ok := parseBratCommand(msg); ok {
		go h.handleBratCommand(evt, bratText)
		return
	}

	// AI chat commands (thinking mode via DeepSeek v4-pro)
	if h.ai != nil {
		// AI chat: /ai or .ai
		if aiText, ok := parseAICommand(msg); ok {
			if !h.checkAIRateLimit(chatKey) {
				h.sendText(chat, "⏳ Mohon tunggu beberapa detik sebelum bertanya lagi.")
				return
			}
			if aiText == "" {
				// Empty — set pending audio state
				h.aiAudioMu.Lock()
				h.pendingAIAudio[chatKey] = time.Now()
				h.aiAudioMu.Unlock()
				h.sendText(chat, "🎤 *Mode AI Suara*\n\nKirim voice note/audio sekarang untuk ditranskrip dan dijawab AI.\n\n_Ketik /ai <pertanyaan> untuk tanya pakai teks._\n\n⏰ Timeout: 2 menit")
				return
			}
			go h.handleAICommand(evt, aiText)
			return
		}
	}

	if matches := tomp3Pattern.FindStringSubmatch(strings.TrimSpace(msg)); len(matches) == 2 {
		tiktokURL := matches[1]
		platform := utils.DetectSocialMediaURL(tiktokURL)
		if platform != nil && platform.Platform == utils.PlatformTikTok {
			if h.isDuplicateURL(chatKey, tiktokURL) {
				h.sendText(chat, fmt.Sprintf(
					"⚠️ *Link sudah pernah di-download!*\n\n🔗 %s\n\n_Link ini sudah di-download sebelumnya. Kirim link baru atau tunggu 30 menit._",
					tiktokURL,
				))
				fmt.Printf("🚫 [Anti-Spam] Duplicate TikTok MP3 blocked from %s: %s\n", evt.Info.Sender.User, tiktokURL)
				return
			}
			go h.processTikTokAudio(evt, tiktokURL)
			return
		}
		h.sendText(chat, "❌ URL bukan TikTok. Command `tomp3()` hanya untuk link TikTok.")
		return
	}

	h.playlistMu.RLock()
	pending, hasPending := h.pendingPlaylists[chatKey]
	h.playlistMu.RUnlock()

	if hasPending {
		if time.Since(pending.CreatedAt) > 10*time.Minute {
			h.playlistMu.Lock()
			delete(h.pendingPlaylists, chatKey)
			h.playlistMu.Unlock()
		} else {
			switch {
			case msgLower == "all" || msgLower == "semua":
				h.playlistMu.Lock()
				delete(h.pendingPlaylists, chatKey)
				h.playlistMu.Unlock()
				go h.processPlaylistDownload(evt, pending, nil)
				return
			case msgLower == "cancel" || msgLower == "batal":
				h.playlistMu.Lock()
				delete(h.pendingPlaylists, chatKey)
				h.playlistMu.Unlock()
				h.sendText(chat, "❌ Download playlist dibatalkan.")
				return
			default:
				indices := parseTrackSelection(msgLower, len(pending.Playlist.Tracks))
				if len(indices) > 0 {
					h.playlistMu.Lock()
					delete(h.pendingPlaylists, chatKey)
					h.playlistMu.Unlock()
					go h.processPlaylistDownload(evt, pending, indices)
					return
				}
			}
		}
	}

	platform := utils.DetectSocialMediaURL(msg)
	if platform == nil {
		// No social media link — route to AI (Hermes + DeepSeek)
		if h.ai != nil {
			if !h.checkAIRateLimit(chatKey) {
				h.sendText(chat, "⏳ Mohon tunggu beberapa detik...")
				return
			}
			go h.handleAICommand(evt, msg)
			return
		}
		// AI not configured — ignore
		return
	}

	if platform.Platform == utils.PlatformSpotifyPlaylist {
		go h.handleSpotifyPlaylist(evt, platform)
		return
	}

	if h.isDuplicateURL(chatKey, platform.URL) {
		h.sendText(chat, fmt.Sprintf(
			"⚠️ *Link sudah pernah di-download!*\n\n🔗 %s\n\n_Link ini sudah di-download dalam 30 menit terakhir. Kirim link baru atau tunggu 30 menit._",
			platform.URL,
		))
		fmt.Printf("🚫 [Anti-Spam] Duplicate URL blocked from %s: %s\n", evt.Info.Sender.User, platform.URL)
		return
	}

	h.processDownload(evt, platform)
}

// sendHelp sends the help message
func (h *Handler) sendHelp(evt *events.Message) {
	help := `🤖 *WhatsApp Social Media Downloader*

📥 Kirim link dari platform berikut dan saya akan download video/foto untuk kamu!

*Platform yang didukung:*
🎵 TikTok — Video, Photo Slideshow, Audio
📸 Instagram — Reels, Posts, Carousel
🎬 Snack Video — Video
🎧 Spotify — Lagu (Cover + Audio)
🎧 Spotify Playlist — Download banyak lagu sekaligus
🧵 Threads — Photos, Videos, Carousel
🐦 Twitter/X — Photos, Videos, Carousel

*AI (Hermes + DeepSeek v4-pro):*
/ai <pertanyaan> — Tanya AI (thinking mode)
.ai <pertanyaan> — Sama, dengan prefix titik
Kirim gambar + caption /ai — Analisis gambar

*Commands Lainnya:*
/help — Tampilkan pesan ini
/ping — Cek apakah bot aktif
tomp3(link tiktok) — download audio TikTok saja
menfess <no-wa> <pesan> — kirim pesan anonim
balas (pesan) — balas menfess aktif
brat / .brat / brat — bikin sticker teks
s / sticker — mode sticker untuk foto

*Cara pakai:*
Cukup kirim/paste link dan bot akan otomatis download! 🚀

_Contoh:_
https://vt.tiktok.com/ZS2abc123/
https://www.instagram.com/p/ABC123/
https://sck.io/p/abc-XYZ
https://open.spotify.com/track/3daK4oX3...
https://open.spotify.com/playlist/1uvZC8...
https://www.threads.net/@user/post/CwVDau3r3nQ/
https://x.com/username/status/1234567890/`

	h.sendText(evt.Info.Chat, help)
}
