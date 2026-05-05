package bot

import (
	"context"
	"fmt"
	"time"

	"bot-wa/internal/downloader"

	"go.mau.fi/whatsmeow"
	waProto "go.mau.fi/whatsmeow/binary/proto"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	"google.golang.org/protobuf/proto"
)

// sendText sends a text message
func (h *Handler) sendText(chat types.JID, text string) {
	_, _ = h.sendTextMessage(chat, text)
}

// sendTextMessage sends a text message and returns the sent message ID.
func (h *Handler) sendTextMessage(chat types.JID, text string) (string, error) {
	resp, err := h.client.SendMessage(context.Background(), chat, &waProto.Message{
		Conversation: proto.String(text),
	})
	if err != nil {
		fmt.Printf("❌ Failed to send text: %v\n", err)
		return "", err
	}
	return resp.ID, nil
}

// editMessage edits a previously sent message by ID
func (h *Handler) editMessage(chat types.JID, messageID string, newText string) {
	chatJID := chat.String()

	editMsg := &waProto.Message{
		ProtocolMessage: &waProto.ProtocolMessage{
			Key: &waProto.MessageKey{
				RemoteJID: proto.String(chatJID),
				FromMe:    proto.Bool(true),
				ID:        proto.String(messageID),
			},
			Type: waProto.ProtocolMessage_MESSAGE_EDIT.Enum(),
			EditedMessage: &waProto.Message{
				Conversation: proto.String(newText),
			},
		},
	}

	_, err := h.client.SendMessage(context.Background(), chat, editMsg)
	if err != nil {
		fmt.Printf("⚠️ Failed to edit message: %v\n", err)
	}
}

// reactToRequest adds a reaction to the user's original message.
func (h *Handler) reactToRequest(evt *events.Message, emoji string) {
	if evt == nil || emoji == "" || h.client == nil {
		return
	}

	msg := h.client.BuildReaction(evt.Info.Chat, evt.Info.Sender, evt.Info.ID, emoji)
	_, err := h.client.SendMessage(context.Background(), evt.Info.Chat, msg)
	if err != nil {
		fmt.Printf("⚠️ Failed to react to message: %v\n", err)
	}
}

// isDuplicateURL checks if a URL was recently downloaded in this chat
func (h *Handler) isDuplicateURL(chatKey, urlStr string) bool {
	h.urlCacheMu.RLock()
	defer h.urlCacheMu.RUnlock()

	chatCache, exists := h.recentURLs[chatKey]
	if !exists {
		return false
	}

	cached, exists := chatCache[urlStr]
	if !exists {
		return false
	}

	// Check if expired
	if time.Since(cached.CreatedAt) > duplicateCacheExpiry {
		return false
	}

	return true
}

// markURLDownloaded adds a URL to the duplicate cache for a chat
func (h *Handler) markURLDownloaded(chatKey, urlStr, platform string) {
	h.urlCacheMu.Lock()
	defer h.urlCacheMu.Unlock()

	if h.recentURLs[chatKey] == nil {
		h.recentURLs[chatKey] = make(map[string]*cachedURL)
	}

	h.recentURLs[chatKey][urlStr] = &cachedURL{
		URL:       urlStr,
		Platform:  platform,
		CreatedAt: time.Now(),
	}

	// Cleanup expired entries for this chat
	for u, cached := range h.recentURLs[chatKey] {
		if time.Since(cached.CreatedAt) > duplicateCacheExpiry {
			delete(h.recentURLs[chatKey], u)
		}
	}
}

// truncateError truncates long error messages
func truncateError(msg string) string {
	if len(msg) > 200 {
		return msg[:200] + "..."
	}
	return msg
}

// sendVideo uploads and sends a video
func (h *Handler) sendVideo(chat types.JID, data []byte, caption string) error {
	uploaded, err := h.client.Upload(context.Background(), data, whatsmeow.MediaVideo)
	if err != nil {
		return fmt.Errorf("upload video: %w", err)
	}

	msg := &waProto.Message{
		VideoMessage: &waProto.VideoMessage{
			Caption:       proto.String(caption),
			URL:           proto.String(uploaded.URL),
			DirectPath:    proto.String(uploaded.DirectPath),
			MediaKey:      uploaded.MediaKey,
			Mimetype:      proto.String("video/mp4"),
			FileEncSHA256: uploaded.FileEncSHA256,
			FileSHA256:    uploaded.FileSHA256,
			FileLength:    proto.Uint64(uint64(len(data))),
		},
	}

	_, err = h.client.SendMessage(context.Background(), chat, msg)
	return err
}

// sendImage uploads and sends an image
func (h *Handler) sendImage(chat types.JID, data []byte, caption string) error {
	uploaded, err := h.client.Upload(context.Background(), data, whatsmeow.MediaImage)
	if err != nil {
		return fmt.Errorf("upload image: %w", err)
	}

	msg := &waProto.Message{
		ImageMessage: &waProto.ImageMessage{
			Caption:       proto.String(caption),
			URL:           proto.String(uploaded.URL),
			DirectPath:    proto.String(uploaded.DirectPath),
			MediaKey:      uploaded.MediaKey,
			Mimetype:      proto.String("image/jpeg"),
			FileEncSHA256: uploaded.FileEncSHA256,
			FileSHA256:    uploaded.FileSHA256,
			FileLength:    proto.Uint64(uint64(len(data))),
		},
	}

	_, err = h.client.SendMessage(context.Background(), chat, msg)
	return err
}

// sendAudio uploads and sends an audio file
func (h *Handler) sendAudio(chat types.JID, data []byte) error {
	uploaded, err := h.client.Upload(context.Background(), data, whatsmeow.MediaAudio)
	if err != nil {
		return fmt.Errorf("upload audio: %w", err)
	}

	msg := &waProto.Message{
		AudioMessage: &waProto.AudioMessage{
			URL:           proto.String(uploaded.URL),
			DirectPath:    proto.String(uploaded.DirectPath),
			MediaKey:      uploaded.MediaKey,
			Mimetype:      proto.String("audio/mpeg"),
			FileEncSHA256: uploaded.FileEncSHA256,
			FileSHA256:    uploaded.FileSHA256,
			FileLength:    proto.Uint64(uint64(len(data))),
		},
	}

	_, err = h.client.SendMessage(context.Background(), chat, msg)
	return err
}

// sendDocument uploads and sends a document (fallback)
func (h *Handler) sendDocument(chat types.JID, data []byte, filename string, caption string) error {
	uploaded, err := h.client.Upload(context.Background(), data, whatsmeow.MediaDocument)
	if err != nil {
		return fmt.Errorf("upload document: %w", err)
	}

	msg := &waProto.Message{
		DocumentMessage: &waProto.DocumentMessage{
			Caption:       proto.String(caption),
			URL:           proto.String(uploaded.URL),
			DirectPath:    proto.String(uploaded.DirectPath),
			MediaKey:      uploaded.MediaKey,
			Mimetype:      proto.String("application/octet-stream"),
			FileName:      proto.String(filename),
			FileEncSHA256: uploaded.FileEncSHA256,
			FileSHA256:    uploaded.FileSHA256,
			FileLength:    proto.Uint64(uint64(len(data))),
		},
	}

	_, err = h.client.SendMessage(context.Background(), chat, msg)
	return err
}

// mediaTypeStr returns a human-readable string for a media type
func mediaTypeStr(mt downloader.MediaType) string {
	switch mt {
	case downloader.MediaTypeVideo:
		return "video"
	case downloader.MediaTypeImage:
		return "image"
	case downloader.MediaTypeAudio:
		return "audio"
	default:
		return "document"
	}
}
