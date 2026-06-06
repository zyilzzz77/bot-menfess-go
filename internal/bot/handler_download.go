package bot

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"bot-wa/internal/downloader"
	"bot-wa/internal/utils"

	waProto "go.mau.fi/whatsmeow/binary/proto"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	"google.golang.org/protobuf/proto"
)

// processDownload handles the full download and send flow.
func (h *Handler) processDownload(evt *events.Message, platform *utils.PlatformInfo) {
	chat := evt.Info.Chat
	chatKey := chat.String()
	sender := evt.Info.Sender.User

	fmt.Printf("📥 [%s] Download request from %s: %s\n", platform.Label, sender, platform.URL)
	h.reactToRequest(evt, "⚡")

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	result, err := h.downloader.DownloadGeneric(ctx, string(platform.Platform), platform.URL)
	if err != nil {
		fmt.Printf("❌ Download failed for %s: %v\n", platform.URL, err)
		h.reactToRequest(evt, "❌")
		h.sendText(chat, fmt.Sprintf("❌ Gagal download dari %s", platform.Label))
		return
	}
	defer h.downloader.CleanupAll(result)

	totalItems := len(result.Items)
	successCount := 0

	for i, item := range result.Items {
		fileSizeMB := float64(item.FileSize) / 1024 / 1024
		if fileSizeMB > 64 {
			h.sendText(chat, fmt.Sprintf("⚠️ File %d/%d terlalu besar (%.1fMB). Maks 64MB.", i+1, totalItems, fileSizeMB))
			continue
		}

		fileData, readErr := os.ReadFile(item.FilePath)
		if readErr != nil {
			fmt.Printf("❌ Failed to read file %s: %v\n", item.FilePath, readErr)
			continue
		}

		// Simple caption
		var caption string
		if totalItems > 1 {
			caption = fmt.Sprintf("%s · %d/%d", result.Title, i+1, totalItems)
		} else {
			caption = result.Title
		}

		var sendErr error
		switch item.MediaType {
		case downloader.MediaTypeVideo:
			sendErr = h.sendVideo(chat, fileData, caption)
		case downloader.MediaTypeImage:
			sendErr = h.sendImage(chat, fileData, caption)
		case downloader.MediaTypeAudio:
			sendErr = h.sendAudio(chat, fileData)
		default:
			sendErr = h.sendDocument(chat, fileData, item.FileName, caption)
		}

		if sendErr != nil {
			fmt.Printf("❌ Failed to send media %d/%d to %s: %v\n", i+1, totalItems, sender, sendErr)
			continue
		}

		successCount++
		fmt.Printf("✅ [%s] Sent %d/%d to %s (%.1fMB, type: %s)\n",
			platform.Label, i+1, totalItems, sender, fileSizeMB, mediaTypeStr(item.MediaType))

		if i < totalItems-1 {
			time.Sleep(sendDelay)
		}
	}

	if successCount > 0 {
		h.reactToRequest(evt, "✅")
		h.markURLDownloaded(chatKey, platform.URL, string(platform.Platform))

		if platform.Platform == utils.PlatformTikTok {
			go h.sendTikTokAudioButton(chat, platform.URL)
		}
	} else {
		h.reactToRequest(evt, "❌")
		h.sendText(chat, "❌ Gagal mengirim media")
	}
}

// processTikTokAudio handles the tomp3() command — downloads only audio from TikTok
func (h *Handler) processTikTokAudio(evt *events.Message, tiktokURL string) {
	chat := evt.Info.Chat
	chatKey := chat.String()
	sender := evt.Info.Sender.User

	fmt.Printf("🎵 [TikTok MP3] Audio request from %s: %s\n", sender, tiktokURL)
	h.reactToRequest(evt, "⚡")

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	result, err := h.downloader.DownloadTikTokAudio(ctx, tiktokURL)
	if err != nil {
		h.sendText(chat, fmt.Sprintf("❌ *Gagal download audio*\n\nError: %s", truncateError(err.Error())))
		return
	}
	defer h.downloader.CleanupAll(result)

	sentAudio := false
	for _, item := range result.Items {
		fileSizeMB := float64(item.FileSize) / 1024 / 1024
		fileData, readErr := os.ReadFile(item.FilePath)
		if readErr != nil {
			continue
		}

		if err := h.sendAudio(chat, fileData); err != nil {
			h.sendText(chat, fmt.Sprintf("❌ Gagal mengirim audio: %s", err.Error()))
			return
		}
		sentAudio = true

		fmt.Printf("✅ [TikTok MP3] Sent audio to %s (%.1fMB)\n", sender, fileSizeMB)
	}

	if sentAudio {
		h.reactToRequest(evt, "✅")
		h.markURLDownloaded(chatKey, tiktokURL, string(utils.PlatformTikTok))
	}
}

