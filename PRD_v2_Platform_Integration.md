# 🌀 Smara CLI v2.0 — PRD: Multi-Platform Integration

> **Smara** (Sanskerta: स्मृति) — "Ingatan" | Autonomous Multi-Agent Terminal
> **Versi PRD**: 2.0.0 | **Tanggal**: 2026-04-20 | **Status**: Draft

---

## 📋 I. Executive Summary

PRD ini mendefinisikan arsitektur dan roadmap untuk mengintegrasikan Smara CLI sebagai **AI Agent bot** di berbagai platform messaging: **WhatsApp, Telegram, Discord, Slack, LINE, Matrix, IRC, dan Microsoft Teams**. Integrasi ini memungkinkan user berinteraksi dengan agen AI Smara dari platform manapun, dengan kemampuan penuh termasuk MCP tool calling, memori kolektif, dan multi-agent orchestration.

### Visi
> _"Akses kekuatan penuh Smara dari chat platform manapun — tanpa buka terminal."_

### Prinsip Desain
1. **Adapter Pattern** — Setiap platform adalah adapter yang pluggable
2. **Core Agnostic** — Logic AI tetap di Smara Core, adapter hanya handle I/O
3. **Unified Experience** — User experience konsisten di semua platform
4. **Security First** — Sandboxing, rate limiting, authentication per platform
5. **Local First** — Data tetap di mesin user, adapter hanya jembatan

---

## 📊 II. Platform Matrix

| Platform | Protocol | Go Library | Auth Method | Media Support | Priority | Status |
|----------|----------|------------|-------------|---------------|----------|--------|
| **Telegram** | Bot API HTTP | `go-telegram-bot-api/v5` | BotFather Token | Text, Image, File, Voice | **P0** | Planned |
| **Discord** | WebSocket Gateway | `bwmarrin/discordgo` | Bot Token + OAuth2 | Text, Embed, File, Reaction | **P0** | Planned |
| **WhatsApp** | WhatsApp Web Protocol | `tulir/whatsmeow` | QR Code Pairing | Text, Image, File, Voice | **P1** | Planned |
| **Slack** | Events API + WebSocket | `slack-go/slack` | Bot Token + App | Text, Block Kit, File | **P1** | Planned |
| **LINE** | Messaging API | `line/line-bot-sdk-go` | Channel Token | Text, Flex Message, Image | **P2** | Planned |
| **Matrix** | Client-Server API | `mautrix/go` | Access Token | Text, Image, E2EE | **P2** | Planned |
| **IRC** | IRC Protocol | `lrstanley/girc` | SASL/NickServ | Text only | **P3** | Planned |
| **MS Teams** | Bot Framework | `microsoft/botframework-sdk` | Azure AD | Text, Adaptive Card | **P3** | Planned |

---

## 👤 III. User Stories

### P0 — Must Have
- **[US-01]** Sebagai developer, saya ingin mengirim prompt ke Smara via Telegram bot agar bisa mendapat jawaban AI tanpa membuka terminal.
- **[US-02]** Sebagai tim, kami ingin Smara bot di Discord server kami agar semua anggota bisa mengakses agen AI yang sama dengan memori bersama.
- **[US-03]** Sebagai user, saya ingin melihat output MCP tools (misalnya screenshot Blender) langsung di chat platform.
- **[US-04]** Sebagai admin, saya ingin mengatur siapa saja yang boleh menggunakan bot (whitelist/blacklist).

### P1 — Should Have
- **[US-05]** Sebagai user mobile, saya ingin berinteraksi dengan Smara via WhatsApp karena itu chat app utama saya.
- **[US-06]** Sebagai DevOps engineer, saya ingin integrasi Slack agar Smara bisa mengirim notifikasi dan menerima perintah dari channel DevOps.
- **[US-07]** Sebagai user, saya ingin berpindah antar platform dan memiliki history percakapan yang tersinkronisasi.
- **[US-08]** Sebagai admin, saya ingin rate limiting per user untuk mencegah penyalahgunaan.

