package telegram

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Bot represents a Telegram bot for remote monitoring and control
type Bot struct {
	Token      string
	AdminID    int64
	httpClient *http.Client
	baseURL    string

	// Callbacks for controlling the WA bot
	OnPair      func(phone string) (string, error) // returns pairing code
	OnReconnect func() error
	OnStatus    func() string
	OnLogout    func() error

	lastUpdateID int64
	running      bool
}

// NewBot creates a new Telegram bot instance
func NewBot(token string, adminID int64) *Bot {
	return &Bot{
		Token:   token,
		AdminID: adminID,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		baseURL: fmt.Sprintf("https://api.telegram.org/bot%s", token),
	}
}

// IsConfigured returns true if the bot has valid credentials
func (b *Bot) IsConfigured() bool {
	return b.Token != "" && b.AdminID != 0
}

// === Telegram API Types ===

type telegramUpdate struct {
	UpdateID int64            `json:"update_id"`
	Message  *telegramMessage `json:"message"`
}

type telegramMessage struct {
	MessageID int64        `json:"message_id"`
	From      telegramUser `json:"from"`
	Chat      telegramChat `json:"chat"`
	Text      string       `json:"text"`
}

type telegramUser struct {
	ID int64 `json:"id"`
}

type telegramChat struct {
	ID int64 `json:"id"`
}

type telegramResponse struct {
	OK     bool              `json:"ok"`
	Result json.RawMessage   `json:"result"`
	Desc   string            `json:"description"`
}

// === Notification Methods ===

