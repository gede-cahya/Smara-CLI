# Smara CLI — PRD: Skill Ecosystem v2

**Version**: 2.0.0 | **Date**: 2026-05-15 | **Status**: Draft

---

## I. Executive Summary

PRD ini mendefinisikan evolusi sistem Skill Smara menjadi ekosistem lengkap: skill dapat dijalankan oleh workflow agents, di-install dari remote URL, di-generate otomatis dari eksekusi workflow sukses, dan didistribusikan melalui marketplace/registry.

**Visi**: Setiap urutan tool call yang berhasil adalah calon skill. Setiap skill adalah aset yang bisa di-share, di-install, dan dijalankan.

**Prinsip Desain**:
1. **Composable** — Skill adalah unit eksekusi yang dapat dipanggil oleh workflow agents
2. **Portable** — Skill JSON flat-file bisa di-install dari URL/GitHub/marketplace
3. **Auto-Discoverable** — Workflow run sukses ditawarkan untuk dijadikan skill
4. **Community-Ready** — Marketplace dengan versioning, tagging, dan search

---

## II. Konteks & Motivasi

**Masalah Saat Ini**:
1. Skill terpisah dari workflow — agents tidak bisa memanfaatkan skill tersimpan
2. Tidak ada cara share skill — hanya tersimpan lokal di `~/.smara/skills/`
3. Repetisi manual — workflow sukses harus di-rekonstruksi manual menjadi skill
4. Tidak ada discovery — tidak bisa mencari skill berdasarkan tag/domain

**Solusi**: Ekosistem skill v2 dengan 4 pilar — Bridge, Remote Install, Auto-Generate, Marketplace.

---

## III. User Stories

### P0 — Must Have

| ID | Story | Acceptance Criteria |
|----|-------|-------------------|
| US-01 | Workflow agent memanggil skill tersimpan | `Supervisor.SkillExecutor()` support prefix `skill:` routing ke store |
| US-02 | Install skill dari URL publik | `smara skill install <url>` download, validate, simpan ke `~/.smara/skills/` |
| US-03 | Workflow sukses ditawarkan jadi skill | Post-run capture tool calls + LLM generate + user confirm save |
| US-04 | Cari skill berdasarkan tag | `smara skill search [tag]` filter daftar skill |
| US-05 | Publish skill ke marketplace privat | `smara skill publish` validasi + upload ke registry |

### P1 — Should Have

| ID | Story | Acceptance Criteria |
|----|-------|-------------------|
| US-06 | Support GitHub/raw gist URL | Auto-convert GitHub blob/gist ke raw content URL |
| US-07 | Lihat detail skill sebelum install | `smara skill info <nama>` tampilkan deskripsi, steps, version |
| US-08 | Auto-check update skill | `smara skill update` cek manifest registry untuk versi lebih baru |
| US-09 | Auto-generate via LLM prompt | LLM rangkum tool calls menjadi skill JSON meaningful |

### P2 — Nice to Have

| ID | Story | Acceptance Criteria |
|----|-------|-------------------|
| US-10 | Rate dan review skill | Registry simpan rating/review per skill |
| US-11 | Parameterisasi skill | Skill definisikan `params` di-substitute saat runtime |
| US-12 | Marketplace web UI | Halaman web browse dan install skill |

---

## IV. Arsitektur & Desain

### 4.1 Skill-Workflow Execution Bridge

Saat ini `Supervisor.SkillExecutor()` (`internal/agent/supervisor.go:775-785`) hanya meroute ke `executeToolCall()` (built-in + MCP). Bridge v2 menambahkan routing skill invocation:

```
Workflow Agent ──► Supervisor ──► SkillExecutor()
                                      │
                              ┌───────┼───────┐
                              ▼       ▼       ▼
                           Built-in  MCP    skill:xxx
                                    (store)
```

Jika `toolName` diawali `skill:`, executor memuat skill dari store dan eksekusi steps secara rekursif. Contoh: `skill:deploy-static-site` memuat skill `deploy-static-site` dan jalankan steps-nya. Parameter substitution: runtime args di-merge ke step args sebelum eksekusi.