### P2 — Nice to Have
- **[US-09]** Sebagai komunitas, kami ingin self-host Smara bot di Matrix server privat kami dengan E2E encryption.
- **[US-10]** Sebagai user LINE, saya ingin mendapat response dalam format Flex Message yang rich dan interaktif.

### P3 — Future
- **[US-11]** Sebagai developer retro, saya ingin mengakses Smara dari IRC channel.
- **[US-12]** Sebagai enterprise, kami ingin integrasi Microsoft Teams dengan Azure AD SSO.

---

## 🏗️ IV. Arsitektur Sistem

### 4.1 High-Level Architecture

```
┌─────────────────────────────────────────────────────┐
│                  Platform Layer                      │
│  ┌──────┐ ┌───────┐ ┌────┐ ┌─────┐ ┌────┐ ┌──────┐│
│  │  TG  │ │  DC   │ │ WA │ │Slack│ │LINE│ │Matrix││
│  └──┬───┘ └──┬────┘ └─┬──┘ └──┬──┘ └─┬──┘ └──┬───┘│
│     │        │        │       │      │       │     │
│     └────────┴────────┴───┬───┴──────┴───────┘     │
│                           │                         │
│                    ┌──────▼──────┐                   │
│                    │   Gateway   │                   │
│                    │   Router    │                   │
│                    └──────┬──────┘                   │
├───────────────────────────┼─────────────────────────┤
│                    Smara Core                        │
│  ┌────────┐  ┌────┬──────▼──────┐  ┌─────────────┐ │
│  │  Auth  │  │Rate│ Supervisor  │  │   Session    │ │
│  │Manager │  │Lim.│   Agent     │  │   Manager    │ │
│  └────────┘  └────┴──────┬──────┘  └─────────────┘ │
│                    ┌─────┼─────┐                     │
│              ┌─────▼┐ ┌──▼──┐ ┌▼─────┐              │
│              │ MCP  │ │ LLM │ │Memory│              │
│              │Client│ │Prov.│ │Store │              │
│              └──────┘ └─────┘ └──────┘              │
└─────────────────────────────────────────────────────┘
```

### 4.2 Package Structure

```
internal/
├── platform/               # NEW — Platform integration layer
│   ├── adapter.go           # PlatformAdapter interface
│   ├── gateway.go           # Gateway Router (message dispatch)
│   ├── types.go             # Shared types (IncomingMessage, OutgoingMessage)
│   ├── auth.go              # Auth & permission manager
│   ├── ratelimit.go         # Rate limiter per user/channel
│   ├── telegram/
│   │   └── telegram.go      # Telegram Bot API adapter
│   ├── discord/
│   │   └── discord.go       # Discord adapter
│   ├── whatsapp/
│   │   └── whatsapp.go      # WhatsApp adapter via whatsmeow
│   ├── slack/
│   │   └── slack.go         # Slack adapter
│   ├── line/
│   │   └── line.go          # LINE adapter
│   └── matrix/
│       └── matrix.go        # Matrix adapter
```

### 4.3 Core Interfaces

```go
// PlatformAdapter — setiap platform mengimplementasikan ini
type PlatformAdapter interface {
    Name() string
    Connect(ctx context.Context, cfg AdapterConfig) error
    Listen(ctx context.Context, handler MessageHandler) error
    SendMessage(ctx context.Context, channelID string, msg OutgoingMessage) error
    SendTypingIndicator(ctx context.Context, channelID string) error
    Close() error
}

// MessageHandler — callback ketika pesan masuk
type MessageHandler func(ctx context.Context, msg IncomingMessage) error

// IncomingMessage — pesan masuk dari platform apapun
type IncomingMessage struct {
    ID          string
    Platform    string            // "telegram", "discord", "whatsapp"
    ChannelID   string            // chat/channel/group ID
    UserID      string            // platform-specific user ID
    Username    string            // display name
    Content     string            // text content
    Attachments []Attachment      // files, images, voice
    ReplyTo     string            // reply-to message ID
    Metadata    map[string]string // platform-specific metadata
    Timestamp   time.Time
}

// OutgoingMessage — pesan keluar ke platform
type OutgoingMessage struct {
    Content     string
    Format      MessageFormat     // Plain, Markdown, HTML
    Attachments []Attachment
    ReplyTo     string
    Embeds      []Embed           // untuk Discord/Slack
}

// Attachment — file/media attachment
type Attachment struct {
    Type     string // "image", "file", "voice", "video"
    URL      string
    FilePath string
    MimeType string
    Size     int64
}

// AdapterConfig — konfigurasi per-platform
type AdapterConfig struct {
    Token       string
    WebhookURL  string
    AllowedUsers []string          // whitelist user IDs
    BlockedUsers []string          // blacklist user IDs
    RateLimit   RateLimitConfig
    Extra       map[string]string  // platform-specific settings
}
```

