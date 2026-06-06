# Bot WA AI LLM · Downloader v4.0

## Overview

Bot WhatsApp berbasis Go dengan **dual function**: download media sosial + AI chat (Hermes Agent + DeepSeek v4-pro). Bot otomatis mendeteksi link sosmed untuk download, atau merespon dengan AI untuk pesan teks biasa (tanpa `/ai` prefix).

---

## Tech Stack

| Komponen | Teknologi |
|----------|-----------|
| Language | Go 1.25+ |
| WhatsApp Client | `go.mau.fi/whatsmeow` |
| SQLite Driver | `github.com/mattn/go-sqlite3` (CGO) |
| Download API | `api.neoxr.eu` (REST API) |
| AI Engine | Hermes Agent + DeepSeek v4-pro (thinking mode) |
| AI Fallback | Direct DeepSeek API (jika Hermes unavailable) |
| Telegram Bot | Native HTTP (no external lib) |
| Auth | Phone pairing code (8 digit) |
| Container | Docker multi-stage build, Alpine |

---

## Project Structure

```
bot-wa/
├── main.go                              # Entry point, .env loader, init Telegram + WA bot
├── go.mod / go.sum                      # Go module dependencies
├── .env                                 # Environment variables (API_KEY, TELEGRAM_*)
├── .env.example                         # Template .env
├── .gitignore                           # Ignores: .env, store/, downloads/, *.exe
├── Dockerfile                           # Multi-stage build (builder → Alpine runtime)
├── docker-compose.yml                   # Container orchestration with volumes
├── store/                               # SQLite session database (auto-created)
│   └── wa_session.db                    # WhatsApp session persistence
├── downloads/                           # Temporary media files (auto-cleaned)
└── internal/
    ├── bot/
    │   ├── bot.go                       # WhatsApp client lifecycle, Telegram integration
    │   ├── handler.go                   # Router: commands, URLs, playlist reply, menfess entrypoint
    │   ├── handler_brat.go              # Brat text-to-sticker command
    │   ├── handler_download.go          # Download flow, anti-spam, TikTok audio button
    │   ├── handler_menfess.go           # Anonymous relay + balas command
    │   ├── handler_messaging.go         # Shared send/edit/react helpers
    │   ├── handler_playlist.go          # Spotify playlist flow
        │   └── handler_sticker.go           # Photo-to-sticker flow with square canvas
    ├── downloader/
    │   ├── downloader.go                # Core: types, TikTok API, file downloader, routing
    │   ├── instagram.go                 # Instagram API downloader
    │   ├── snackvideo.go                # Snack Video API downloader
    │   ├── spotify.go                   # Spotify single track downloader
    │   ├── spotify_playlist.go          # Spotify playlist fetcher
    │   ├── threads.go                   # Threads API downloader
    │   ├── twitter.go                   # Twitter/X API downloader
    │   └── brat.go                      # Brat sticker generator API wrapper
    ├── telegram/
    │   └── telegram.go                  # Telegram bot: polling, commands, notifications
    ├── ai/
    │   └── ai.go                       # AI client: DeepSeek + Hermes Agent integration
    └── utils/
        └── url.go                       # URL detection, platform patterns, regex matching
```

---

## Environment Variables (.env)

```env
# WAJIB
API_KEY=OXlJB9                    # API key dari neoxr.eu

# OPSIONAL
DOWNLOAD_DIR=./downloads          # Direktori download sementara
TELEGRAM_BOT_TOKEN=               # Token dari @BotFather
TELEGRAM_ADMIN_ID=                # Chat ID admin (dari @userinfobot)
DEEPSEEK_API_KEY=                 # API key DeepSeek
HERMES_API_URL=                   # Hermes Agent API (localhost:8642/v1)
HERMES_API_KEY=                   # Hermes API_SERVER_KEY
```

> Untuk deploy ke VPS/headless, isi `TELEGRAM_BOT_TOKEN` dan `TELEGRAM_ADMIN_ID` supaya pairing bisa dilakukan lewat Telegram `/pair`.

---