**Perubahan `Supervisor.SkillExecutor()`:**
```go
func (s *Supervisor) SkillExecutor() skill.StepExecutor {
    return func(toolName string, args map[string]interface{}) (string, error) {
        if strings.HasPrefix(toolName, "skill:") {
            skName := strings.TrimPrefix(toolName, "skill:")
            sk, err := skill.Load(skName)
            if err != nil {
                return "", fmt.Errorf("skill '%s' not found: %w", skName, err)
            }
            sk = sk.WithArgs(args)
            result, err := sk.Run(s.SkillExecutor())
            if err != nil { return "", err }
            return result.Summary, nil
        }
        tc := llm.ToolCall{Function: toolName, Args: args}
        return s.executeToolCall(tc)
    }
}
```

**Perubahan `internal/skill/types.go`:**
```go
type ParamDef struct {
    Name        string      `json:"name"`
    Type        string      `json:"type"`
    Description string      `json:"description"`
    Required    bool        `json:"required"`
    Default     interface{} `json:"default,omitempty"`
}

type Skill struct {
    Name        string     `json:"name"`
    Description string     `json:"description"`
    Steps       []Step     `json:"steps"`
    Version     int        `json:"version"`
    Tags        []string   `json:"tags,omitempty"`
    Author      string     `json:"author,omitempty"`
    SourceURL   string     `json:"source_url,omitempty"`
    Params      []ParamDef `json:"params,omitempty"`
}

// WithArgs returns copy dengan parameter substitution applied.
func (s *Skill) WithArgs(runtimeArgs map[string]interface{}) *Skill
```

### 4.2 Remote Skill Install

```
smara skill install <url>
    │
    ├── Parse URL (GitHub/gist/direct)
    ├── Download JSON (timeout 30s, max 1MB)
    ├── Validate schema (name, steps, no dup tools)
    ├── Check name collision (prompt overwrite if exists)
    └── Save ke ~/.smara/skills/<name>.json + SQLite
```

**URL Parsing Support:**

| Input Pattern | Resolved URL |
|---------------|--------------|
| Direct URL | `https://example.com/skill.json` |
| GitHub blob | `github.com/user/repo/blob/main/skill.json` → `raw.githubusercontent.com/user/repo/main/skill.json` |
| GitHub gist | `gist.github.com/user/gistid` → `gist.githubusercontent.com/user/gistid/raw/skill.json` |
| Shorthand | `user/repo/skill.json` → prepend `https://raw.githubusercontent.com/...` |

**CLI Commands:**
```bash
smara skill install https://raw.githubusercontent.com/user/repo/main/deploy-site.json
smara skill install https://github.com/user/repo/blob/main/deploy-site.json
smara skill install https://gist.github.com/user/abc123
smara skill install https://example.com/skill.json --as my-deploy
smara skill update deploy-site
smara skill info deploy-site
```

**Package baru: `internal/skill/installer.go`**
```go
package skill

type InstallOptions struct {
    URL       string
    Alias     string
    Overwrite bool
}

func InstallFromURL(opts InstallOptions) (*Skill, error)
func resolveGitHubURL(raw string) string
```

### 4.3 Auto-Generate Skill from Workflow

Setelah `RunWorkflow` (`internal/agent/workflow/workflow.go:36-57`) selesai, sistem menawarkan capture skill:

```
Workflow Result
       │
       ▼
Check: all success? && steps >= 2?
       │ Yes
       ▼
Collect Successful ToolCalls dari Orchestrator
       │
       ▼
LLM Prompt (Generate Skill JSON) ──► User Confirm (Y/n/name)
                                          │
                                          ▼
                                   skill.Save() ke store
```

**LLM Prompt Template:**
```
Kamu adalah Skill Generator untuk Smara CLI. Diberikan daftar tool calls sukses dalam workflow, buat skill JSON.

Tool calls:
- Tool: {{.Tool}}, Args: {{.ArgsJSON}}, Output: {{.OutputSummary}}

Buat skill JSON dengan:
1. "name": snake_case deskriptif (max 30 chars)
2. "description": 1-2 kalimat
3. "steps": array tool calls dalam urutan sama
4. "tags": array 1-3 tag kategori
5. "version": 1

HANYA output JSON.
```

