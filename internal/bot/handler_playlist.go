package bot

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"bot-wa/internal/downloader"
	"bot-wa/internal/utils"

	waProto "go.mau.fi/whatsmeow/binary/proto"
	"go.mau.fi/whatsmeow/types/events"
	"google.golang.org/protobuf/proto"
)

// handleSpotifyPlaylist fetches playlist info and shows track list
func (h *Handler) handleSpotifyPlaylist(evt *events.Message, platform *utils.PlatformInfo) {
	chat := evt.Info.Chat
	sender := evt.Info.Sender.User

	fmt.Printf("📥 [Spotify Playlist] Request from %s: %s\n", sender, platform.URL)
	h.reactToRequest(evt, "⚡")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	playlist, err := h.downloader.FetchSpotifyPlaylist(ctx, platform.URL)
	if err != nil {
		fmt.Printf("❌ Playlist fetch failed: %v\n", err)
		h.sendText(chat, fmt.Sprintf(
			"❌ *Gagal mengambil playlist*\n\nError: %s",
			truncateError(err.Error()),
		))
		return
	}

	// Build track list message
	totalTracks := len(playlist.Tracks)
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("🎧 *%s*\n", playlist.Title))
	sb.WriteString(fmt.Sprintf("📀 %d lagu ditemukan\n\n", totalTracks))

	// Show max 50 tracks in list
	maxShow := totalTracks
	if maxShow > 50 {
		maxShow = 50
	}

	for i := 0; i < maxShow; i++ {
		t := playlist.Tracks[i]
		sb.WriteString(fmt.Sprintf("*%d.* %s _%s_\n", i+1, t.Title, t.Duration))
	}

	if totalTracks > 50 {
		sb.WriteString(fmt.Sprintf("\n_...dan %d lagu lainnya_\n", totalTracks-50))
	}

	sb.WriteString("\n━━━━━━━━━━━━━━━━━━━\n")
	sb.WriteString("📥 *Pilih yang mau di-download:*\n\n")
	sb.WriteString("• Ketik *all* — download semua lagu\n")
	sb.WriteString("• Ketik nomor — misal: *1,3,5*\n")
	sb.WriteString("• Ketik range — misal: *1-10*\n")
	sb.WriteString("• Ketik *cancel* — batalkan\n")
	sb.WriteString("\n⏳ _Berlaku 10 menit_")

	if _, err := h.sendTextMessage(chat, sb.String()); err != nil {
		return
	}

	// Store pending playlist
	h.playlistMu.Lock()
	h.pendingPlaylists[chat.String()] = &pendingPlaylist{
		Playlist:  playlist,
		URL:       platform.URL,
		CreatedAt: time.Now(),
	}
	h.playlistMu.Unlock()

	h.reactToRequest(evt, "✅")
	fmt.Printf("📋 [Spotify Playlist] Stored %d tracks for %s\n", totalTracks, sender)
}