## Supported Platforms & API Endpoints

### 1. 🎵 TikTok
- **API:** `GET /api/snaptik-v2?url={url}&apikey={key}`
- **Media:** Video (HD preferred), Photo Slideshow, Audio
- **File:** `internal/downloader/downloader.go` → `DownloadTikTok()`
- **Response:** `data.video`, `data.videoHD`, `data.audio`, `data.photo[]`
- **Logic:** Cek `photo[]` dulu → jika ada, kirim sebagai images. Jika tidak, ambil `videoHD` (fallback `video`). Jika keduanya kosong, ambil `audio`.

### 2. 📸 Instagram
- **API:** `GET /api/ig?url={url}&apikey={key}`
- **Media:** Reels, Posts, Carousel (multi-media)
- **File:** `internal/downloader/instagram.go` → `DownloadInstagram()`
- **Response:** `data[]` array of `{type, url}`
- **Logic:** Iterasi setiap item, deteksi tipe dari field `type` (mp4→video, jpg→image, dll). Support carousel (multiple items).

### 3. 🎬 Snack Video
- **API:** `GET /api/snackvid?url={url}&apikey={key}`
- **Media:** Video only
- **File:** `internal/downloader/snackvideo.go` → `DownloadSnackVideo()`
- **Response:** `data.url` (direct MP4 URL)
- **Logic:** Download langsung dari `data.url`, set title dari `data.author`.

### 4. 🎧 Spotify (Single Track)
- **API:** `GET /api/spotify?url={url}&apikey={key}`
- **Media:** Cover Art (thumbnail) + Audio MP3
- **File:** `internal/downloader/spotify.go` → `DownloadSpotify()`
- **Response:** `data.thumbnail`, `data.url` (full), `data.preview` (fallback)
- **Logic:** Download thumbnail sebagai image + audio (prefer full URL, fallback ke preview). Dikirim 2 media: cover art → audio.

### 5. 🎧 Spotify Playlist
- **API:** Same endpoint, beda URL format (playlist vs track)
- **File:** `internal/downloader/spotify_playlist.go` → `FetchSpotifyPlaylist()`
- **Response:** `data.cover`, `data.title`, `tracks[]` with individual track URLs
- **Logic:** Fetch playlist → tampilkan daftar → user pilih → download per-track via single track API

### 6. 🧵 Threads
- **API:** `GET /api/threads?url={url}&apikey={key}`
- **Media:** Photos, Videos, Carousel (multi-media)
- **File:** `internal/downloader/threads.go` → `DownloadThreads()`
- **Response:** `data[]` array of `{type, url}` (sama format dengan Instagram)
- **Logic:** Iterasi setiap item, deteksi tipe dari field `type` (mp4→video, jpg→image). Support carousel. Title auto-generated: "Threads Photo", "Threads Video", "Threads Carousel (N media)".

---



---

## AI Chat (Hermes Agent + DeepSeek v4-pro)

**Auto-AI:** Semua pesan teks tanpa link sosmed otomatis dijawab AI. Tidak perlu  prefix.

| Fitur | Deskripsi |
|-------|-----------|
| Thinking Mode |  via DeepSeek v4-pro |
| Image Analysis | Kirim gambar + caption  |
| Hermes Agent | Ratusan tools (web search, browser, skills, memory) |
| Direct Fallback | Jika Hermes unavailable, bot langsung ke DeepSeek |

### AI Architecture

WhatsApp -> Bot Go -> [Hermes Agent (port 8642) / DeepSeek Direct]

Hermes menyediakan OpenAI-compatible API di .

## URL Detection (`internal/utils/url.go`)

Platform dideteksi menggunakan regex patterns yang dicompile saat startup:

| Platform | URL Patterns |
|----------|-------------|
| TikTok | `tiktok.com/@*/video/*`, `vt.tiktok.com/*`, `tiktok.com/t/*` |
| Instagram | `instagram.com/p/*`, `instagram.com/reel/*`, `instagram.com/stories/*` |
| Facebook | `facebook.com/*/videos/*`, `facebook.com/reel/*`, `fb.watch/*` |
| Twitter/X | `twitter.com/*/status/*`, `x.com/*/status/*`, `t.co/*` |
| Snack Video | `snackvideo.com/*`, `sck.io/p/*`, `sck.io/*` |
| Spotify Track | `open.spotify.com/track/*`, `spotify.link/*` |
| Spotify Playlist | `open.spotify.com/playlist/*` |
| Threads | `threads.net/@*/post/*`, `threads.net/t/*`, `threads.com/@*/post/*` |

> **Penting:** Spotify Playlist patterns harus dicek SEBELUM Spotify Track agar playlist URL tidak salah match sebagai track.

---

## Core Architecture

### Data Types (`internal/downloader/downloader.go`)

```go
type MediaType int // MediaTypeVideo=0, MediaTypeImage=1, MediaTypeAudio=2

type MediaItem struct {
    FilePath  string    // Absolute path ke file yang didownload
    FileName  string    // Nama file
    MediaType MediaType // Tipe media
    FileSize  int64     // Ukuran file dalam bytes
}

type DownloadResult struct {
    Items []*MediaItem // Bisa lebih dari 1 (carousel, cover+audio)
    Title string       // Judul media
}
```

### Download Routing

`DownloadGeneric(ctx, platform, url)` → switch berdasarkan platform string:
- `"tiktok"` → `DownloadTikTok()`
- `"instagram"` → `DownloadInstagram()`
- `"snackvideo"` → `DownloadSnackVideo()`
- `"spotify"` → `DownloadSpotify()`
- `"threads"` → `DownloadThreads()`

### Shared `downloadFile()` method
Semua downloader menggunakan satu method `downloadFile(ctx, url, destPath)` yang:
- Set User-Agent header (mimic browser)
- Stream response body ke file
- Handle error dan cleanup file yang gagal

---

## Message Processing Flow (`internal/bot/handler.go`)

### HandleMessage Flow

```
Message masuk
    │
    ├─ Ignore if: from self, broadcast, empty text
    │
    ├─ Check commands: /help, /ping
    ├─ Check menfess flow:
    │   ├─ `menfess <no-wa> <pesan>` → kirim pesan anonim
    │   ├─ `balas (pesan)` → balas menfess aktif tanpa reply quote
    │   └─ reply ke pesan menfess bot → forward ke lawan bicara
    │
    ├─ Check pending playlist reply:
    │   ├─ "all" / "semua" → download all tracks
    │   ├─ "cancel" / "batal" → cancel
    │   ├─ "1,3,5" or "1-10" → download selected tracks
    │   └─ expired (>10min) → cleanup & continue
    │
    ├─ Detect social media URL (regex matching)
    │   ├─ Spotify Playlist → handleSpotifyPlaylist()
    │   └─ Other platforms → processDownload()
    │
    └─ No match → ignored (no reply)
```

### processDownload Flow (Single Media)

```
1. Start progressTracker (sends editable status message)
   └─ Updates every 2s: "⏳ Downloading... ⏱️ Elapsed: Xs"

2. Call API via DownloadGeneric()
   └─ Progress stage: "📥 Mengunduh media..."

3. For each MediaItem in result:
   ├─ Read file data
   ├─ Check size (max 64MB for WhatsApp)
   ├─ Upload & send via WhatsApp (video/image/audio/document)
   ├─ After 1st file sent → create STATUS MESSAGE
   │   └─ Shows: "⏳ Sisa X file lagi... ⏱️ Menunggu Xs..."
   │   └─ Countdown: edits every 1s with remaining seconds
   └─ Delay 3s between files (anti-ban)

4. Finish progressTracker: "✅ Selesai! X/Y media dikirim"
5. Update status message: final summary
6. Cleanup downloaded files
```

### Spotify Playlist Flow