**Package baru: `internal/skill/generator.go`**
```go
package skill

type WorkflowCapture struct {
    ProjectName string
    Steps       []CapturedStep
}

type CapturedStep struct {
    Tool          string
    Args          map[string]interface{}
    OutputSummary string
    Success       bool
}

func GenerateFromWorkflow(capture WorkflowCapture, provider llm.Provider) (*Skill, error)
func PromptUserForCapture(capture WorkflowCapture) (bool, string, error)
```

**Integrasi ke RunWorkflow:** Tambahkan flag `--capture` ke workflow command. Jika aktif dan run sukses, panggil `GenerateFromWorkflow` dan `PromptUserForCapture`.

### 4.3b Dual-Format Skill Storage (JSON + Markdown)

Skills dapat disimpan dalam format **JSON** (default, machine-optimized) atau **Markdown dengan YAML Frontmatter** (human-readable, git-diffable).

**Markdown Skill Format:**
```markdown
---
name: deploy-static-site
version: 3
tags: [deploy, netlify, frontend]
author: gede-cahya
source_url: https://raw.githubusercontent.com/.../deploy-site.json
params:
  - name: env
    type: string
    default: production
steps:
  - tool: build
    args:
      cmd: npm run build
  - tool: deploy
    args:
      provider: netlify
      dir: ./dist
---

# Deploy Static Site

Deploy website statis ke Netlify via CLI.

## Steps

1. **build** — Run `npm run build`
   - `cmd`: `npm run build`
2. **deploy** — Deploy to Netlify
   - `provider`: `netlify`
   - `dir`: `./dist`
```

**API:**
```go
func ParseMarkdownSkill(data []byte) (*Skill, error)
func (s *Skill) ToMarkdown() ([]byte, error)
func IsMarkdownSkill(data []byte) bool
func SaveAsMarkdown(s *Skill, db *sql.DB) error
```

**Behavior:**
- `Load(name)` tries `.json` first, then falls back to `.md`
- `List()` returns unique names across both `.json` and `.md` files
- `InstallFromURL()` auto-detects markdown by URL extension (`.md`) or content prefix (`---`)
- `Delete(name)` removes both `.json` and `.md` if they exist
- `skill create <name> --format md` accepts markdown input from stdin

### 4.4 Skill Marketplace / Registry

**Manifest Format (`skill-registry.json`):**
```json
{
  "registry_url": "https://smara-skills.example.com",
  "version": "1.0.0",
  "skills": [
    {
      "name": "deploy-static-site",
      "description": "Deploy static website ke Netlify via CLI",
      "version": 3,
      "author": "gede-cahya",
      "url": "https://raw.githubusercontent.com/.../deploy-static-site.json",
      "tags": ["deploy", "netlify", "frontend"],
      "downloads": 142,
      "rating": 4.5,
      "updated_at": "2026-05-10T12:00:00Z"
    }
  ]
}
```

**Default Registry:** GitHub repo dengan `skill-registry.json` di root. User tambahkan registry privat via config:
```yaml
skill_registries:
  - name: "smara-official"
    url: "https://raw.githubusercontent.com/gede-cahya/smara-skills/main/skill-registry.json"
  - name: "company-internal"
    url: "https://internal.example.com/smara/skills/registry.json"
    auth_token: "${SKILL_REGISTRY_TOKEN}"
```

**CLI Commands:**
```bash
smara skill search                    # list all
smara skill search deploy             # filter by tag/keyword
smara skill search --registry official
smara skill install deploy-static-site # install dari marketplace (resolve via registry)
smara skill publish deploy-static-site --registry company-internal
smara skill registry sync             # sync cache lokal
```

