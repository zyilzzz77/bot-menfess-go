package bot

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"

	"bot-wa/internal/ai"
	"bot-wa/internal/downloader"
	"bot-wa/internal/telegram"
)

// Config holds all bot configuration from environment variables.
type Config struct {
	APIKey         string
	DownloadDir    string
	DeepSeekKey    string
	AIModel        string
	AISystemPrompt string
	HermesURL      string // optional: Hermes Agent API base URL
	HermesKey      string // optional: Hermes API_SERVER_KEY
	TgBot          *telegram.Bot
}

// Bot represents the WhatsApp bot instance
type Bot struct {
	client     *whatsmeow.Client
	handler    *Handler
	downloader *downloader.Downloader
	ai         *ai.Client
	tgBot      *telegram.Bot
	container  *sqlstore.Container

	mu           sync.Mutex
	reconnectMu  sync.Mutex
	reconnecting bool
	isConnected  bool
}

// establishConnection connects the client with retry and optional session reset.
func (b *Bot) establishConnection(reset bool) error {
	b.reconnectMu.Lock()
	if b.reconnecting {
		b.reconnectMu.Unlock()
		return fmt.Errorf("reconnect already in progress")
	}
	b.reconnecting = true
	b.reconnectMu.Unlock()
	defer func() {
		b.reconnectMu.Lock()
		b.reconnecting = false
		b.reconnectMu.Unlock()
	}()

	if reset {
		b.client.Disconnect()
		time.Sleep(750 * time.Millisecond)
	}

	const maxAttempts = 3
	var lastErr error

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if attempt > 1 {
			b.client.Disconnect()
			time.Sleep(time.Duration(attempt) * time.Second)
		}

		if err := b.client.Connect(); err != nil {
			lastErr = err
			fmt.Printf("⚠️ WhatsApp connect attempt %d/%d failed: %v\n", attempt, maxAttempts, err)
			continue
		}

		b.mu.Lock()
		b.isConnected = true
		b.mu.Unlock()
		return nil
	}

	return fmt.Errorf("failed to connect after %d attempts: %w", maxAttempts, lastErr)
}

// NewBot creates a new Bot instance from Config.
func NewBot(cfg Config) *Bot {
	dl := downloader.NewDownloader(cfg.APIKey, cfg.DownloadDir)

	var aiClient *ai.Client
	if cfg.DeepSeekKey != "" || cfg.HermesURL != "" {
		aiClient = ai.NewClient(ai.Config{
			DeepSeekKey:  cfg.DeepSeekKey,
			Model:        cfg.AIModel,
			SystemPrompt: cfg.AISystemPrompt,
			HermesURL:    cfg.HermesURL,
			HermesKey:    cfg.HermesKey,
		})
		if cfg.HermesURL != "" {
			fmt.Println("✅ AI (Hermes) configured")
		} else {
			fmt.Println("✅ AI (DeepSeek) configured")
		}
	} else {
		fmt.Println("ℹ️  AI features not configured (set HERMES_API_URL or DEEPSEEK_API_KEY in .env)")
	}

	return &Bot{
		downloader: dl,
		ai:         aiClient,
		tgBot:      cfg.TgBot,
	}
}

