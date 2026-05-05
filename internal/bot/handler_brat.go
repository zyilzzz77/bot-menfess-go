package bot

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"go.mau.fi/whatsmeow/types/events"
)

var bratCommandPattern = regexp.MustCompile(`(?is)^(?:/|\.)?brat(?:\s+(.+))?$`)

// parseBratCommand extracts text from /brat, .brat, or brat commands.
func parseBratCommand(msg string) (string, bool) {
	matches := bratCommandPattern.FindStringSubmatch(strings.TrimSpace(msg))
	if len(matches) != 2 {
		return "", false
	}

	return strings.TrimSpace(matches[1]), true
}

// handleBratCommand generates a text sticker from the brat API.
func (h *Handler) handleBratCommand(evt *events.Message, text string) {
	chat := evt.Info.Chat
	sender := evt.Info.Sender.User
	text = strings.TrimSpace(text)

	if text == "" {
		h.sendText(chat, "❌ Teks untuk brat masih kosong.\n\nContoh: /brat ayo scroll fesnuk 😜")
		return
	}

	fmt.Printf("🧵 [Brat] Request from %s: %s\n", sender, text)
	h.reactToRequest(evt, "⚡")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	stickerBytes, err := h.downloader.GenerateBratImage(ctx, text)
	if err != nil {
		h.sendText(chat, fmt.Sprintf("❌ Gagal membuat sticker teks: %s", truncateError(err.Error())))
		return
	}

	if err := h.sendStickerFromImageData(chat, sender, stickerBytes); err != nil {
		h.sendText(chat, fmt.Sprintf("❌ Gagal mengirim sticker brat: %s", truncateError(err.Error())))
		return
	}

	h.reactToRequest(evt, "✅")
	fmt.Printf("✅ [Brat] Sent sticker to %s\n", sender)
}