### 4.4 Gateway Router

```go
// Gateway — routes messages between platforms and Smara core
type Gateway struct {
    adapters    map[string]PlatformAdapter
    supervisor  *agent.Supervisor
    sessions    map[string]*PlatformSession // channelID → session
    auth        *AuthManager
    rateLimiter *RateLimiter
    mu          sync.RWMutex
}

func (g *Gateway) HandleIncoming(ctx context.Context, msg IncomingMessage) error {
    // 1. Auth check
    if !g.auth.IsAllowed(msg.Platform, msg.UserID) {
        return g.sendReply(msg, "⛔ Akses ditolak.")
    }

    // 2. Rate limit check
    if !g.rateLimiter.Allow(msg.UserID) {
        return g.sendReply(msg, "⏳ Rate limit tercapai. Coba lagi nanti.")
    }

    // 3. Get or create session for this channel
    session := g.getOrCreateSession(msg.ChannelID, msg.Platform)

    // 4. Send typing indicator
    g.adapters[msg.Platform].SendTypingIndicator(ctx, msg.ChannelID)

    // 5. Process via Supervisor (existing Smara core)
    response, err := g.supervisor.ProcessPrompt(ctx, msg.Content)
    if err != nil {
        return g.sendReply(msg, "❌ Error: " + err.Error())
    }

    // 6. Send response back
    return g.sendReply(msg, response)
}
```

---

## 🔄 V. Message Flow

### 5.1 Basic Prompt Flow

```
User (Telegram)              Smara
     │                         │
     │── "/ask Explain Go" ──►│
     │                         ├─ Auth Check ✓
     │                         ├─ Rate Limit ✓
     │◄── typing... ──────────┤
     │                         ├─ ProcessPrompt()
     │                         │   ├─ Memory Search
     │                         │   ├─ LLM Chat
     │                         │   └─ Save Memory
     │◄── "Go adalah..." ─────┤
     │                         │
```

### 5.2 MCP Tool Call Flow (Discord)

```
User (Discord)      Gateway       Supervisor      MCP Blender
     │                │              │                │
     │─ "Buat kubus" ►│              │                │
     │                │─ Process ───►│                │
     │                │              │─ tool_call ───►│
     │                │              │◄─ cube.obj ────│
     │◄─ Embed+File ──│◄─ result ────│                │
     │                │              │                │
```

---

## ⚙️ VI. Konfigurasi

### 6.1 Config File: `~/.smara/platforms.yaml`

