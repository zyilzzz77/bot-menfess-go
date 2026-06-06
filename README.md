# 🤖 Bot WA AI LLM · Downloader v4.0

Bot WhatsApp dengan **dual function**: download media sosial + AI chat.

---

## Fitur

### 📥 Download Media Sosial
| Platform | Media |
|----------|-------|
| 🎵 TikTok | Video, Photo Slideshow, Audio, **Tombol Audio** |
| 📸 Instagram | Reels, Posts, Carousel |
| 🐦 Twitter/X | Photos, Videos |
| 🎬 Snack Video | Video |
| 🎧 Spotify | Lagu (Cover + Audio) |
| 🎧 Spotify Playlist | Multi-track download |
| 🧵 Threads | Photos, Videos |

### 🧠 AI Chat (Hermes Agent + DeepSeek v4-pro)
- **Auto-AI**: Kirim teks biasa, langsung dijawab AI (tanpa `/ai`)
- **Thinking Mode**: DeepSeek v4-pro reasoning
- **Image Analysis**: Kirim gambar + caption `/ai`
- **Ratusan Tools**: Web search, browser, terminal, skills, memory

---

## Cara Pakai

```
Kirim link sosmed → Auto download
Kirim teks biasa → Auto AI
Kirim gambar + /ai → Analisis gambar
```

---

## Deploy

```bash
# Clone
git clone https://github.com/zyilzzz77/bot-menfess-go.git
cd bot-menfess-go

# Setup .env (copy dari .env.example, isi API keys)
cp .env.example .env
nano .env

# Run
docker compose up -d --build
```

### Environment Variables

```env
API_KEY=                # API key neoxr.eu (WAJIB)
TELEGRAM_BOT_TOKEN=     # Token @BotFather
TELEGRAM_ADMIN_ID=      # Chat ID admin
DEEPSEEK_API_KEY=       # API key DeepSeek
HERMES_API_URL=         # Hermes Agent API (opsional)
HERMES_API_KEY=         # Hermes API_SERVER_KEY (opsional)
```

---

## Tech Stack

| Komponen | Teknologi |
|----------|-----------|
| Language | Go 1.25+ |
| WhatsApp | whatsmeow |
| AI Engine | Hermes Agent + DeepSeek v4-pro |
| Download API | api.neoxr.eu |
| Container | Docker + Alpine |
