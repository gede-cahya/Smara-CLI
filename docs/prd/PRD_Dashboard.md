# 🌀 Smara CLI — PRD: Dashboard TUI

> **Smara** (Sanskerta: स्मृति) — "Ingatan" | Autonomous Multi-Agent Terminal
> **Versi PRD**: 1.0.0 | **Tanggal**: 2026-04-23 | **Status**: Draft

---

## I. Executive Summary

PRD ini mendefinisikan fitur **Dashboard** untuk Smara CLI — sebuah antarmuka TUI (Terminal User Interface) yang memberikan visibilitas real-time terhadap seluruh ekosistem Smara: status platform bot, penggunaan LLM, metrik agen, memori, dan sesi aktif. Dashboard ini diakses melalui perintah `smara dashboard` dan dibangun menggunakan stack TUI yang sudah ada (bubbletea + lipgloss).

### Visi
> _"Pantau, kelola, dan pahami seluruh aktivitas Smara dari satu layar terminal."_

### Prinsip Desain
1. **Glanceable** — Informasi kritis terlihat dalam 3 detik pertama
2. **Real-time** — Data diperbarui secara live tanpa perlu refresh manual
3. **Keyboard-first** — Navigasi penuh via keyboard, tanpa mouse
4. **Resource-light** — Overhead CPU/RAM minimal, cocok untuk VPS kecil
5. **Consistent** — Menggunakan design language yang sama dengan TUI `smara start`

---

## II. Konteks & Motivasi

### Masalah Saat Ini
1. **Blind spot operasional**: Saat `smara serve` berjalan, user tidak punya cara melihat apa yang terjadi — berapa pesan masuk, dari platform mana, berapa token terpakai, apakah ada error.
2. **Debugging sulit**: Jika bot tidak merespons, user harus membaca log mentah untuk mencari tahu penyebabnya.
3. **Tidak ada insight penggunaan**: User tidak bisa melihat berapa biaya LLM yang terpakai, siapa user paling aktif, atau berapa memori yang tersimpan.
4. **Manajemen terpisah**: Untuk melihat sesi, memori, atau status MCP, user harus menjalankan beberapa perintah CLI yang berbeda.

### Solusi
Satu dashboard terpusat yang menyatukan semua metrik dan kontrol ke dalam antarmuka TUI yang elegan dan real-time.

---

## III. User Stories

### P0 — Must Have
| ID | Story | Acceptance Criteria |
|----|-------|-------------------|
| **US-01** | Sebagai operator, saya ingin melihat status semua platform bot (online/offline) di satu layar | Dashboard menampilkan indikator status per platform dengan warna hijau/merah |
| **US-02** | Sebagai operator, saya ingin melihat jumlah pesan masuk/keluar per platform secara real-time | Counter live yang terupdate setiap ada pesan baru |
| **US-03** | Sebagai developer, saya ingin melihat total token dan estimasi biaya LLM | Panel statistik menampilkan input/output tokens dan cost estimation |
| **US-04** | Sebagai operator, saya ingin melihat error terbaru tanpa membaca log file | Panel error/log menampilkan 10 error terbaru dengan timestamp |
| **US-05** | Sebagai user, saya ingin melihat daftar sesi aktif dan statusnya | Tabel sesi dengan kolom: ID, platform, user, mode, last activity |
| **US-13** | Sebagai developer, saya ingin mengelompokkan pekerjaan ke dalam Workspace | Dukungan folder proyek (workspace) untuk isolasi memori dan context |
| **US-14** | Sebagai user baru, saya ingin dipandu melalui tutorial interaktif | Fitur `smara guide` yang menjelaskan navigasi dan fitur dasar |

### P1 — Should Have
| ID | Story | Acceptance Criteria |
|----|-------|-------------------|
| **US-06** | Sebagai operator, saya ingin melihat status koneksi MCP server | Panel MCP menampilkan nama server, status connected/disconnected, jumlah tools |
| **US-07** | Sebagai admin, saya ingin melihat top users berdasarkan jumlah request | Leaderboard 5 user teratas per platform |
| **US-08** | Sebagai developer, saya ingin melihat rata-rata response latency | Metrik latency (avg, P95) ditampilkan di panel statistik |
| **US-09** | Sebagai operator, saya ingin melihat jumlah memori tersimpan dan sync status | Counter memori + status sync daemon (last sync time, pending deltas) |

