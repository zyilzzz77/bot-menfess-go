package bot

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	waProto "go.mau.fi/whatsmeow/binary/proto"
	"go.mau.fi/whatsmeow/types/events"
)

// aiRateLimitInterval is the minimum time between AI queries per chat.
const aiRateLimitInterval = 10 * time.Second

// aiCommandPattern matches /ai or .ai commands with optional text.
// Examples: /ai, .ai, /ai halo, .ai apa itu AI?
var aiCommandPattern = regexp.MustCompile(`(?is)^(?:/|\.)ai(?:\s+(.+))?$`)

// aisCommandPattern matches /ais or .ais (AI with web search shortcut).
var aisCommandPattern = regexp.MustCompile(`(?is)^(?:/|\.)ais(?:\s+(.+))?$`)

// parseAICommand extracts text from /ai or .ai commands.
func parseAICommand(msg string) (string, bool) {
	matches := aiCommandPattern.FindStringSubmatch(strings.TrimSpace(msg))
	if len(matches) != 2 {
		return "", false
	}
	return strings.TrimSpace(matches[1]), true
}

// parseAISCommand extracts text from /ais or .ais commands (AI + search).
func parseAISCommand(msg string) (string, bool) {
	matches := aisCommandPattern.FindStringSubmatch(strings.TrimSpace(msg))
	if len(matches) != 2 {
		return "", false
	}
	return strings.TrimSpace(matches[1]), true
}

// handleAICommand processes a text AI query.
func (h *Handler) handleAICommand(evt *events.Message, text string) {
	chat := evt.Info.Chat
	sender := evt.Info.Sender.User
	text = strings.TrimSpace(text)

	if text == "" {
		// No text — give hint about usage
		h.sendText(chat, "🤖 *AI Assistant*\n\nSilakan ketik pertanyaan setelah /ai\n\nContoh:\n/ai apa itu black hole?\n.ai jelaskan cara kerja internet\n\n_Untuk search internet: /ais <query> atau .ais <query>_")
		return
	}

	fmt.Printf("🤖 [AI] Query from %s: %s\n", sender, truncateError(text))
	h.reactToRequest(evt, "🤖")

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	response, err := h.ai.Chat(ctx, text)
	if err != nil {
		h.sendText(chat, fmt.Sprintf("❌ AI error: %s", truncateError(err.Error())))
		return
	}

	if response == "" {
		h.sendText(chat, "❌ AI tidak memberikan respons.")
		return
	}

	h.sendText(chat, response)
	h.reactToRequest(evt, "✅")
	fmt.Printf("✅ [AI] Responded to %s (%d chars)\n", sender, len(response))
}

// handleAISearchCommand processes a web search + AI summary query.
func (h *Handler) handleAISearchCommand(evt *events.Message, query string) {
	chat := evt.Info.Chat
	sender := evt.Info.Sender.User
	query = strings.TrimSpace(query)

	if query == "" {
		h.sendText(chat, "🤖 *AI Web Search*\n\nSilakan ketik kata kunci pencarian setelah /ais\n\nContoh:\n/ais berita terbaru AI 2025\n.ais harga bitcoin hari ini")
		return
	}

	fmt.Printf("🔍 [AI-Search] Query from %s: %s\n", sender, truncateError(query))

	// Send a status message that we'll edit
	statusID, err := h.sendTextMessage(chat, "🔍 *Mencari di web...* ⏳")
	if err != nil {
		statusID = ""
	}

	h.reactToRequest(evt, "🔍")

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	answer, results, err := h.ai.ChatWithWebSearch(ctx, query)

	// Edit status message
	if statusID != "" {
		if err != nil {
			h.editMessage(chat, statusID, "❌ Pencarian web gagal. Menjawab tanpa search...")
		} else {
			h.editMessage(chat, statusID, fmt.Sprintf("🔍 Ditemukan %d hasil. Merangkum... ⏳", len(results)))
		}
	}

	if err != nil {
		// If chat succeeded but search failed, we still have the answer
		if answer != "" {
			h.sendText(chat, answer)
			h.reactToRequest(evt, "✅")
			return
		}
		h.sendText(chat, fmt.Sprintf("❌ Search error: %s", truncateError(err.Error())))
		return
	}

	if answer == "" {
		h.sendText(chat, "❌ Tidak dapat menemukan informasi.")
		return
	}

	// Add source links at the end
	if len(results) > 0 {
		var sources strings.Builder
		sources.WriteString("\n\n───\n🌐 *Sumber:*\n")
		for i, r := range results {
			if i >= 3 {
				break
			}
			sources.WriteString(fmt.Sprintf("• %s\n", r.URL))
		}
		// Only append sources if the answer isn't already too long
		if len(answer)+sources.Len() < 4000 {
			answer += sources.String()
		}
	}

	h.sendText(chat, answer)
	h.reactToRequest(evt, "✅")
	fmt.Printf("✅ [AI-Search] Responded to %s (%d chars, %d sources)\n", sender, len(answer), len(results))
}