// SendNotification sends a text message to the admin
func (b *Bot) SendNotification(text string) error {
	if !b.IsConfigured() {
		return nil
	}

	params := url.Values{
		"chat_id":    {strconv.FormatInt(b.AdminID, 10)},
		"text":       {text},
		"parse_mode": {"Markdown"},
	}

	resp, err := b.httpClient.PostForm(b.baseURL+"/sendMessage", params)
	if err != nil {
		return fmt.Errorf("telegram send failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var result telegramResponse
	json.Unmarshal(body, &result)

	if !result.OK {
		return fmt.Errorf("telegram API error: %s", result.Desc)
	}

	return nil
}

// NotifyStarted sends a bot started notification
func (b *Bot) NotifyStarted() {
	b.SendNotification("✅ *WhatsApp Bot Started*\n\n🤖 Bot berhasil dijalankan dan siap menerima pesan.\n\nKetik /help untuk melihat perintah yang tersedia.")
}

// NotifyConnected sends a connected notification
func (b *Bot) NotifyConnected() {
	b.SendNotification("📡 *WhatsApp Connected*\n\n✅ Terhubung ke WhatsApp server.")
}

// NotifyDisconnected sends a disconnected notification
func (b *Bot) NotifyDisconnected() {
	b.SendNotification("⚠️ *WhatsApp Disconnected*\n\n❌ Koneksi WhatsApp terputus!\n\n💡 Gunakan /reconnect untuk mencoba menyambungkan kembali\natau /pair <nomor> untuk pairing ulang.")
}

// NotifyLoggedOut sends a logged out notification
func (b *Bot) NotifyLoggedOut() {
	b.SendNotification("🔒 *WhatsApp Logged Out*\n\n❌ Session WhatsApp telah di-logout!\n\n💡 Gunakan /pair <nomor> untuk login kembali.\nContoh: `/pair 6281234567890`")
}

// NotifyError sends an error notification
func (b *Bot) NotifyError(errMsg string) {
	b.SendNotification(fmt.Sprintf("🚨 *Error*\n\n```\n%s\n```", errMsg))
}

// === Polling & Command Handling ===

// StartPolling starts listening for Telegram commands in a goroutine
func (b *Bot) StartPolling() {
	if !b.IsConfigured() {
		fmt.Println("⚠️  Telegram bot not configured, skipping polling")
		return
	}

	b.running = true
	go b.pollLoop()
	fmt.Println("✅ Telegram bot polling started")
}

// StopPolling stops the polling loop
func (b *Bot) StopPolling() {
	b.running = false
}

// pollLoop continuously polls for new Telegram messages
func (b *Bot) pollLoop() {
	for b.running {
		updates, err := b.getUpdates()
		if err != nil {
			time.Sleep(5 * time.Second)
			continue
		}

		for _, update := range updates {
			if update.Message != nil {
				b.handleCommand(update.Message)
			}
			b.lastUpdateID = update.UpdateID + 1
		}

		time.Sleep(1 * time.Second)
	}
}

// getUpdates fetches new messages from Telegram
func (b *Bot) getUpdates() ([]telegramUpdate, error) {
	params := url.Values{
		"offset":  {strconv.FormatInt(b.lastUpdateID, 10)},
		"timeout": {"25"},
	}

	resp, err := b.httpClient.PostForm(b.baseURL+"/getUpdates", params)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var result struct {
		OK     bool             `json:"ok"`
		Result []telegramUpdate `json:"result"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}

	return result.Result, nil
}

// handleCommand processes a single Telegram command
func (b *Bot) handleCommand(msg *telegramMessage) {
	// Only respond to admin
	if msg.From.ID != b.AdminID {
		b.reply(msg.Chat.ID, "⛔ Unauthorized. Bot ini hanya untuk admin.")
		return
	}

	text := strings.TrimSpace(msg.Text)
	parts := strings.Fields(text)
	if len(parts) == 0 {
		return
	}

	cmd := strings.ToLower(parts[0])

	switch cmd {
	case "/start", "/help":
		b.handleHelp(msg.Chat.ID)
	case "/status":
		b.handleStatus(msg.Chat.ID)
	case "/reconnect":
		b.handleReconnect(msg.Chat.ID)
	case "/pair":
		if len(parts) < 2 {
			b.reply(msg.Chat.ID, "❌ *Format salah*\n\nGunakan: `/pair 6281234567890`")
			return
		}
		b.handlePair(msg.Chat.ID, parts[1])
	case "/logout":
		b.handleLogout(msg.Chat.ID)
	default:
		b.reply(msg.Chat.ID, "❓ Command tidak dikenal.\nKetik /help untuk melihat daftar perintah.")
	}
}

// handleHelp sends the help message
func (b *Bot) handleHelp(chatID int64) {
	help := `🤖 *WA Bot Remote Control*

*Commands:*
/status — Cek status koneksi WhatsApp
/reconnect — Coba reconnect ke WhatsApp
/pair <nomor> — Pairing ulang dengan nomor baru
/logout — Logout session WhatsApp
/help — Tampilkan pesan ini

*Contoh:*
` + "`/pair 6281234567890`" + `

*Auto Notifications:*
• ⚠️ Disconnect — saat koneksi terputus
• ✅ Reconnect — saat berhasil tersambung kembali
• 🔒 Logout — saat session di-logout
• 🚨 Error — saat terjadi error`

	b.reply(chatID, help)
}

// handleStatus checks and reports WA connection status
func (b *Bot) handleStatus(chatID int64) {
	if b.OnStatus == nil {
		b.reply(chatID, "⚠️ Status callback not configured")
		return
	}
	status := b.OnStatus()
	b.reply(chatID, fmt.Sprintf("📊 *WhatsApp Bot Status*\n\n%s", status))
}

// handleReconnect attempts to reconnect the WA client
func (b *Bot) handleReconnect(chatID int64) {
	if b.OnReconnect == nil {
		b.reply(chatID, "⚠️ Reconnect callback not configured")
		return
	}

	b.reply(chatID, "🔄 Mencoba reconnect...")

	err := b.OnReconnect()
	if err != nil {
		b.reply(chatID, fmt.Sprintf("❌ *Reconnect gagal*\n\n```\n%s\n```\n\n💡 Coba /pair <nomor> untuk pairing ulang.", err.Error()))
	} else {
		b.reply(chatID, "✅ *Reconnect berhasil!*")
	}
}

// handlePair generates a new pairing code for the given phone number
func (b *Bot) handlePair(chatID int64, phone string) {
	if b.OnPair == nil {
		b.reply(chatID, "⚠️ Pair callback not configured")
		return
	}

	// Validate phone number format
	if !strings.HasPrefix(phone, "+") && !strings.HasPrefix(phone, "62") && !strings.HasPrefix(phone, "0") {
		b.reply(chatID, "⚠️ Format nomor tidak valid.\nGunakan format: `6281234567890`")
		return
	}

	b.reply(chatID, fmt.Sprintf("🔑 Generating pairing code untuk %s...", phone))

	code, err := b.OnPair(phone)
	if err != nil {
		b.reply(chatID, fmt.Sprintf("❌ *Pairing gagal*\n\n```\n%s\n```", err.Error()))
		return
	}

	b.reply(chatID, fmt.Sprintf("🔑 *Pairing Code:* `%s`\n\n📱 Buka WhatsApp di HP:\n1. Settings → Linked Devices\n2. Link a Device\n3. Pilih 'Link with phone number instead'\n4. Masukkan kode di atas\n\n⏳ Kode berlaku beberapa menit.", code))
}

// handleLogout logs out the WA session
func (b *Bot) handleLogout(chatID int64) {
	if b.OnLogout == nil {
		b.reply(chatID, "⚠️ Logout callback not configured")
		return
	}

	b.reply(chatID, "🔒 Logging out WhatsApp session...")

	err := b.OnLogout()
	if err != nil {
		b.reply(chatID, fmt.Sprintf("❌ *Logout gagal*\n\n```\n%s\n```", err.Error()))
	} else {
		b.reply(chatID, "✅ *Session logged out.*\n\nGunakan /pair <nomor> untuk login kembali.")
	}
}

// reply sends a text message to a specific chat
func (b *Bot) reply(chatID int64, text string) {
	params := url.Values{
		"chat_id":    {strconv.FormatInt(chatID, 10)},
		"text":       {text},
		"parse_mode": {"Markdown"},
	}

	resp, err := b.httpClient.PostForm(b.baseURL+"/sendMessage", params)
	if err != nil {
		fmt.Printf("❌ Telegram reply failed: %v\n", err)
		return
	}
	resp.Body.Close()
}