### P2 — Nice to Have
| ID | Story | Acceptance Criteria |
|----|-------|-------------------|
| **US-10** | Sebagai admin, saya ingin bisa meng-kick/ban user langsung dari dashboard | Action menu pada user list dengan opsi block |
| **US-11** | Sebagai developer, saya ingin melihat grafik request per menit (sparkline) | Sparkline chart di panel utama |
| **US-12** | Sebagai operator, saya ingin melihat uptime server | Timer uptime di header dashboard |

---

## IV. Arsitektur & Desain

### 4.1 Integrasi dengan Sistem yang Ada

```
┌──────────────────────────────────────────────────────────────┐
│                     smara dashboard                          │
│  ┌────────────────────────────────────────────────────────┐  │
│  │               Dashboard TUI (bubbletea)                │  │
│  │  ┌──────────┐ ┌──────────┐ ┌───────────┐ ┌─────────┐ │  │
│  │  │ Platform │ │   Stats  │ │  Sessions │ │  Logs   │ │  │
│  │  │  Status  │ │  Panel   │ │   Table   │ │  Panel  │ │  │
│  │  └────┬─────┘ └────┬─────┘ └─────┬─────┘ └────┬────┘ │  │
│  └───────┼────────────┼─────────────┼────────────┼───────┘  │
│          │            │             │            │           │
│  ┌───────▼────────────▼─────────────▼────────────▼───────┐  │
│  │              Dashboard Data Collector                  │  │
│  │         (polls from existing subsystems)               │  │
│  └──┬──────────┬───────────┬────────────┬────────────┬───┘  │
│     │          │           │            │            │       │
│  ┌──▼──┐  ┌───▼───┐  ┌───▼────┐  ┌────▼───┐  ┌────▼────┐  │
│  │Gate │  │Superv.│  │Session │  │Memory  │  │  Sync   │  │
│  │way  │  │Stats  │  │Store   │  │Store   │  │ Daemon  │  │
│  └─────┘  └───────┘  └────────┘  └────────┘  └─────────┘  │
└──────────────────────────────────────────────────────────────┘
```

### 4.2 Pendekatan Data Collection

Dashboard **tidak** menjalankan `smara serve` sendiri. Ia membaca data dari:

1. **SQLite Database** (`~/.smara/smara.db`) — sesi, memori, histori
2. **Unix Domain Socket / Shared Memory** — metrik real-time dari `smara serve` yang sedang berjalan
3. **File-based metrics** (`~/.smara/metrics.json`) — fallback jika socket tidak tersedia

Pendekatan yang dipilih: **File-based metrics** sebagai MVP, lalu upgrade ke Unix socket di iterasi berikutnya.

```go
// internal/metrics/collector.go

// Metrics holds real-time operational metrics.
type Metrics struct {
    StartedAt       time.Time              `json:"started_at"`
    Uptime          string                 `json:"uptime"`
    Platforms       map[string]PlatformMetrics `json:"platforms"`
    LLM             LLMMetrics             `json:"llm"`
    MCP             []MCPMetrics           `json:"mcp"`
    Memory          MemoryMetrics          `json:"memory"`
    Sync            SyncMetrics            `json:"sync"`
    RecentErrors    []ErrorEntry           `json:"recent_errors"`
    ActiveSessions  int                    `json:"active_sessions"`
}

// PlatformMetrics holds per-platform statistics.
type PlatformMetrics struct {
    Name            string `json:"name"`
    Status          string `json:"status"` // "online", "offline", "error"
    MessagesIn      int64  `json:"messages_in"`
    MessagesOut     int64  `json:"messages_out"`
    ActiveUsers     int    `json:"active_users"`
    ErrorCount      int    `json:"error_count"`
    TopUsers        []UserActivity `json:"top_users"`
    AvgLatencyMs    int64  `json:"avg_latency_ms"`
    P95LatencyMs    int64  `json:"p95_latency_ms"`
}

// LLMMetrics holds LLM usage statistics.
type LLMMetrics struct {
    Provider        string  `json:"provider"`
    Model           string  `json:"model"`
    TotalRequests   int64   `json:"total_requests"`
    InputTokens     int64   `json:"input_tokens"`
    OutputTokens    int64   `json:"output_tokens"`
    EstimatedCostUSD float64 `json:"estimated_cost_usd"`
    AvgLatencyMs    int64   `json:"avg_latency_ms"`
}

// MCPMetrics holds per-MCP server metrics.
type MCPMetrics struct {
    Name       string `json:"name"`
    Connected  bool   `json:"connected"`
    ToolCount  int    `json:"tool_count"`
    CallCount  int64  `json:"call_count"`
    ErrorCount int    `json:"error_count"`
}

// MemoryMetrics holds memory store statistics.
type MemoryMetrics struct {
    TotalMemories   int    `json:"total_memories"`
    UnsyncedCount   int    `json:"unsynced_count"`
    DBSizeBytes     int64  `json:"db_size_bytes"`
}

// SyncMetrics holds sync daemon status.
type SyncMetrics struct {
    Enabled         bool      `json:"enabled"`
    LastSyncAt      time.Time `json:"last_sync_at"`
    PendingDeltas   int       `json:"pending_deltas"`
    Status          string    `json:"status"` // "idle", "syncing", "error"
}

// ErrorEntry represents a recent error.
type ErrorEntry struct {
    Timestamp time.Time `json:"timestamp"`
    Source    string    `json:"source"` // "telegram", "llm", "mcp:blender"
    Message   string    `json:"message"`
}

// UserActivity tracks per-user stats.
type UserActivity struct {
    UserID   string `json:"user_id"`
    Username string `json:"username"`
    Platform string `json:"platform"`
    Requests int64  `json:"requests"`
}
```

