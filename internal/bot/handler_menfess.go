package bot

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	waProto "go.mau.fi/whatsmeow/binary/proto"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
)

const menfessThreadExpiry = 24 * time.Hour

var (
	menfessParenPattern = regexp.MustCompile(`(?is)^menfess\s*\(\s*([^)]+?)\s*\)\s*\(\s*(.+)\s*\)\s*$`)
	menfessPlainPattern = regexp.MustCompile(`(?is)^menfess\s+(\S+)\s+(.+)$`)
	menfessReplyPattern = regexp.MustCompile(`(?is)^balas\s*(?:\(\s*(.+?)\s*\)|(.+))\s*$`)
)

type menfessThread struct {
	Sender       types.JID
	Target       types.JID
	LastActivity time.Time
}

// parseMenfessCommand extracts the destination number and body from a menfess command.
func parseMenfessCommand(msg string) (types.JID, string, error) {
	trimmed := strings.TrimSpace(msg)
	if trimmed == "" {
		return types.JID{}, "", fmt.Errorf("format menfess tidak valid")
	}

	var targetRaw, body string
	if matches := menfessParenPattern.FindStringSubmatch(trimmed); len(matches) == 3 {
		targetRaw = matches[1]
		body = matches[2]
	} else if matches := menfessPlainPattern.FindStringSubmatch(trimmed); len(matches) == 3 {
		targetRaw = matches[1]
		body = matches[2]
	} else {
		return types.JID{}, "", fmt.Errorf("format menfess: menfess <no-wa> <pesan>")
	}

	targetPhone := normalizeMenfessPhone(targetRaw)
	if targetPhone == "" || len(targetPhone) < 8 {
		return types.JID{}, "", fmt.Errorf("nomor tujuan tidak valid")
	}

	body = strings.TrimSpace(body)
	if body == "" {
		return types.JID{}, "", fmt.Errorf("pesan menfess masih kosong")
	}

	return types.NewJID(targetPhone, types.DefaultUserServer), body, nil
}

// handleMenfessCommand sends an anonymous message to the target number.
func (h *Handler) handleMenfessCommand(evt *events.Message, target types.JID, body string) {
	chat := evt.Info.Chat
	sender := evt.Info.Sender
	text := strings.TrimSpace(body)

	if text == "" {
		h.sendText(chat, "❌ Pesan menfess masih kosong.")
		return
	}
	if target.IsEmpty() {
		h.sendText(chat, "❌ Nomor tujuan menfess tidak valid.")
		return
	}
	if sameMenfessParticipant(target, sender) {
		h.sendText(chat, "❌ Menfess ke nomor sendiri tidak bisa.")
		return
	}

	h.reactToRequest(evt, "⚡")

	thread := &menfessThread{
		Sender:       sender,
		Target:       target,
		LastActivity: time.Now(),
	}

	payload := buildMenfessIncomingPayload(text)
	msgID, err := h.sendTextMessage(target, payload)
	if err != nil {
		h.sendText(chat, fmt.Sprintf("❌ Gagal mengirim menfess: %s", truncateError(err.Error())))
		return
	}

	h.registerMenfessMessage(target, msgID, thread)
	h.reactToRequest(evt, "✅")
	fmt.Printf("📨 [Menfess] Sent from %s to %s\n", sender.String(), target.String())
}

// handleMenfessReply forwards replies between the sender and target anonymously.
func (h *Handler) handleMenfessReply(evt *events.Message) bool {
	quotedID := extractQuotedMessageID(evt.Message)
	if quotedID != "" {
		thread, ok := h.lookupMenfessThread(quotedID)
		if !ok {
			return false
		}

		body := extractReplyBody(evt.Message)
		if strings.TrimSpace(body) == "" {
			h.sendText(evt.Info.Chat, "❌ Menfess hanya mendukung teks atau caption media.")
			return true
		}

		currentSender := evt.Info.Sender
		var destination types.JID
		if sameMenfessParticipant(currentSender, thread.Target) {
			destination = thread.Sender
		} else if sameMenfessParticipant(currentSender, thread.Sender) {
			destination = thread.Target
		} else {
			return false
		}

		return h.forwardMenfessMessage(evt, thread, destination, strings.TrimSpace(body))
	}

	body, matched, err := parseMenfessReplyCommand(extractReplyBody(evt.Message))
	if !matched {
		return false
	}
	if err != nil {
		h.sendText(evt.Info.Chat, fmt.Sprintf("❌ %s\n\nFormat: balas (pesan yang ingin disampaikan)", err.Error()))
		return true
	}

	thread, ok := h.lookupMenfessThreadByChat(evt.Info.Chat)
	if !ok {
		h.sendText(evt.Info.Chat, "❌ Belum ada menfess aktif di chat ini. Kirim menfess dulu atau reply ke pesan menfess yang ada.")
		return true
	}

	currentSender := evt.Info.Sender
	var destination types.JID
	if sameMenfessParticipant(currentSender, thread.Target) {
		destination = thread.Sender
	} else if sameMenfessParticipant(currentSender, thread.Sender) {
		destination = thread.Target
	} else {
		h.sendText(evt.Info.Chat, "❌ Kamu bukan bagian dari menfess aktif ini.")
		return true
	}

	return h.forwardMenfessMessage(evt, thread, destination, body)
}

