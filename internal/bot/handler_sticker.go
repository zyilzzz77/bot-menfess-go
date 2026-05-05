package bot

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/draw"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"math"
	"time"

	gowebp "github.com/urtie/gowebp"
	"go.mau.fi/whatsmeow"
	waProto "go.mau.fi/whatsmeow/binary/proto"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	xdraw "golang.org/x/image/draw"
	"google.golang.org/protobuf/proto"
)

const stickerCanvasSize = 512
const stickerWebPQuality = 80
const stickerWebPEffort = 4

// handleStickerFromImage converts an image message into a WhatsApp sticker
func (h *Handler) handleStickerFromImage(evt *events.Message, imgMsg *waProto.ImageMessage) {
	chat := evt.Info.Chat
	sender := evt.Info.Sender.User

	fmt.Printf("🖼️ [Sticker] Request from %s\n", sender)
	h.reactToRequest(evt, "⚡")

	// Download the image from WhatsApp
	data, err := h.client.Download(context.Background(), imgMsg)
	if err != nil {
		h.sendText(chat, fmt.Sprintf("❌ Gagal download gambar: %s", err.Error()))
		return
	}

	if err := h.sendStickerFromImageData(chat, sender, data); err != nil {
		h.sendText(chat, fmt.Sprintf("❌ Gagal memproses foto jadi sticker: %s", truncateError(err.Error())))
		return
	}
}

// sendStickerFromImageData converts raw image bytes into a WhatsApp sticker.
func (h *Handler) sendStickerFromImageData(chat types.JID, sender string, data []byte) error {
	stickerSource, err := decodeStickerSource(data)
	if err != nil {
		return fmt.Errorf("decode sticker source: %w", err)
	}

	stickerCanvas := fitStickerCanvas(stickerSource)

	var stickerBuf bytes.Buffer
	if err := gowebp.Encode(&stickerBuf, stickerCanvas, &gowebp.EncodeOptions{
		Mode:    gowebp.EncodeLossy,
		Quality: gowebp.LossyQuality(stickerWebPQuality),
		Effort:  gowebp.EncodingEffort(stickerWebPEffort),
	}); err != nil {
		return fmt.Errorf("encode sticker: %w", err)
	}

	stickerData := stickerBuf.Bytes()
	uploaded, err := h.client.Upload(context.Background(), stickerData, whatsmeow.MediaImage)
	if err != nil {
		return fmt.Errorf("upload sticker: %w", err)
	}

	now := time.Now().Unix()
	bounds := stickerCanvas.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()

	msg := &waProto.Message{
		StickerMessage: &waProto.StickerMessage{
			URL:               proto.String(uploaded.URL),
			DirectPath:        proto.String(uploaded.DirectPath),
			MediaKey:          uploaded.MediaKey,
			Mimetype:          proto.String("image/webp"),
			FileEncSHA256:     uploaded.FileEncSHA256,
			FileSHA256:        uploaded.FileSHA256,
			FileLength:        proto.Uint64(uint64(len(stickerData))),
			Width:             proto.Uint32(uint32(width)),
			Height:            proto.Uint32(uint32(height)),
			MediaKeyTimestamp: proto.Int64(now),
			StickerSentTS:     proto.Int64(now),
			IsAnimated:        proto.Bool(false),
		},
	}

	if _, err := h.client.SendMessage(context.Background(), chat, msg); err != nil {
		return fmt.Errorf("send sticker: %w", err)
	}

	fmt.Printf("✅ [Sticker] Sent sticker to %s (%dx%d)\n", sender, width, height)
	return nil
}

// decodeStickerSource turns downloaded media bytes into an image.Image.
func decodeStickerSource(data []byte) (image.Image, error) {
	decoded, _, decodeErr := image.Decode(bytes.NewReader(data))
	if decodeErr == nil {
		return decoded, nil
	}

	webpDecoded, webpErr := gowebp.Decode(bytes.NewReader(data))
	if webpErr == nil {
		return webpDecoded, nil
	}

	return nil, fmt.Errorf("image decode failed: %v; webp decode failed: %v", decodeErr, webpErr)
}

// fitStickerCanvas keeps the original photo ratio and centers it on a square canvas.
func fitStickerCanvas(src image.Image) *image.NRGBA {
	bounds := src.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()
	canvas := image.NewNRGBA(image.Rect(0, 0, stickerCanvasSize, stickerCanvasSize))
	if width <= 0 || height <= 0 {
		return canvas
	}

	scale := float64(stickerCanvasSize) / float64(maxInt(width, height))
	if scale > 1 {
		scale = 1
	}

	targetWidth := int(math.Round(float64(width) * scale))
	targetHeight := int(math.Round(float64(height) * scale))
	if targetWidth < 1 {
		targetWidth = 1
	}
	if targetHeight < 1 {
		targetHeight = 1
	}

	resized := image.NewNRGBA(image.Rect(0, 0, targetWidth, targetHeight))
	xdraw.CatmullRom.Scale(resized, resized.Bounds(), src, bounds, xdraw.Over, nil)

	offsetX := (stickerCanvasSize - targetWidth) / 2
	offsetY := (stickerCanvasSize - targetHeight) / 2
	draw.Draw(canvas, image.Rect(offsetX, offsetY, offsetX+targetWidth, offsetY+targetHeight), resized, image.Point{}, draw.Over)

	return canvas
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