### 4.3 Layout TUI

```
┌─ 🌀 Smara Dashboard ─────────────────── v1.8.3 ── ⏱ Uptime: 2h 34m ─┐
│                                                                        │
│  ┌─ Platform Status ─────────┐  ┌─ LLM Usage ─────────────────────┐   │
│  │ ● Telegram    online  142 │  │ Provider: Anthropic              │   │
│  │ ● Discord     online   87 │  │ Model:    claude-3.5-sonnet      │   │
│  │ ○ WhatsApp    offline   0 │  │ Requests: 229                    │   │
│  │                           │  │ Tokens:   45.2K in / 12.8K out   │   │
│  │ Total: 229 msgs today     │  │ Cost:     ~$0.34                 │   │
│  └───────────────────────────┘  │ Avg Latency: 1.2s               │   │
│                                 └──────────────────────────────────┘   │
│  ┌─ MCP Servers ─────────────────────────────────────────────────┐    │
│  │ ● blender      connected   12 tools   34 calls   0 errors    │    │
│  │ ● filesystem   connected    8 tools   156 calls  2 errors    │    │
│  │ ○ context7     disconnected                                   │    │
│  └───────────────────────────────────────────────────────────────┘    │
│                                                                        │
│  ┌─ Active Sessions ──────────────────────────────────────────────┐   │
│  │ ID       Platform   User         Mode   Last Activity         │   │
│  │ a1b2c3   telegram   @cahya       plan   2 min ago             │   │
│  │ d4e5f6   discord    DevTeam#gen  ask    5 min ago             │   │
│  │ g7h8i9   telegram   @rizki       rush   12 min ago            │   │
│  └───────────────────────────────────────────────────────────────┘    │
│                                                                        │
│  ┌─ Memory & Sync ──────────┐  ┌─ Recent Errors ──────────────────┐  │
│  │ Memories:  1,247          │  │ 14:32 [telegram] timeout after   │  │
│  │ Unsynced:  3              │  │       2s on prompt processing    │  │
│  │ DB Size:   4.2 MB         │  │ 14:15 [mcp:blender] connection  │  │
│  │ Last Sync: 3 min ago      │  │       refused on tool call      │  │
│  │ Status:    ● idle         │  │ 13:58 [llm] rate limit hit,     │  │
│  └───────────────────────────┘  │       retrying in 30s           │  │
│                                 └──────────────────────────────────┘  │
│                                                                        │
├────────────────────────────────────────────────────────────────────────┤
│ [q] Quit  [r] Refresh  [tab] Navigate  [/] Filter  [?] Help          │
└────────────────────────────────────────────────────────────────────────┘
```

### 4.4 Package Structure