func (h *Handler) forwardMenfessMessage(evt *events.Message, thread *menfessThread, destination types.JID, body string) bool {
	h.reactToRequest(evt, "⚡")

	payload := buildMenfessReplyPayload(evt.Info.Sender, body)
	msgID, err := h.sendTextMessage(destination, payload)
	if err != nil {
		h.sendText(evt.Info.Chat, fmt.Sprintf("❌ Gagal meneruskan menfess: %s", truncateError(err.Error())))
		return true
	}

	h.registerMenfessMessage(destination, msgID, thread)
	h.reactToRequest(evt, "✅")
	return true
}

// registerMenfessMessage stores a bot-sent menfess relay message for reply routing.
func (h *Handler) registerMenfessMessage(chat types.JID, messageID string, thread *menfessThread) {
	if messageID == "" || thread == nil || chat.IsEmpty() {
		return
	}

	h.menfessMu.Lock()
	defer h.menfessMu.Unlock()

	now := time.Now()
	thread.LastActivity = now
	h.cleanupMenfessLocked(now)
	h.menfessThreads[messageID] = thread
	h.menfessChatMessages[chat.ToNonAD().String()] = messageID
}

// lookupMenfessThread returns the thread for a quoted bot-sent menfess message.
func (h *Handler) lookupMenfessThread(messageID string) (*menfessThread, bool) {
	if messageID == "" {
		return nil, false
	}

	h.menfessMu.Lock()
	defer h.menfessMu.Unlock()

	h.cleanupMenfessLocked(time.Now())
	thread, ok := h.menfessThreads[messageID]
	return thread, ok
}

// lookupMenfessThreadByChat returns the most recent active menfess thread for a chat.
func (h *Handler) lookupMenfessThreadByChat(chat types.JID) (*menfessThread, bool) {
	if chat.IsEmpty() {
		return nil, false
	}

	h.menfessMu.Lock()
	defer h.menfessMu.Unlock()

	h.cleanupMenfessLocked(time.Now())
	messageID, ok := h.menfessChatMessages[chat.ToNonAD().String()]
	if !ok {
		return nil, false
	}

	thread, ok := h.menfessThreads[messageID]
	if !ok {
		delete(h.menfessChatMessages, chat.ToNonAD().String())
		return nil, false
	}

	return thread, true
}

// cleanupMenfessLocked removes expired menfess threads. Caller must hold menfessMu.
func (h *Handler) cleanupMenfessLocked(now time.Time) {
	expired := make(map[string]struct{})
	for messageID, thread := range h.menfessThreads {
		if now.Sub(thread.LastActivity) > menfessThreadExpiry {
			delete(h.menfessThreads, messageID)
			expired[messageID] = struct{}{}
		}
	}

	if len(expired) == 0 {
		return
	}

	for chatKey, messageID := range h.menfessChatMessages {
		if _, ok := expired[messageID]; ok {
			delete(h.menfessChatMessages, chatKey)
		}
	}
}

func normalizeMenfessPhone(raw string) string {
	var builder strings.Builder
	for _, r := range raw {
		if r >= '0' && r <= '9' {
			builder.WriteRune(r)
		}
	}

	digits := builder.String()
	switch {
	case digits == "":
		return ""
	case strings.HasPrefix(digits, "62"):
		return digits
	case strings.HasPrefix(digits, "0"):
		return "62" + digits[1:]
	case strings.HasPrefix(digits, "8"):
		return "62" + digits
	default:
		return digits
	}
}

