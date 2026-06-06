package main

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"

	"bot-wa/internal/bot"
	"bot-wa/internal/telegram"
)

func main() {
	fmt.Println("╔══════════════════════════════════════════════════╗")
	fmt.Println("║  🤖 Bot WA AI LLM · Downloader v4.0  ║")
	fmt.Println("║──────────────────────────────────────────────────║")
	fmt.Println("║  Media: TikTok · IG · Twitter · Spotify · Threads ║")
	fmt.Println("║  AI: Hermes Agent + DeepSeek v4-pro · Auto-Ready                   ║")
	fmt.Println("╚══════════════════════════════════════════════════╝")
	fmt.Println()

	// Load .env file
	loadEnv(".env")

	// Configuration
	apiKey := os.Getenv("API_KEY")
	downloadDir := getEnv("DOWNLOAD_DIR", "./downloads")
	tgToken := os.Getenv("TELEGRAM_BOT_TOKEN")
	tgAdminStr := os.Getenv("TELEGRAM_ADMIN_ID")

	// AI Configuration (optional)
	deepseekKey := os.Getenv("DEEPSEEK_API_KEY")
	aiModel := getEnv("AI_MODEL", "deepseek-v4-pro")
	aiSystemPrompt := os.Getenv("AI_SYSTEM_PROMPT")
	hermesURL := os.Getenv("HERMES_API_URL")   // optional: Hermes Agent API
	hermesKey := os.Getenv("HERMES_API_KEY")  // optional: Hermes API_SERVER_KEY

	if apiKey == "" {
		fmt.Println("⚠️  API_KEY belum di-set!")
		fmt.Println("📝 Buat file .env dengan isi:")
		fmt.Println("   API_KEY=your_api_key_here")
		fmt.Println()
		os.Exit(1)
	}

	// Ensure directories exist
	os.MkdirAll(downloadDir, 0755)
	os.MkdirAll("store", 0755)

	// Setup Telegram bot (optional)
	var tgBot *telegram.Bot
	if tgToken != "" && tgAdminStr != "" {
		adminID, err := strconv.ParseInt(tgAdminStr, 10, 64)
		if err != nil {
			fmt.Printf("⚠️  TELEGRAM_ADMIN_ID tidak valid: %v\n", err)
		} else {
			tgBot = telegram.NewBot(tgToken, adminID)
			fmt.Println("✅ Telegram bot configured")
		}
	} else {
		fmt.Println("ℹ️  Telegram bot not configured (optional)")
		fmt.Println("   Set TELEGRAM_BOT_TOKEN dan TELEGRAM_ADMIN_ID di .env untuk remote control")
	}

	// Create and start the bot
	b := bot.NewBot(bot.Config{
		APIKey:         apiKey,
		DownloadDir:    downloadDir,
		DeepSeekKey:    deepseekKey,
		AIModel:        aiModel,
		AISystemPrompt: aiSystemPrompt,
		HermesURL:      hermesURL,
		HermesKey:      hermesKey,
		TgBot:          tgBot,
	})
	if err := b.Start(); err != nil {
		fmt.Printf("❌ Fatal error: %v\n", err)
		if tgBot != nil {
			tgBot.NotifyError(err.Error())
		}
		os.Exit(1)
	}
}

// getEnv returns an environment variable value or a default
func getEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

// loadEnv loads key=value pairs from a .env file into environment variables
func loadEnv(filename string) {
	file, err := os.Open(filename)
	if err != nil {
		return // .env file is optional
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			key := strings.TrimSpace(parts[0])
			value := strings.TrimSpace(parts[1])
			value = strings.Trim(value, "\"'")
			os.Setenv(key, value)
		}
	}
}