```
internal/
├── metrics/                    # NEW — Metrics collection & export
│   ├── collector.go            # MetricsCollector — aggregates stats
│   ├── writer.go               # Writes metrics to ~/.smara/metrics.json
│   ├── reader.go               # Reads metrics (used by dashboard)
│   └── types.go                # Metrics, PlatformMetrics, LLMMetrics, etc.
│
├── dashboard/                  # NEW — Dashboard TUI
│   ├── dashboard.go            # Main bubbletea model & Update loop
│   ├── panels.go               # Individual panel components
│   ├── keybindings.go          # Keyboard navigation handlers
│   └── styles.go               # lipgloss styles & theme
│
cmd/smara/
├── dashboard.go                # NEW — `smara dashboard` command
```

---

## V. Spesifikasi Teknis

### 5.1 CLI Command

```bash
# Buka dashboard (mode default: real-time)
smara dashboard

# Buka dashboard dengan refresh interval tertentu
smara dashboard --refresh 5s

# Tampilkan snapshot sekali saja (non-interactive)
smara dashboard --once

# Filter platform tertentu
smara dashboard --platform telegram
```

### 5.2 Keyboard Shortcuts

| Key | Action |
|-----|--------|
| `q` / `Ctrl+C` | Keluar dari dashboard |
| `r` | Force refresh data |
| `Tab` / `Shift+Tab` | Navigasi antar panel |
| `j` / `k` | Scroll atas/bawah di panel aktif |
| `Enter` | Expand detail item yang dipilih |
| `/` | Buka filter/search |
| `1`-`5` | Jump ke panel tertentu |
| `?` | Tampilkan help overlay |
| `l` | Toggle tampilan log lengkap |
| `e` | Expand panel error |

### 5.3 Data Flow: Serve → Metrics → Dashboard

```
smara serve (proses A)                  smara dashboard (proses B)
┌──────────────────────┐                ┌──────────────────────┐
│  Gateway             │                │  Dashboard TUI       │
│  ├─ on message ──┐   │                │  ├─ ticker (2s) ─┐   │
│  │   metrics++   │   │                │  │   read file    │   │
│  │               ▼   │                │  │               ▼   │
│  │  MetricsCollector │  ──write──►    │  │  MetricsReader │   │
│  │       │           │  metrics.json  │  │       │        │   │
│  │       ▼           │                │  │       ▼        │   │
│  │  metrics.json     │                │  │  Update panels │   │
│  └──────────────────┘                │  └─────────────────┘  │
│                                       └──────────────────────┘
```

`smara serve` menulis metrik ke `~/.smara/metrics.json` setiap 2 detik.
`smara dashboard` membaca file tersebut dan memperbarui tampilan.

### 5.4 Metrics Writer (di serve)

```go
// MetricsCollector terintegrasi di Gateway.
// Setiap kali ada event (message in, message out, error, tool call),
// collector meng-update counter di memory.
// Goroutine background menulis snapshot ke file setiap 2 detik.

func (c *MetricsCollector) Start(ctx context.Context) {
    ticker := time.NewTicker(2 * time.Second)
    defer ticker.Stop()
    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            c.writeSnapshot()
        }
    }
}
```

### 5.5 Dashboard Bubbletea Model

```go
type DashboardModel struct {
    metrics     *metrics.Metrics    // latest metrics snapshot
    reader      *metrics.Reader     // reads from metrics.json
    activePanel int                 // currently focused panel
    panels      []Panel             // all panels
    width       int                 // terminal width
    height      int                 // terminal height
    err         error               // last error
    quitting    bool
}

type Panel interface {
    Title() string
    View(width, height int) string
    Update(metrics *metrics.Metrics)
    HandleKey(msg tea.KeyMsg) tea.Cmd
}
```

### 5.6 Fallback: Database-only Mode

Jika `smara serve` tidak berjalan (tidak ada `metrics.json`), dashboard tetap bisa menampilkan data statis dari SQLite:
- Daftar sesi tersimpan
- Jumlah memori
- Konfigurasi saat ini

Dashboard menampilkan banner: `⚠ Serve tidak aktif — menampilkan data tersimpan`

---

## VI. Data yang Ditampilkan per Panel

### Panel 1: Platform Status
| Data | Sumber | Update |
|------|--------|--------|
| Nama platform | Config | Static |
| Status (online/offline) | metrics.json | 2s |
| Jumlah pesan masuk | metrics.json | 2s |
| Jumlah pesan keluar | metrics.json | 2s |
| Active users count | metrics.json | 2s |
| Total pesan hari ini | metrics.json (aggregated) | 2s |