**Package baru: `internal/skill/registry.go`**
```go
package skill

type RegistryEntry struct {
    Name        string    `json:"name"`
    Description string    `json:"description"`
    Version     int       `json:"version"`
    Author      string    `json:"author"`
    URL         string    `json:"url"`
    Tags        []string  `json:"tags"`
    Downloads   int       `json:"downloads"`
    Rating      float64   `json:"rating"`
    UpdatedAt   time.Time `json:"updated_at"`
}

type RegistryManifest struct {
    RegistryURL string          `json:"registry_url"`
    Version     string          `json:"version"`
    Skills      []RegistryEntry `json:"skills"`
}

type RegistryConfig struct {
    Name      string `json:"name"`
    URL       string `json:"url"`
    AuthToken string `json:"auth_token,omitempty"`
}

func Search(query string, registries []RegistryConfig) ([]RegistryEntry, error)
func Publish(sk *Skill, registry RegistryConfig) error
func Sync(registries []RegistryConfig) error
```

---

## V. Spesifikasi Teknis

### 5.1 API Interface

| Function | Package | Signature | Description |
|----------|---------|-----------|-------------|
| `SkillExecutor()` | `internal/agent` | `func (s *Supervisor) SkillExecutor() skill.StepExecutor` | Extended dengan prefix `skill:` routing |
| `WithArgs()` | `internal/skill` | `func (s *Skill) WithArgs(args map[string]interface{}) *Skill` | Parameter substitution |
| `InstallFromURL()` | `internal/skill` | `func InstallFromURL(opts InstallOptions) (*Skill, error)` | Download + validate + save dari remote |
| `GenerateFromWorkflow()` | `internal/skill` | `func GenerateFromWorkflow(capture WorkflowCapture, provider llm.Provider) (*Skill, error)` | LLM-based generation dari captured steps |
| `Search()` | `internal/skill` | `func Search(query string, registries []RegistryConfig) ([]RegistryEntry, error)` | Search across registries |
| `Publish()` | `internal/skill` | `func Publish(sk *Skill, registry RegistryConfig) error` | Upload entry ke registry |
| `Sync()` | `internal/skill` | `func Sync(registries []RegistryConfig) error` | Cache manifests locally |

### 5.2 CLI Commands

```bash
# Remote Install
smara skill install <url> [--as <alias>] [--overwrite]
smara skill install <skill-name>              # install dari registry
smara skill update <skill-name>               # re-download dari source_url
smara skill info <skill-name>                 # detail skill

# Marketplace
smara skill search [query/tag] [--registry <name>]
smara skill publish <skill-name> --registry <name>
smara skill registry sync
smara skill registry list                     # daftar configured registries

# Existing commands (sudah ada)
smara skill run <nama>
smara skill list
smara skill delete <nama>
smara skill create <nama>
```

### 5.3 Package Structure

```
internal/skill/
├── types.go           # Skill, Step, ParamDef structs + WithArgs()
├── store.go           # Save, Load, Delete, List — dual-format JSON + MD
├── runner.go          # Run, StepExecutor (existing)
├── refinement.go      # Feedback, ProposeRefinement (existing)
├── installer.go       # InstallFromURL, resolveGitHubURL — auto-detect .md
├── generator.go       # WorkflowCapture, GenerateFromWorkflow, PromptUserForCapture
├── registry.go        # RegistryEntry, RegistryManifest, Search, Publish, Sync
├── registry_cache.go  # Local cache read/write untuk registry manifests
└── markdown.go        # ParseMarkdownSkill, ToMarkdown, IsMarkdownSkill, SaveAsMarkdown

cmd/smara/
├── skill.go          # EXTENDED — install, update, info, search, publish, registry subcommands + --format md
└── workflow.go       # EXTENDED — flag --capture untuk auto-generate skill post-run

internal/config/
└── config.go         # EXTENDED — SkillRegistries []RegistryConfig
```

### 5.4 Config Extension

```go
// internal/config/config.go
type SmaraConfig struct {
    // ... existing fields ...
    SkillRegistries []skill.RegistryConfig `mapstructure:"skill_registries" yaml:"skill_registries"`
}
```

---

## VI. Acceptance Criteria

### AC-1: Skill-Workflow Bridge
- [ ] Workflow agent bisa memanggil tool `skill:deploy-site` dan skill dieksekusi step-by-step
- [ ] Jika skill tidak ditemukan, error message jelas: `skill 'deploy-site' not found`
- [ ] Parameter substitution bekerja: runtime args override default skill args
- [ ] Nested skill call (skill A calls skill B) tidak menyebabkan infinite loop (max depth 5)