// sendTikTokAudioButton sends an interactive button message below the downloaded TikTok media.
// The button lets the user request the audio (MP3) version of the video.
func (h *Handler) sendTikTokAudioButton(chat types.JID, tiktokURL string) {
	// Generate a short unique button ID
	buttonID := fmt.Sprintf("ta_%d", time.Now().UnixNano())

	// Store mapping: buttonID -> tiktokURL (cleaned up in handler or after 10min)
	h.tiktokAudioMu.Lock()
	h.tiktokAudioButtons[buttonID] = tiktokURL
	h.tiktokAudioMu.Unlock()

	// Cleanup old entries
	go h.cleanupTikTokAudioButtons()

	msg := &waProto.Message{
		ButtonsMessage: &waProto.ButtonsMessage{
			Header:      &waProto.ButtonsMessage_Text{Text: "🎵 TikTok Audio"},
			ContentText: proto.String("Ingin download audio dari TikTok ini?"),
			FooterText:  proto.String("Bot WA Downloader"),
			HeaderType:  waProto.ButtonsMessage_TEXT.Enum(),
			Buttons: []*waProto.ButtonsMessage_Button{
				{
					ButtonID: proto.String(buttonID),
					ButtonText: &waProto.ButtonsMessage_Button_ButtonText{
						DisplayText: proto.String("🎵 Download Audio"),
					},
					Type: waProto.ButtonsMessage_Button_RESPONSE.Enum(),
				},
			},
		},
	}

	resp, err := h.client.SendMessage(context.Background(), chat, msg)
	if err != nil {
		fmt.Printf("⚠️ Failed to send TikTok audio button: %v\n", err)
	} else {
		fmt.Printf("🔘 [TikTok Button] Sent audio button (msgID: %s, buttonID: %s)\n", resp.ID, buttonID)
	}
}

// handleTikTokAudioButton processes a button tap on the "Download Audio" button.
func (h *Handler) handleTikTokAudioButton(evt *events.Message) bool {
	btnMsg := evt.Message.GetButtonsResponseMessage()
	if btnMsg == nil {
		return false
	}

	buttonID := btnMsg.GetSelectedButtonID()
	if buttonID == "" || !strings.HasPrefix(buttonID, "ta_") {
		return false
	}

	// Look up the TikTok URL
	h.tiktokAudioMu.RLock()
	tiktokURL, exists := h.tiktokAudioButtons[buttonID]
	h.tiktokAudioMu.RUnlock()

	if !exists {
		h.sendText(evt.Info.Chat, "⚠️ Tombol ini sudah expired. Silakan download ulang video TikTok-nya.")
		return true
	}

	// Remove the mapping to prevent double-tap
	h.tiktokAudioMu.Lock()
	delete(h.tiktokAudioButtons, buttonID)
	h.tiktokAudioMu.Unlock()

	// Process audio download in background
	go h.processTikTokAudioFromButton(evt, tiktokURL)
	return true
}

// processTikTokAudioFromButton handles audio download triggered by button tap.
func (h *Handler) processTikTokAudioFromButton(evt *events.Message, tiktokURL string) {
	chat := evt.Info.Chat
	chatKey := chat.String()
	sender := evt.Info.Sender.User

	fmt.Printf("🎵 [TikTok Button] Audio request from %s: %s\n", sender, tiktokURL)
	h.reactToRequest(evt, "🎵")

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	result, err := h.downloader.DownloadTikTokAudio(ctx, tiktokURL)
	if err != nil {
		h.sendText(chat, fmt.Sprintf("❌ *Gagal download audio*\n\nError: %s", truncateError(err.Error())))
		return
	}
	defer h.downloader.CleanupAll(result)

	sentAudio := false
	for _, item := range result.Items {
		fileData, readErr := os.ReadFile(item.FilePath)
		if readErr != nil {
			continue
		}

		if err := h.sendAudio(chat, fileData); err != nil {
			h.sendText(chat, fmt.Sprintf("❌ Gagal mengirim audio: %s", err.Error()))
			return
		}
		sentAudio = true
		fileSizeMB := float64(item.FileSize) / 1024 / 1024
		fmt.Printf("✅ [TikTok Button] Sent audio to %s (%.1fMB)\n", sender, fileSizeMB)
	}

	if sentAudio {
		h.reactToRequest(evt, "✅")
		h.markURLDownloaded(chatKey, tiktokURL, string(utils.PlatformTikTok))
	}
}

// cleanupTikTokAudioButtons removes button mappings older than 10 minutes.
func (h *Handler) cleanupTikTokAudioButtons() {
	h.tiktokAudioMu.Lock()
	defer h.tiktokAudioMu.Unlock()

	// We clean based on timestamp encoded in the buttonID: "ta_<unix_nano>"
	cutoff := time.Now().Add(-10 * time.Minute)
	for id := range h.tiktokAudioButtons {
		// Parse nano timestamp from buttonID
		if len(id) > 3 && id[:3] == "ta_" {
			var ns int64
			if _, err := fmt.Sscanf(id, "ta_%d", &ns); err == nil {
				t := time.Unix(0, ns)
				if t.Before(cutoff) {
					delete(h.tiktokAudioButtons, id)
				}
			}
		}
	}
}
