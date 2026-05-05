package bot

import (
	"context"
	"fmt"
	"sync"
	"time"

	"go.mau.fi/whatsmeow"
	waProto "go.mau.fi/whatsmeow/binary/proto"
	"go.mau.fi/whatsmeow/types"
	"google.golang.org/protobuf/proto"
)

// progressTracker manages a live-updating progress message
type progressTracker struct {
	client    *whatsmeow.Client
	chat      types.JID
	messageID string
	startTime time.Time
	mu        sync.Mutex
	stopped   bool
	stopCh    chan struct{}
	stage     string
}

// newProgressTracker sends an initial progress message and starts live updates
func (h *Handler) newProgressTracker(chat types.JID, platformLabel string) *progressTracker {
	initialText := "⏳"

	resp, err := h.client.SendMessage(context.Background(), chat, &waProto.Message{
		Conversation: proto.String(initialText),
	})
	if err != nil {
		fmt.Printf("❌ Failed to send progress message: %v\n", err)
		return nil
	}

	pt := &progressTracker{
		client:    h.client,
		chat:      chat,
		messageID: resp.ID,
		startTime: time.Now(),
		stopCh:    make(chan struct{}),
	}

	return pt
}

// runUpdates periodically edits the progress message with elapsed time
func (pt *progressTracker) runUpdates(platformLabel string) {
	ticker := time.NewTicker(progressInterval)
	defer ticker.Stop()

	for {
		select {
		case <-pt.stopCh:
			return
		case <-ticker.C:
			pt.mu.Lock()
			if pt.stopped {
				pt.mu.Unlock()
				return
			}
			elapsed := time.Since(pt.startTime).Truncate(time.Second)
			stage := pt.stage
			pt.mu.Unlock()

			text := fmt.Sprintf("⏳ *Downloading from %s...*\n\n⏱️ Elapsed: %s\n%s",
				platformLabel, elapsed, stage)
			pt.editMessage(text)
		}
	}
}

// setStage updates the current stage text
func (pt *progressTracker) setStage(stage string) {
	pt.mu.Lock()
	defer pt.mu.Unlock()
	pt.stage = stage
}

// editMessage edits the progress message with new text
func (pt *progressTracker) editMessage(newText string) {
	chatJID := pt.chat.String()

	editMsg := &waProto.Message{
		ProtocolMessage: &waProto.ProtocolMessage{
			Key: &waProto.MessageKey{
				RemoteJID: proto.String(chatJID),
				FromMe:    proto.Bool(true),
				ID:        proto.String(pt.messageID),
			},
			Type: waProto.ProtocolMessage_MESSAGE_EDIT.Enum(),
			EditedMessage: &waProto.Message{
				Conversation: proto.String(newText),
			},
		},
	}

	_, err := pt.client.SendMessage(context.Background(), pt.chat, editMsg)
	if err != nil {
		fmt.Printf("⚠️ Failed to edit progress message: %v\n", err)
	}
}

// finish stops the progress updates and shows final message
func (pt *progressTracker) finish(finalText string) {
	pt.mu.Lock()
	pt.stopped = true
	pt.mu.Unlock()

	close(pt.stopCh)

	pt.editMessage(finalText)
}