### AC-2: Remote Skill Install
- [ ] `smara skill install <direct-url>` berhasil download dan simpan
- [ ] `smara skill install <github-blob-url>` auto-convert ke raw URL
- [ ] `smara skill install <gist-url>` auto-convert ke raw gist URL
- [ ] Skill invalid (missing name/steps) ditolak dengan error validasi
- [ ] Name collision prompt user untuk overwrite atau cancel
- [ ] Download timeout 30s, max file size 1MB

### AC-3: Auto-Generate from Workflow
- [ ] Flag `--capture` pada workflow run aktifkan post-run capture
- [ ] Hanya workflow dengan semua step sukses dan >= 2 steps yang ditawarkan
- [ ] LLM prompt menghasilkan skill JSON valid
- [ ] User bisa confirm (Y), edit nama, atau cancel (n)
- [ ] Generated skill tersimpan di `~/.smara/skills/` dan muncul di `smara skill list`

### AC-4: Marketplace / Registry
- [ ] `smara skill search` list semua skill dari semua registries
- [ ] `smara skill search deploy` filter skill dengan tag/name mengandung "deploy"
- [ ] `smara skill install <skill-name>` resolve via registry cache, lalu download skill JSON
- [ ] `smara skill publish` validasi skill dan upload manifest entry
- [ ] `smara skill registry sync` download dan cache semua registry manifests
- [ ] Registry cache tersimpan di `~/.smara/registry-cache/`

---

## VII. Risiko & Mitigasi

| Risiko | Dampak | Mitigasi |
|--------|--------|----------|
| Skill dari remote berisi malicious tool calls | Eksekusi kode berbahaya | Validasi tool names whitelist, sandbox args, dan tanda tangan digital (future) |
| Infinite loop nested skill calls | Stack overflow / hang | Max recursion depth 5, cycle detection dalam skill call graph |
| Registry downtime | Install/search gagal | Local cache fallback, cache TTL 24 jam |
| LLM generate skill invalid | JSON parsing error | Validate LLM output, fallback ke manual editor |
| Skill name collision saat install | Data skill lama tertimpa | Prompt overwrite confirmation, backup skill lama ke `.bak` |

---

## VIII. Roadmap Implementasi

### Phase 1 — Bridge & Remote Install (v2.0.0)
- [ ] Extend `Supervisor.SkillExecutor()` dengan `skill:` prefix routing

### Phase 2 — Auto-Generate (v2.1.0)
- [x] `internal/skill/generator.go` — `WorkflowCapture`, `GenerateFromWorkflow`
- [x] Integrasi `--capture` flag ke `RunWorkflow`
- [x] `PromptUserForCapture` TUI/CLI confirmation flow

### Phase 2b — Markdown Support (v2.1.1)
- [x] `internal/skill/markdown.go` — `ParseMarkdownSkill`, `ToMarkdown`, `IsMarkdownSkill`
- [x] Dual-format storage: `.json` (primary) + `.md` (human-readable, git-diffable)
- [x] `Load()` auto-detect: `.json` first, fallback ke `.md`
- [x] `SaveAsMarkdown()` — serialize skill ke YAML frontmatter + markdown body
- [x] `cmd/smara skill create --format md` — input skill dari stdin sebagai markdown
- [x] `InstallFromURL()` auto-detect markdown content (URL `.md` atau body `---`)

### Phase 3 — Marketplace (v2.2.0)
- [x] `internal/skill/registry.go` — `RegistryManifest`, `FetchManifest`, `Search`, `Publish`
- [x] `internal/skill/registry_cache.go` — cache 24h untuk registry index
- [x] `cmd/smara/skill.go` — subcommand `search`, `publish`, `registry list`, `registry sync`
- [ ] Default public registry GitHub repo setup

### Phase 4 — Polish (v2.3.0)
- [ ] Parameterisasi skill (`params` + runtime substitution UI)
- [ ] Rate/review skill
- [ ] Skill dependency graph (skill A depends on skill B)
- [ ] Marketplace web UI (optional, far future)

---

**Status**: Ready for Review  
**Version**: 2.0.0-draft  
**Author**: Smara Team