// Start initializes the WhatsApp client and starts the bot
func (b *Bot) Start() error {
	// Validate API key
	if b.downloader.APIKey == "" {
		return fmt.Errorf("API_KEY tidak boleh kosong. Set di file .env atau environment variable")
	}
	fmt.Println("✅ API Key configured")

	// Setup logger
	dbLog := waLog.Stdout("Database", "WARN", true)

	// Create SQLite store for session persistence
	container, err := sqlstore.New(context.Background(), "sqlite3", "file:store/wa_session.db?_foreign_keys=on", dbLog)
	if err != nil {
		return fmt.Errorf("failed to create session store: %w", err)
	}
	b.container = container

	// Initialize WA client
	if err := b.initClient(); err != nil {
		return err
	}

	// Setup Telegram bot callbacks
	b.setupTelegramCallbacks()

	// Start Telegram polling
	if b.tgBot != nil && b.tgBot.IsConfigured() {
		b.tgBot.StartPolling()
	}

	// Connect to WhatsApp
	if b.client.Store.ID == nil {
		// No session — need pairing
		if b.tgBot != nil && b.tgBot.IsConfigured() {
			// If Telegram is configured, prompt via Telegram
			fmt.Println("📱 No WhatsApp session found.")
			fmt.Println("💡 Kirim /pair <nomor> di Telegram untuk login.")
			b.tgBot.SendNotification("📱 *WhatsApp Bot Started*\n\n⚠️ Belum ada session WhatsApp.\nGunakan /pair <nomor> untuk login.\n\nContoh: `/pair 6281234567890`")
		} else {
			// No Telegram — use console pairing
			if err := b.consolePairing(); err != nil {
				return err
			}
		}
	} else {
		// Session exists — reconnect
		fmt.Println("🔄 Reconnecting with existing session...")
		if err := b.establishConnection(false); err != nil {
			return fmt.Errorf("failed to connect: %w", err)
		}
		fmt.Println("✅ Connected successfully!")
	}

	fmt.Println()
	fmt.Println("🤖 Bot is now running!")
	fmt.Println("📥 Send a social media link to download media")
	fmt.Println("⏹  Press Ctrl+C to stop")
	fmt.Println()

	// Wait for interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

	fmt.Println("\n👋 Shutting down bot...")
	if b.tgBot != nil {
		b.tgBot.StopPolling()
		b.tgBot.SendNotification("👋 *WhatsApp Bot Stopped*\n\nBot telah di-shutdown.")
	}
	b.client.Disconnect()

	return nil
}

// initClient creates a new WhatsApp client from the container
func (b *Bot) initClient() error {
	deviceStore, err := b.container.GetFirstDevice(context.Background())
	if err != nil {
		return fmt.Errorf("failed to get device: %w", err)
	}

	clientLog := waLog.Stdout("Client", "WARN", true)
	b.client = whatsmeow.NewClient(deviceStore, clientLog)

	// Create message handler
	b.handler = NewHandler(b.client, b.downloader, b.ai)

	// Register event handler
	b.client.AddEventHandler(b.eventHandler)

	return nil
}

// setupTelegramCallbacks wires the Telegram bot commands to WA bot actions
func (b *Bot) setupTelegramCallbacks() {
	if b.tgBot == nil || !b.tgBot.IsConfigured() {
		return
	}

	// /status command
	b.tgBot.OnStatus = func() string {
		b.mu.Lock()
		connected := b.isConnected
		b.mu.Unlock()

		hasSession := b.client.Store.ID != nil

		status := ""
		if connected {
			status += "🟢 *Koneksi:* Connected\n"
		} else {
			status += "🔴 *Koneksi:* Disconnected\n"
		}

		if hasSession {
			status += "📱 *Session:* Active\n"
			status += fmt.Sprintf("🆔 *Device:* %s\n", b.client.Store.ID.String())
		} else {
			status += "📱 *Session:* No session\n"
		}

		return status
	}

	// /reconnect command
	b.tgBot.OnReconnect = func() error {
		if b.client.Store.ID == nil {
			return fmt.Errorf("tidak ada session. Gunakan /pair <nomor> untuk login dulu")
		}
		return b.establishConnection(true)
	}

	// /pair command
	b.tgBot.OnPair = func(phone string) (string, error) {
		// Disconnect existing connection if any
		b.client.Disconnect()

		// Remove old session and WAL files
		os.Remove("store/wa_session.db")
		os.Remove("store/wa_session.db-wal")
		os.Remove("store/wa_session.db-shm")

		// Recreate the database container since we deleted the file
		dbLog := waLog.Stdout("Database", "WARN", true)
		container, err := sqlstore.New(context.Background(), "sqlite3", "file:store/wa_session.db?_foreign_keys=on", dbLog)
		if err != nil {
			return "", fmt.Errorf("failed to create session store: %w", err)
		}
		b.container = container

		// Reinitialize client with fresh device store
		if err := b.initClient(); err != nil {
			return "", fmt.Errorf("failed to reinit client: %w", err)
		}

		// Rewire telegram callbacks after reinit
		b.setupTelegramCallbacks()

		// Connect
		if err := b.establishConnection(false); err != nil {
			return "", fmt.Errorf("failed to connect: %w", err)
		}

		// Request pairing code
		code, err := b.client.PairPhone(context.Background(), phone, true, whatsmeow.PairClientChrome, "Chrome (Linux)")
		if err != nil {
			return "", fmt.Errorf("pairing failed: %w", err)
		}

		return code, nil
	}

	// /logout command
	b.tgBot.OnLogout = func() error {
		if b.client.Store.ID == nil {
			return fmt.Errorf("tidak ada session aktif")
		}
		err := b.client.Logout(context.Background())
		if err != nil {
			// Force cleanup
			b.client.Disconnect()
			os.Remove("store/wa_session.db")
			os.Remove("store/wa_session.db-wal")
			os.Remove("store/wa_session.db-shm")
		}
		return err
	}
}