### Panel 2: LLM Usage
| Data | Sumber | Update |
|------|--------|--------|
| Provider & model name | Config / metrics.json | Static |
| Total requests | metrics.json | 2s |
| Input/output tokens | metrics.json | 2s |
| Estimated cost (USD) | Calculated | 2s |
| Average latency | metrics.json | 2s |

**Cost Calculation:**
```go
// Estimasi berdasarkan provider/model pricing
func EstimateCost(provider, model string, inputTokens, outputTokens int64) float64 {
    switch provider {
    case "anthropic":
        switch {
        case strings.Contains(model, "sonnet"):
            return float64(inputTokens)*3.0/1_000_000 + float64(outputTokens)*15.0/1_000_000
        case strings.Contains(model, "haiku"):
            return float64(inputTokens)*0.25/1_000_000 + float64(outputTokens)*1.25/1_000_000
        case strings.Contains(model, "opus"):
            return float64(inputTokens)*15.0/1_000_000 + float64(outputTokens)*75.0/1_000_000
        }
    case "openai":
        return float64(inputTokens)*5.0/1_000_000 + float64(outputTokens)*15.0/1_000_000
    }
    return 0 // ollama = free
}
```

### Panel 3: MCP Servers
| Data | Sumber | Update |
|------|--------|--------|
| Server name | Supervisor MCPInfo | 2s |
| Connection status | metrics.json | 2s |
| Tool count | metrics.json | 2s |
| Call count | metrics.json | 2s |
| Error count | metrics.json | 2s |

### Panel 4: Active Sessions
| Data | Sumber | Update |
|------|--------|--------|
| Session ID (truncated) | SQLite sessions table | 5s |
| Platform origin | SQLite / metrics.json | 5s |
| Username | metrics.json | 5s |
| Agent mode | SQLite sessions table | 5s |
| Last activity (relative) | Computed | 5s |

### Panel 5: Memory & Sync
| Data | Sumber | Update |
|------|--------|--------|
| Total memories | SQLite COUNT | 10s |
| Unsynced count | SQLite query | 10s |
| DB file size | os.Stat | 10s |
| Last sync time | metrics.json | 5s |
| Sync status | metrics.json | 5s |

### Panel 6: Recent Errors
| Data | Sumber | Update |
|------|--------|--------|
| Timestamp | metrics.json | 2s |
| Source (platform/mcp/llm) | metrics.json | 2s |
| Error message (truncated) | metrics.json | 2s |
| Max 10 entries | Ring buffer | — |

---

## VII. Modifikasi pada Komponen yang Ada

### 7.1 Gateway (internal/platform/gateway.go)
- **Tambah**: field `metrics *metrics.MetricsCollector`
- **Tambah**: hook `OnMessageIn()`, `OnMessageOut()`, `OnError()` yang meng-update metrics
- **Dampak**: Minimal — hanya menambahkan counter increment di handler yang sudah ada

### 7.2 Supervisor (internal/agent/supervisor.go)
- **Tambah**: Expose `Stats` yang sudah ada ke metrics collector
- **Tambah**: Hook `OnToolCall()` counter untuk MCP metrics
- **Dampak**: Minimal — Stats struct sudah ada, hanya perlu expose

### 7.3 Config (internal/config/config.go)
- **Tambah**: `DashboardConfig` ke `SmaraConfig`
```go
type DashboardConfig struct {
    RefreshInterval string `mapstructure:"refresh_interval" yaml:"refresh_interval"` // default "2s"
    MetricsFile     string `mapstructure:"metrics_file" yaml:"metrics_file"`         // default "~/.smara/metrics.json"
}
```

### 7.4 Cmd (cmd/smara/)
- **Tambah**: `dashboard.go` — perintah `smara dashboard`

---

## VIII. Metrik Keberhasilan

| Metrik | Target | Pengukuran |
|--------|--------|------------|
| **Startup time** | < 200ms | Waktu dari `smara dashboard` hingga layar pertama muncul |
| **Refresh rate** | 2s default, configurable | Interval update panel |
| **CPU usage** | < 2% idle | Dashboard tidak boleh membebani sistem |
| **Memory usage** | < 20MB RSS | Termasuk bubbletea rendering |
| **Metrics file size** | < 50KB | Snapshot JSON tidak membengkak |
| **Data freshness** | < 3 detik | Dari event terjadi hingga tampil di dashboard |