```
1. User sends playlist link
2. Bot: "⏳ Mengambil data playlist..."
3. FetchSpotifyPlaylist() → get track list
4. Bot sends numbered track list:
   "🎧 METAL PHONK
    📀 88 lagu ditemukan
    1. Kordhell - Memphis Doom 2:34
    2. DEATHPHONK - SAMSON! 3:09
    ...
    Ketik all / 1,3,5 / 1-10 / cancel"
5. Store in pendingPlaylists[chatJID] (expires 10 min)
6. User replies → processPlaylistDownload()
7. Download each track via DownloadSpotify():
   ├─ Status message edited per track:
   │   "📥 Lagu 3/10: SAMSON! 🔄 Mengunduh..."
   ├─ Send cover + audio per track
   ├─ 3s delay between tracks
   └─ Countdown shown in status message
8. Final: "✅ Playlist selesai! 10/10 lagu dikirim"
```

### Track Selection Parser

`parseTrackSelection(input, maxTracks)` supports:
- Single: `"3"` → [2] (0-indexed)
- Comma: `"1,3,5"` → [0, 2, 4]
- Range: `"1-5"` → [0, 1, 2, 3, 4]
- Mixed: `"1,5-8,15"` → [0, 4, 5, 6, 7, 14]
- Deduplication: seen map prevents duplicates

---

## Progress Tracking System

### progressTracker (Download Phase)
- Sends initial text message, stores message ID
- Goroutine updates every 2s via WhatsApp message edit
- Stages: "Menghubungi server" → "Mengunduh media" → "Mengirim ke WhatsApp"
- Shows elapsed time
- `finish()` stops ticker, shows total time

### Status Message (Multi-file Send Phase)
- Created after 1st file is sent (for multi-file results)
- Edited with countdown between files: "Menunggu 3s... 2s... 1s..."
- Shows current file being uploaded: "Mengirim file 2/3 (1.2MB)..."
- Final: "✅ Selesai! 3/3 media dikirim"

### Message Editing
Uses WhatsApp ProtocolMessage with `MESSAGE_EDIT` type:
```go
editMsg := &waProto.Message{
    ProtocolMessage: &waProto.ProtocolMessage{
        Key: &waProto.MessageKey{
            RemoteJID: chatJID,
            FromMe:    true,
            ID:        originalMessageID,
        },
        Type:          MESSAGE_EDIT,
        EditedMessage: &waProto.Message{Conversation: newText},
    },
}
```

---

## WhatsApp Bot Lifecycle (`internal/bot/bot.go`)

### Startup Flow
```
1. Validate API_KEY
2. Create SQLite store (WAL mode, foreign keys enabled)
3. Initialize whatsmeow client
4. Setup Telegram callbacks
5. Start Telegram polling (if configured)
6. Check session:
   ├─ No session + Telegram → prompt via Telegram (/pair)
    ├─ No session + no Telegram → console pairing (stdin, local only)
   └─ Session exists → auto-reconnect
7. Wait for Ctrl+C (SIGINT/SIGTERM)
8. Graceful shutdown
```

### Authentication
- **Phone Pairing Code** (not QR): `client.PairPhone(phone, true, PairClientChrome, "Chrome (Linux)")`
- Returns 8-digit code user enters in WhatsApp mobile app
- Session persisted in SQLite (`store/wa_session.db`)

### Event Handling
| Event | Action |
|-------|--------|
| `events.Message` | Route to `handler.HandleMessage()` (goroutine) |
| `events.Connected` | Set connected=true, notify Telegram |
| `events.PairSuccess` | Log device ID, notify Telegram |
| `events.Disconnected` | Set connected=false, notify Telegram |
| `events.LoggedOut` | Delete session DB, notify Telegram, stay running |

> **Key:** LoggedOut does NOT `os.Exit()` — allows Telegram re-pairing remotely.

---

## Telegram Remote Control (`internal/telegram/telegram.go`)

### Architecture
- Pure HTTP implementation (no external Telegram library)
- Long-polling via `getUpdates` with 25s timeout
- Admin-only: checks `msg.From.ID == b.AdminID`
- Callback-based: WA bot registers functions for each command

### Commands