// consolePairing handles pairing via console (when Telegram is not configured)
func (b *Bot) consolePairing() error {
	fmt.Println("╔══════════════════════════════════════════════╗")
	fmt.Println("║   📱 WhatsApp Social Media Downloader Bot   ║")
	fmt.Println("║──────────────────────────────────────────────║")
	fmt.Println("║   Login menggunakan Pairing Code             ║")
	fmt.Println("╚══════════════════════════════════════════════╝")
	fmt.Println()

	fmt.Print("📞 Masukkan nomor WhatsApp (contoh: 6281234567890): ")
	reader := bufio.NewReader(os.Stdin)
	phoneNumber, _ := reader.ReadString('\n')
	phoneNumber = strings.TrimSpace(phoneNumber)

	if phoneNumber == "" {
		return fmt.Errorf("nomor telepon tidak boleh kosong")
	}

	if err := b.establishConnection(false); err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}

	code, err := b.client.PairPhone(context.Background(), phoneNumber, true, whatsmeow.PairClientChrome, "Chrome (Linux)")
	if err != nil {
		return fmt.Errorf("failed to get pairing code: %w", err)
	}

	fmt.Println()
	fmt.Println("╔══════════════════════════════════════════════╗")
	fmt.Printf("║         🔑 PAIRING CODE: %s          ║\n", code)
	fmt.Println("║──────────────────────────────────────────────║")
	fmt.Println("║  Buka WhatsApp di HP:                        ║")
	fmt.Println("║  Settings → Linked Devices → Link Device    ║")
	fmt.Println("║  Pilih 'Link with phone number instead'     ║")
	fmt.Println("║  Masukkan kode di atas                       ║")
	fmt.Println("╚══════════════════════════════════════════════╝")
	fmt.Println()
	fmt.Println("⏳ Menunggu konfirmasi dari WhatsApp...")

	return nil
}

// eventHandler handles all WhatsApp events
func (b *Bot) eventHandler(evt interface{}) {
	switch v := evt.(type) {
	case *events.Message:
		go b.handler.HandleMessage(v)

	case *events.Connected:
		b.mu.Lock()
		b.isConnected = true
		b.mu.Unlock()
		fmt.Println("📡 WhatsApp connection established")
		if b.tgBot != nil {
			b.tgBot.NotifyConnected()
		}

	case *events.PairSuccess:
		fmt.Printf("✅ Login berhasil! Terhubung sebagai %s\n", v.ID.String())
		if b.tgBot != nil {
			b.tgBot.SendNotification(fmt.Sprintf("✅ *Pairing Berhasil!*\n\nTerhubung sebagai `%s`", v.ID.String()))
		}

	case *events.Disconnected:
		b.mu.Lock()
		b.isConnected = false
		b.mu.Unlock()
		fmt.Println("⚠️  WhatsApp disconnected")
		b.reconnectMu.Lock()
		reconnecting := b.reconnecting
		b.reconnectMu.Unlock()
		if b.tgBot != nil && !reconnecting {
			b.tgBot.NotifyDisconnected()
		}

	case *events.LoggedOut:
		b.mu.Lock()
		b.isConnected = false
		b.mu.Unlock()
		fmt.Println("🔒 Session logged out.")
		os.Remove("store/wa_session.db")
		os.Remove("store/wa_session.db-wal")
		os.Remove("store/wa_session.db-shm")
		if b.tgBot != nil {
			b.tgBot.NotifyLoggedOut()
		}
		// Don't exit — let Telegram control handle re-pairing
	}
}