// handleAIImageCommand processes an image with an AI query.
func (h *Handler) handleAIImageCommand(evt *events.Message, imgMsg *waProto.ImageMessage, text string) {
	chat := evt.Info.Chat
	sender := evt.Info.Sender.User
	text = strings.TrimSpace(text)

	fmt.Printf("🖼️ [AI-Image] Query from %s (caption: %s)\n", sender, truncateError(text))
	h.reactToRequest(evt, "🤖")

	// Download image from WhatsApp
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	imageData, err := h.client.Download(ctx, imgMsg)
	if err != nil {
		h.sendText(chat, fmt.Sprintf("❌ Gagal download gambar: %s", truncateError(err.Error())))
		return
	}

	mimeType := imgMsg.GetMimetype()
	if mimeType == "" {
		mimeType = "image/jpeg"
	}

	response, err := h.ai.ChatWithImage(ctx, text, imageData, mimeType)
	if err != nil {
		h.sendText(chat, fmt.Sprintf("❌ AI error: %s", truncateError(err.Error())))
		return
	}

	if response == "" {
		h.sendText(chat, "❌ AI tidak memberikan respons untuk gambar ini.")
		return
	}

	h.sendText(chat, response)
	h.reactToRequest(evt, "✅")
	fmt.Printf("✅ [AI-Image] Responded to %s (%d chars)\n", sender, len(response))
}

// handleAIAudioCommand processes an audio message by transcribing then querying AI.
// Note: Audio transcription requires a separate speech-to-text service.
// Currently, this falls back to text-based chat explaining the limitation.
func (h *Handler) handleAIAudioCommand(evt *events.Message, audioMsg *waProto.AudioMessage) {
	chat := evt.Info.Chat
	sender := evt.Info.Sender.User

	fmt.Printf("🎤 [AI-Audio] Voice note from %s\n", sender)
	h.reactToRequest(evt, "🎤")

	// Audio transcription requires Whisper API or similar service.
	// For now, inform the user that voice-to-text is not yet configured.
	h.sendText(chat, "🎤 *Voice Note Diterima*\n\nFitur transkripsi suara belum tersedia karena membutuhkan OpenAI API key untuk Whisper.\n\n_Saran: Kirim pertanyaanmu dalam bentuk teks dengan /ai <pertanyaan> atau kirim gambar dengan caption /ai_")

	fmt.Printf("⚠️ [AI-Audio] Voice note from %s — transcription not configured\n", sender)
}

// detectMimeTypeFromBytes reads magic bytes to detect image format.
func detectMimeTypeFromBytes(data []byte) string {
	if len(data) < 4 {
		return "image/jpeg"
	}
	// JPEG: FF D8 FF
	if data[0] == 0xFF && data[1] == 0xD8 && data[2] == 0xFF {
		return "image/jpeg"
	}
	// PNG: 89 50 4E 47
	if data[0] == 0x89 && data[1] == 0x50 && data[2] == 0x4E && data[3] == 0x47 {
		return "image/png"
	}
	// WebP: 52 49 46 46 (RIFF)
	if data[0] == 0x52 && data[1] == 0x49 && data[2] == 0x46 && data[3] == 0x46 {
		return "image/webp"
	}
	return "image/jpeg"
}

// checkAIRateLimit returns true if the AI query is allowed, false if rate limited.
func (h *Handler) checkAIRateLimit(chatKey string) bool {
	h.aiRateLimitMu.Lock()
	defer h.aiRateLimitMu.Unlock()

	lastTime, exists := h.aiRateLimit[chatKey]
	if exists && time.Since(lastTime) < aiRateLimitInterval {
		return false
	}

	h.aiRateLimit[chatKey] = time.Now()
	return true
}