---

## IX. Dependensi

### Dependensi Baru
Tidak ada dependensi Go baru yang diperlukan. Stack yang sudah ada sudah mencukupi:

| Kebutuhan | Library yang Sudah Ada |
|-----------|----------------------|
| TUI framework | `charmbracelet/bubbletea` v1.3.10 |
| Styling | `charmbracelet/lipgloss` v1.1.0 |
| Input components | `charmbracelet/bubbles` v1.0.0 |
| JSON serialization | `encoding/json` (stdlib) |
| File watching | `fsnotify/fsnotify` v1.9.0 |
| Database access | `modernc.org/sqlite` v1.49.1 |

---

## X. Roadmap Implementasi

### Phase 1 — Foundation (v1.9.0)
- [ ] `internal/metrics/` — types, collector, writer, reader
- [ ] Integrasi MetricsCollector ke Gateway (`smara serve`)
- [ ] `internal/dashboard/` — model dasar, panel platform status, panel LLM
- [ ] `cmd/smara/dashboard.go` — CLI command
- [ ] Mode `--once` untuk snapshot non-interaktif

### Phase 2 — Full Panels (v1.9.1)
- [ ] Panel MCP servers
- [ ] Panel active sessions (dari SQLite)
- [ ] Panel memory & sync status
- [ ] Panel recent errors
- [ ] Keyboard navigation antar panel

### Phase 3 — Polish & Interactivity (v1.10.0)
- [ ] Detail view saat Enter pada item (expand session, expand error)
- [ ] Filter `/` untuk search di panel
- [ ] Sparkline chart untuk request rate (P2)
- [ ] Top users leaderboard
- [ ] Database-only fallback mode
- [ ] **Workspace Management**: Implementasi perintah `smara workspace` untuk isolasi proyek
- [ ] **Interactive Walkthrough**: Implementasi perintah `smara guide` untuk onboarding tutorial

### Phase 4 — Advanced (v2.x)
- [ ] Unix Domain Socket untuk real-time streaming (ganti file polling)
- [ ] Action panel: restart platform, clear session, block user
- [ ] Export metrik ke Prometheus/Grafana (opsional)
- [ ] Dashboard web view via `smara dashboard --web` (opsional, jauh)

---

## XI. Risiko & Mitigasi

| Risiko | Dampak | Mitigasi |
|--------|--------|----------|
| File lock conflict saat serve & dashboard baca/tulis metrics.json bersamaan | Data corruption | Gunakan atomic write (write temp → rename) |
| Dashboard mengonsumsi terlalu banyak CPU karena re-render | Sistem lambat | Hanya re-render panel yang datanya berubah (diff-based) |
| metrics.json terus membesar | Disk penuh | Fixed-size ring buffer untuk errors, truncate saat write |
| smara serve crash tanpa membersihkan metrics.json | Dashboard tampilkan data stale | Cek timestamp `updated_at` — jika > 10s, tampilkan warning |
| Terminal terlalu kecil untuk layout | Layout rusak | Responsive layout: collapse ke single-column jika width < 80 |

---

## XII. Contoh Penggunaan

### Workflow Khas
```bash
# Terminal 1: Jalankan bot server
smara serve --platform telegram,discord

# Terminal 2: Monitor di dashboard
smara dashboard

# Atau, quick check tanpa interactive mode
smara dashboard --once
```

### Output Non-Interactive (`--once`)
```
🌀 Smara Dashboard — Snapshot at 2026-04-23 14:32:05

Platforms:
  ● Telegram   online   142 msgs   3 active users
  ● Discord    online    87 msgs   8 active users
  ○ WhatsApp   offline

LLM: Anthropic claude-3.5-sonnet | 229 requests | 58K tokens | ~$0.34

MCP: blender (12 tools, 34 calls) | filesystem (8 tools, 156 calls)

Sessions: 3 active | Memory: 1,247 entries (4.2 MB) | Sync: idle

Recent Errors (2):
  14:32 [telegram] timeout after 2s
  14:15 [mcp:blender] connection refused
```

---

**Status**: Ready for Review
**Version**: 1.0.0-draft
**Author**: Smara Team
