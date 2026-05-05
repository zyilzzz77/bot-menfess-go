package bot

import (
	"context"
	"fmt"
	"os"
	"time"

	"bot-wa/internal/downloader"
	"bot-wa/internal/utils"

	"go.mau.fi/whatsmeow/types/events"
)

// processDownload handles the full download and send flow
func (h *Handler) processDownload(evt *events.Message, platform *utils.PlatformInfo) {
	chat := evt.Info.Chat
	chatKey := chat.String()
	sender := evt.Info.Sender.User

	fmt.Printf("📥 [%s] Download request from %s: %s\n", platform.Label, sender, platform.URL)
	h.reactToRequest(evt, "⚡")

	// Start progress tracker (sends initial message + live updates)
	progress := h.newProgressTracker(chat, platform.Label)

	// Download the media via API
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// Update progress stage
	if progress != nil {
		progress.setStage("📥 Mengunduh media...")
	}

	result, err := h.downloader.DownloadGeneric(ctx, string(platform.Platform), platform.URL)
	if err != nil {
		fmt.Printf("❌ Download failed for %s: %v\n", platform.URL, err)
		if progress != nil {
			progress.finish(fmt.Sprintf(
				"❌ *Download gagal*\n\nMaaf, tidak bisa download dari %s.\nError: %s\n\n💡 Tips:\n• Pastikan link valid dan tidak private\n• Konten mungkin dilindungi atau region-locked\n• Coba lagi nanti",
				platform.Label,
				truncateError(err.Error()),
			))
		}
		return
	}
	defer h.downloader.CleanupAll(result)

	totalItems := len(result.Items)

	// Update progress: uploading to WhatsApp
	if progress != nil {
		if totalItems > 1 {
			progress.setStage(fmt.Sprintf("📤 Mengirim %d media ke WhatsApp...", totalItems))
		} else {
			progress.setStage("📤 Mengirim media ke WhatsApp...")
		}
	}

	// Send each media item
	successCount := 0

	for i, item := range result.Items {
		fileSizeMB := float64(item.FileSize) / 1024 / 1024
		if fileSizeMB > 64 {
			h.sendText(chat, fmt.Sprintf(
				"⚠️ File %d/%d terlalu besar (%.1fMB). Maks 64MB.",
				i+1, totalItems, fileSizeMB,
			))
			continue
		}

		// Read the file
		fileData, readErr := os.ReadFile(item.FilePath)
		if readErr != nil {
			fmt.Printf("❌ Failed to read file %s: %v\n", item.FilePath, readErr)
			continue
		}

		// Build caption
		var caption string
		if totalItems > 1 {
			caption = fmt.Sprintf("✅ *%s*\n\n📄 %d/%d · 📁 %.1fMB\n🤖 WA Downloader Bot",
				result.Title, i+1, totalItems, fileSizeMB)
		} else {
			caption = fmt.Sprintf("✅ *%s*\n\n📁 %.1fMB\n🤖 WA Downloader Bot",
				result.Title, fileSizeMB)
		}

		// Send the media in the correct format
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
			fmt.Printf("⏳ Waiting %v before next send (anti-ban)...\n", sendDelay)
			time.Sleep(sendDelay)
		}
	}

	// Final progress update (the download phase message)
	if progress != nil {
		if successCount > 0 {
			progress.finish(fmt.Sprintf("✅ *Selesai!*\n\n📦 %d/%d media berhasil dikirim dari %s",
				successCount, totalItems, platform.Label))
		} else {
			progress.finish("❌ *Gagal mengirim semua media*")
		}
	}

	if successCount > 0 {
		h.reactToRequest(evt, "✅")
		h.markURLDownloaded(chatKey, platform.URL, string(platform.Platform))
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