```yaml
# Platform Bot Configuration
platforms:
  telegram:
    enabled: true
    token: "${SMARA_TELEGRAM_TOKEN}"
    allowed_users:
      - "123456789"
      - "987654321"
    rate_limit:
      requests_per_minute: 20
      burst: 5
    settings:
      parse_mode: "MarkdownV2"
      disable_web_page_preview: true

  discord:
    enabled: true
    token: "${SMARA_DISCORD_TOKEN}"
    guild_ids:
      - "1234567890"
    allowed_roles:
      - "smara-user"
      - "admin"
    rate_limit:
      requests_per_minute: 30
      burst: 10
    settings:
      command_prefix: "!"
      use_slash_commands: true

  whatsapp:
    enabled: false
    phone_number: "+62812xxxxx"
    session_dir: "~/.smara/wa-session"
    allowed_numbers:
      - "+62812xxxxx"
    rate_limit:
      requests_per_minute: 10
      burst: 3

  slack:
    enabled: false
    bot_token: "${SMARA_SLACK_BOT_TOKEN}"
    app_token: "${SMARA_SLACK_APP_TOKEN}"
    channels:
      - "#ai-assistant"
    rate_limit:
      requests_per_minute: 20
      burst: 5

  line:
    enabled: false
    channel_secret: "${SMARA_LINE_CHANNEL_SECRET}"
    channel_token: "${SMARA_LINE_CHANNEL_TOKEN}"

  matrix:
    enabled: false
    homeserver: "https://matrix.org"
    user_id: "@smara:matrix.org"
    access_token: "${SMARA_MATRIX_TOKEN}"
    rooms:
      - "!roomid:matrix.org"
    encryption: true

# Global Settings
global:
  max_response_length: 4000    # chars, auto-split jika lebih
  typing_indicator: true
  error_notify_admin: true
  admin_user_id: "admin_id"
  log_conversations: true
  log_dir: "~/.smara/platform-logs"
```

### 6.2 Environment Variables

```bash
# Platform Tokens (jangan simpan di config file!)
export SMARA_TELEGRAM_TOKEN="bot123:AAH..."
export SMARA_DISCORD_TOKEN="MTIz..."
export SMARA_SLACK_BOT_TOKEN="xoxb-..."
export SMARA_SLACK_APP_TOKEN="xapp-..."
export SMARA_LINE_CHANNEL_SECRET="..."
export SMARA_LINE_CHANNEL_TOKEN="..."
export SMARA_MATRIX_TOKEN="..."
```

---

## 🔐 VII. Security & Permissions

### 7.1 Authentication Model

```
┌─────────────┐     ┌──────────────┐     ┌────────────┐
│  Platform    │────►│  Auth        │────►│  Permission │
│  User ID     │     │  Manager     │     │  Check      │
└─────────────┘     └──────────────┘     └────────────┘
                          │
                    ┌─────▼─────┐
                    │ Whitelist  │
                    │ Blacklist  │
                    │ Role-based │
                    └───────────┘
```

### 7.2 Security Measures

| Layer | Measure | Detail |
|-------|---------|--------|
| **Transport** | TLS/HTTPS | Semua koneksi platform via TLS |
| **Auth** | Token-based | Per-platform token dari env vars |
| **Access** | Whitelist/Blacklist | Per user, per channel |
| **Rate Limit** | Token bucket | Per user, configurable |
| **Sandbox** | Context timeout | Max 2 menit per request |
| **Data** | No plaintext secrets | Token hanya dari env vars |
| **Logging** | Audit trail | Log semua interaksi (opsional) |
| **Isolation** | Session per channel | Tiap channel punya session terpisah |

### 7.3 Rate Limiting

```go
type RateLimitConfig struct {
    RequestsPerMinute int  `yaml:"requests_per_minute"`
    BurstSize         int  `yaml:"burst"`
    CooldownSeconds   int  `yaml:"cooldown_seconds"`
}
```

Implementasi menggunakan `golang.org/x/time/rate` (token bucket algorithm).

---

## 📱 VIII. Platform-Specific Details

### 8.1 Telegram

**Commands:**
| Command | Description |
|---------|-------------|
| `/start` | Memulai sesi baru |
| `/ask <prompt>` | Kirim prompt ke Smara |
| `/mode <ask\|rush\|plan>` | Ganti mode agent |
| `/model <provider> [model]` | Ganti model LLM |
| `/mcp` | Lihat daftar MCP tools |
| `/memory` | Lihat memori tersimpan |
| `/session list` | Lihat daftar sesi |
| `/clear` | Reset conversation |
| `/help` | Bantuan |

**Fitur khusus:** Inline keyboard untuk mode switch, callback queries.