| Command | Callback | Action |
|---------|----------|--------|
| `/status` | `OnStatus()` | Returns connection state + session info |
| `/reconnect` | `OnReconnect()` | Disconnects then reconnects WA client |
| `/pair <phone>` | `OnPair(phone)` | Removes old session, creates new pairing code |
| `/logout` | `OnLogout()` | Calls `client.Logout()`, cleans up session |
| `/help` | — | Shows command list |

### Auto Notifications
- ✅ Connected → `NotifyConnected()`
- ⚠️ Disconnected → `NotifyDisconnected()`
- 🔒 Logged Out → `NotifyLoggedOut()`
- 🚨 Error → `NotifyError(msg)`
- 👋 Shutdown → sent before exit

---

## Anti-Ban Mechanisms

| Mechanism | Value | Purpose |
|-----------|-------|---------|
| `sendDelay` | 3 seconds | Delay between sending multiple media items |
| Countdown display | 1s intervals | Shows user the delay is intentional |
| Sequential sending | One-by-one | Never sends multiple media simultaneously |
| File size check | Max 64MB | WhatsApp's upload limit |

---

## Media Sending Methods

| Type | Method | WhatsApp Type |
|------|--------|---------------|
| Video | `sendVideo()` | `VideoMessage` (video/mp4) |
| Image | `sendImage()` | `ImageMessage` (image/jpeg) |
| Audio | `sendAudio()` | `AudioMessage` (audio/mpeg) |
| Document | `sendDocument()` | `DocumentMessage` (fallback) |

All methods: Upload to WhatsApp servers first → get `DirectPath`, `MediaKey`, etc. → send message with encrypted reference.

---

## Docker Deployment

### Build
```bash
docker compose up -d --build
```

### Architecture
- **Stage 1 (builder):** `golang:1.25.0-alpine` → compile binary with `CGO_ENABLED=0`
- **Stage 2 (runtime):** `alpine:3.20` → only `ca-certificates` needed (for HTTPS)
- **Non-root user:** `appuser:appgroup`
- **Volumes:** `wa_session` (persist session), `wa_downloads` (temp files)
- **No Python/ffmpeg/yt-dlp needed** — all via REST API

### docker-compose.yml Notes
- `env_file: .env` → load bot config directly from the VPS `.env`
- `stdin_open: true` + `tty: true` should be off for headless VPS deploy; use Telegram `/pair`
- `restart: unless-stopped` → auto-restart on crash
- Session volume (`wa_session`) keeps login alive after container restart

---

## WhatsApp Bot Commands

| Command | Description |
|---------|-------------|
| `/help` | Menampilkan daftar platform & contoh link |
| `/ping` | Cek apakah bot aktif |
| `<social media link>` | Auto-detect & download media |
| `tomp3(link tiktok)` | Download audio TikTok saja |
| `menfess <no-wa> <pesan>` | Kirim pesan anonim ke nomor tujuan |
| `balas (pesan)` | Balas menfess aktif tanpa reply quote |
| `s / sticker` | Aktifkan mode sticker untuk foto berikutnya |
| `brat / .brat / brat` | Bikin sticker teks |
| `all` | Download semua lagu dari playlist (saat ada pending playlist) |
| `1,3,5` | Download lagu tertentu dari playlist |
| `1-10` | Download range lagu dari playlist |
| `cancel` | Batalkan download playlist |

---

## Key Design Decisions

1. **Pure-Go SQLite** (`glebarez/go-sqlite`) — menghindari kebutuhan CGO/GCC di Windows
2. **Phone Pairing Code** bukan QR — lebih reliable di terminal, bisa via Telegram
3. **REST API** bukan `yt-dlp` — lebih ringan, no Python dependency, Docker image lebih kecil
4. **Message editing** — progress tracking tanpa spam chat
5. **Telegram integration** — remote control tanpa SSH ke VPS
6. **Callback pattern** — Telegram bot tidak import WA bot langsung, pakai function callbacks
7. **Playlist state management** — per-chat pending playlists dengan 10-minute timeout
8. **Goroutine per message** — `go handler.HandleMessage(v)` untuk non-blocking processing