func buildMenfessIncomingPayload(body string) string {
	return fmt.Sprintf(
		"📩 *Menfess masuk*\n\n📝 Pesan:\n%s\n\n💬 Untuk balas, ketik: balas (pesan)",
		strings.TrimSpace(body),
	)
}

func buildMenfessReplyPayload(source types.JID, body string) string {
	return fmt.Sprintf(
		"↩️ *Balasan menfess*\n\n👤 Dari: %s\n\n📝 Pesan:\n%s\n\n💬 Untuk balas lagi, ketik: balas (pesan)",
		formatMenfessPhoneNumber(source),
		strings.TrimSpace(body),
	)
}

func formatMenfessPhoneNumber(jid types.JID) string {
	jid = jid.ToNonAD()
	if jid.User != "" {
		return jid.User
	}
	return "nomor tidak diketahui"
}

func sameMenfessParticipant(a, b types.JID) bool {
	return a.ToNonAD().String() == b.ToNonAD().String()
}

func parseMenfessReplyCommand(msg string) (string, bool, error) {
	trimmed := strings.TrimSpace(msg)
	if trimmed == "" {
		return "", false, nil
	}

	if !strings.HasPrefix(strings.ToLower(trimmed), "balas") {
		return "", false, nil
	}

	matches := menfessReplyPattern.FindStringSubmatch(trimmed)
	if len(matches) != 3 {
		return "", true, fmt.Errorf("format balas: balas (pesan)")
	}

	body := strings.TrimSpace(matches[1])
	if body == "" {
		body = strings.TrimSpace(matches[2])
	}
	if body == "" {
		return "", true, fmt.Errorf("pesan balas masih kosong")
	}

	return body, true, nil
}

func extractQuotedMessageID(msg *waProto.Message) string {
	if msg == nil {
		return ""
	}

	switch {
	case msg.GetExtendedTextMessage() != nil && msg.GetExtendedTextMessage().GetContextInfo() != nil:
		return msg.GetExtendedTextMessage().GetContextInfo().GetStanzaID()
	case msg.GetImageMessage() != nil && msg.GetImageMessage().GetContextInfo() != nil:
		return msg.GetImageMessage().GetContextInfo().GetStanzaID()
	case msg.GetVideoMessage() != nil && msg.GetVideoMessage().GetContextInfo() != nil:
		return msg.GetVideoMessage().GetContextInfo().GetStanzaID()
	case msg.GetDocumentMessage() != nil && msg.GetDocumentMessage().GetContextInfo() != nil:
		return msg.GetDocumentMessage().GetContextInfo().GetStanzaID()
	case msg.GetStickerMessage() != nil && msg.GetStickerMessage().GetContextInfo() != nil:
		return msg.GetStickerMessage().GetContextInfo().GetStanzaID()
	case msg.GetAudioMessage() != nil && msg.GetAudioMessage().GetContextInfo() != nil:
		return msg.GetAudioMessage().GetContextInfo().GetStanzaID()
	case msg.GetButtonsResponseMessage() != nil && msg.GetButtonsResponseMessage().GetContextInfo() != nil:
		return msg.GetButtonsResponseMessage().GetContextInfo().GetStanzaID()
	case msg.GetListResponseMessage() != nil && msg.GetListResponseMessage().GetContextInfo() != nil:
		return msg.GetListResponseMessage().GetContextInfo().GetStanzaID()
	case msg.GetInteractiveResponseMessage() != nil && msg.GetInteractiveResponseMessage().GetContextInfo() != nil:
		return msg.GetInteractiveResponseMessage().GetContextInfo().GetStanzaID()
	case msg.GetTemplateButtonReplyMessage() != nil && msg.GetTemplateButtonReplyMessage().GetContextInfo() != nil:
		return msg.GetTemplateButtonReplyMessage().GetContextInfo().GetStanzaID()
	default:
		return ""
	}
}

func extractReplyBody(msg *waProto.Message) string {
	if msg == nil {
		return ""
	}

	switch {
	case msg.GetConversation() != "":
		return msg.GetConversation()
	case msg.GetExtendedTextMessage() != nil:
		return msg.GetExtendedTextMessage().GetText()
	case msg.GetImageMessage() != nil:
		return msg.GetImageMessage().GetCaption()
	case msg.GetVideoMessage() != nil:
		return msg.GetVideoMessage().GetCaption()
	case msg.GetDocumentMessage() != nil:
		return msg.GetDocumentMessage().GetCaption()
	default:
		return ""
	}
}