### 8.2 Discord

**Slash Commands:**
- `/smara ask <prompt>` — Kirim prompt
- `/smara mode <mode>` — Switch mode
- `/smara mcp` — List MCP servers
- `/smara session` — Session management

**Fitur khusus:** Embed untuk output terstruktur, thread support, reaction-based feedback.

### 8.3 WhatsApp

**Trigger:** Semua pesan ke nomor bot dianggap prompt (kecuali prefix `/`).

**Fitur khusus:** QR code pairing, voice message transcription (future), image analysis (future).

### 8.4 Slack

**Trigger:** Mention `@smara` atau pesan di channel dedicated.

**Fitur khusus:** Block Kit formatting, modal dialogs, home tab.

---

## 📈 IX. Metrik Keberhasilan

| Metrik | Target | Measurement |
|--------|--------|-------------|
| **Response Latency** | < 5 detik (text) | P95 response time |
| **Uptime** | > 99.5% | Bot availability |
| **Platform Coverage** | 4+ platforms (Phase 2) | Active adapters |
| **Concurrent Users** | 50+ simultaneous | Load testing |
| **Error Rate** | < 2% | Failed responses / total |
| **Memory Sync** | < 30 detik cross-platform | Sync latency |

---

## 🚀 X. Roadmap

### Phase 1 — Foundation (v2.0)
- [x] Core adapter interface design
- [ ] Gateway router implementation
- [ ] Telegram adapter (P0)
- [ ] Discord adapter (P0)
- [ ] CLI command: `smara serve` (starts platform bots)
- [ ] Basic auth & rate limiting

### Phase 2 — Expansion (v2.1)
- [ ] WhatsApp adapter (P1)
- [ ] Slack adapter (P1)
- [ ] Cross-platform session sync
- [ ] Admin dashboard CLI (`smara admin`)
- [ ] Webhook support for serverless deployment

### Phase 3 — Richness (v2.2)
- [ ] LINE adapter (P2)
- [ ] Matrix adapter (P2)
- [ ] Rich media responses (images, files from MCP)
- [ ] Voice message support (STT/TTS)
- [ ] Scheduled messages & reminders

### Phase 4 — Enterprise (v2.3)
- [ ] IRC adapter (P3)
- [ ] Microsoft Teams adapter (P3)
- [ ] Multi-tenant deployment
- [ ] Analytics & usage dashboard
- [ ] Plugin system for custom adapters

---

## 🛠️ XI. Technical Dependencies

### New Go Dependencies
```go
// go.mod additions
require (
    github.com/go-telegram-bot-api/telegram-bot-api/v5 v5.5.1
    github.com/bwmarrin/discordgo v0.28.1
    go.mau.fi/whatsmeow v0.0.0-20250101000000
    github.com/slack-go/slack v0.12.3
    github.com/line/line-bot-sdk-go/v8 v8.3.0
    maunium.net/go/mautrix v0.18.0
    golang.org/x/time v0.5.0  // rate limiting
)
```

### CLI Command Addition
```
smara serve [--platform telegram,discord] [--config platforms.yaml]
```

---

## 🧪 XII. Deployment Topologies

### Self-Hosted (Recommended)
```
┌─────────────────────┐
│   User's Machine    │
│  ┌───────────────┐  │
│  │  smara serve  │  │
│  │  (all-in-one) │  │
│  └───────────────┘  │
└─────────────────────┘
```

### Docker
```dockerfile
FROM golang:1.26-alpine AS builder
RUN go build -o smara ./cmd/smara/
FROM alpine:latest
COPY --from=builder /app/smara /usr/local/bin/
CMD ["smara", "serve", "--platform", "telegram,discord"]
```

### VPS / Systemd
```ini
[Unit]
Description=Smara AI Bot
After=network.target

[Service]
ExecStart=/usr/local/bin/smara serve
EnvironmentFile=/etc/smara/env
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
```

---

**Status**: Ready for Implementation
**Version**: 2.0.0-draft
**Author**: Smara Team