// processPlaylistDownload downloads selected tracks from a playlist
// If indices is nil, downloads all tracks
func (h *Handler) processPlaylistDownload(evt *events.Message, pending *pendingPlaylist, indices []int) {
	chat := evt.Info.Chat
	sender := evt.Info.Sender.User
	tracks := pending.Playlist.Tracks

	// Determine which tracks to download
	var selectedTracks []downloader.SpotifyPlaylistTrack
	if indices == nil {
		selectedTracks = tracks
	} else {
		for _, idx := range indices {
			if idx >= 0 && idx < len(tracks) {
				selectedTracks = append(selectedTracks, tracks[idx])
			}
		}
	}

	totalTracks := len(selectedTracks)
	if totalTracks == 0 {
		h.sendText(chat, "❌ Tidak ada lagu yang dipilih.")
		return
	}

	h.reactToRequest(evt, "⚡")

	fmt.Printf("📥 [Spotify Playlist] Downloading %d tracks for %s\n", totalTracks, sender)

	// Send a minimal placeholder that will be edited into the final result.
	statusResp, err := h.client.SendMessage(context.Background(), chat, &waProto.Message{
		Conversation: proto.String("⏳"),
	})
	var statusMsgID string
	if err == nil {
		statusMsgID = statusResp.ID
	}

	successCount := 0
	for i, track := range selectedTracks {
		// Download the track using single track API
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		result, dlErr := h.downloader.DownloadSpotify(ctx, track.URL)
		cancel()

		if dlErr != nil {
			fmt.Printf("❌ Failed to download track %d: %s - %v\n", i+1, track.Title, dlErr)
			continue
		}

		// Send each item from the result
		for _, item := range result.Items {
			fileSizeMB := float64(item.FileSize) / 1024 / 1024
			if fileSizeMB > 64 {
				continue
			}

			fileData, readErr := os.ReadFile(item.FilePath)
			if readErr != nil {
				continue
			}

			caption := fmt.Sprintf("🎧 *%s*\n🎤 %s\n📀 %s\n⏱️ %s\n\n📄 %d/%d · 📁 %.1fMB",
				track.Title, track.Artists, track.Album, track.Duration,
				i+1, totalTracks, fileSizeMB)

			var sendErr error
			switch item.MediaType {
			case downloader.MediaTypeImage:
				sendErr = h.sendImage(chat, fileData, caption)
			case downloader.MediaTypeAudio:
				sendErr = h.sendAudio(chat, fileData)
			default:
				sendErr = h.sendDocument(chat, fileData, item.FileName, caption)
			}

			if sendErr != nil {
				fmt.Printf("❌ Failed to send %s: %v\n", item.FileName, sendErr)
			}

			// Small delay between cover and audio of same track
			time.Sleep(1 * time.Second)
		}

		h.downloader.CleanupAll(result)
		successCount++

		fmt.Printf("✅ [Spotify Playlist] Sent track %d/%d: %s to %s\n",
			i+1, totalTracks, track.Title, sender)

		// Delay between tracks (anti-ban)
		if i < totalTracks-1 {
			time.Sleep(sendDelay)
		}
	}

	// Final status
	if statusMsgID != "" {
		h.editMessage(chat, statusMsgID, fmt.Sprintf(
			"✅ *Playlist selesai!*\n\n🎧 %s\n📀 %d/%d lagu berhasil dikirim",
			pending.Playlist.Title, successCount, totalTracks,
		))
	}

	if successCount > 0 {
		h.reactToRequest(evt, "✅")
	}

	fmt.Printf("✅ [Spotify Playlist] Completed: %d/%d tracks sent to %s\n",
		successCount, totalTracks, sender)
}

// parseTrackSelection parses user input for track selection
// Supports: "1,3,5", "1-5", "1,3-5,8", etc.
// Returns 0-indexed indices
func parseTrackSelection(input string, maxTracks int) []int {
	input = strings.ReplaceAll(input, " ", "")
	parts := strings.Split(input, ",")

	seen := make(map[int]bool)
	var indices []int

	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		// Check for range (e.g., "1-5")
		if strings.Contains(part, "-") {
			rangeParts := strings.SplitN(part, "-", 2)
			if len(rangeParts) == 2 {
				start, err1 := strconv.Atoi(rangeParts[0])
				end, err2 := strconv.Atoi(rangeParts[1])
				if err1 == nil && err2 == nil && start >= 1 && end >= start {
					if end > maxTracks {
						end = maxTracks
					}
					for i := start; i <= end; i++ {
						idx := i - 1 // convert to 0-indexed
						if !seen[idx] && idx >= 0 && idx < maxTracks {
							seen[idx] = true
							indices = append(indices, idx)
						}
					}
				}
			}
			continue
		}

		// Single number
		num, err := strconv.Atoi(part)
		if err == nil && num >= 1 && num <= maxTracks {
			idx := num - 1 // convert to 0-indexed
			if !seen[idx] {
				seen[idx] = true
				indices = append(indices, idx)
			}
		}
	}

	return indices
}

// minInt returns the smaller of two ints
func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
