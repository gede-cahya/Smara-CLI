# 🔍 Smara CLI — PRD: Reverse Engineering Domain

> **Smara** (Sanskerta: स्मृति) — "Ingatan" | Autonomous Multi-Agent Terminal
> **Versi PRD**: 1.0.0 | **Tanggal**: 2026-05-10 | **Status**: Draft

---

## I. Executive Summary

PRD ini mendefinisikan domain **Reverse Engineering (RE)** untuk Smara CLI — sebuah spesialisasi agen yang memungkinkan analisis static terhadap file binary, firmware, dan source code secara read-only. Domain ini ditujukan untuk security researcher, firmware engineer, dan code archaeologist yang perlu memahami struktur software tanpa mengeksekusi target.

### Visi
> _"Analisis binary dan source code secara otomatis, aman, dan terdokumentasi — tanpa pernah mengeksekusi target."_

### Prinsip Desain
1. **Read-Only by Default** — Semua RE tools bersifat read-only; tidak ada eksekusi binary
2. **Safety First** — File yang tidak dikenali tidak akan dieksekusi; hanya static analysis
3. **Graceful Degradation** — Tools mencoba CLI eksternal (`file`, `strings`) lalu fallback ke pure-Go
4. **Cap & Stream** — File besar di-cap ke 50 MB; entropy dihitung dari sample
5. **Audit Trail** — Semua analisis di-log untuk compliance dan reproducibility

---

## II. Konteks & Motivasi

### Masalah Saat Ini
1. **Tidak ada tooling RE di Smara** — User tidak bisa meminta analisis firmware atau malware melalui workflow agent
2. **Manual tooling** — Tools seperti `strings`, `binwalk`, `objdump` harus dijalankan manual di terminal
3. **Tidak ada integrasi LLM** — Hasil analisis binary tidak terintegrasi dengan planning/agents
4. **Kurangnya safety guardrails** — Tidak ada batasan otomatis terhadap eksekusi file yang tidak dikenali

### Solusi
Domain RE yang terintegrasi penuh dengan Smara workflow engine, dilengkapi 5 built-in tools untuk binary analysis dan source-code archaeology.

---

## III. User Stories

### P0 — Must Have
| ID | Story | Acceptance Criteria |
|----|-------|-------------------|
| **US-01** | Sebagai security researcher, saya ingin mengidentifikasi format dan arsitektur sebuah binary | Tool `analyze_binary` mendeteksi magic bytes (ELF/PE/Mach-O), bitness, dan entropy |
| **US-02** | Sebagai firmware engineer, saya ingin mengekstrak strings dari firmware.bin | Tool `extract_strings` mengekstrak ASCII/Unicode strings dengan panjang minimum yang dapat dikonfigurasi |
| **US-03** | Sebagai malware analyst, saya ingin scan signature/pattern terhadap sample | Tool `scan_signature` mendukung hex patterns, regex, dan plain string matching dengan confidence scoring |
| **US-04** | Sebagai code archaeologist, saya ingin memetakan dependency tree sebuah codebase | Tool `analyze_dependencies` memetakan internal vs external dependencies untuk Go/JS/TS/Python |
| **US-05** | Sebagai reverse engineer, saya ingin melihat call graph dari source code | Tool `generate_call_graph` membuat outline static call graph berdasarkan regex/AST scan |

### P1 — Should Have
| ID | Story | Acceptance Criteria |
|----|-------|-------------------|
| **US-06** | Sebagai user, saya ingin menjalankan workflow RE multi-step dengan satu perintah | Skill template `re_analyzer` tersedia dan dapat dijalankan via `skill_run` |
| **US-07** | Sebagai researcher, saya ingin domain RE terdeteksi otomatis dari prompt | Domain `reverse_engineering` terdaftar di `DomainRegistry` dengan keyword detection |
| **US-08** | Sebagai analyst, saya ingin hasil analisis binary dikelola oleh agent spesialis | Roles `binary_analyst` dan `code_archaeologist` tersedia dengan system prompt dan tools bawaan |

### P2 — Nice to Have
| ID | Story | Acceptance Criteria |
|----|-------|-------------------|
| **US-09** | Sebagai advanced user, saya ingin terhubung ke Ghidra/radare2 MCP server | Dokumentasi cara connect MCP server reverse engineering (Ghidra, r2, Binary Ninja) |
| **US-10** | Sebagai analyst, saya ingin melihat entropy per section (PE/ELF sections) | `analyze_binary` mendukung section-level entropy (future enhancement) |

---

## IV. Arsitektur & Desain

### 4.1 Domain Registration

```
┌─────────────────────────────────────────┐
│         DomainRegistry                  │
│  reverse_engineering                    │
│    ├── binary_analyst                   │
│    └── code_archaeologist               │
└─────────────────────────────────────────┘
```

- **File**: `internal/agent/workflow/blueprint.go`
- Domain terdaftar dengan keyword detection untuk prompt seperti "reverse engineer firmware", "analyze binary", "malware static analysis", "call graph", "dependency map"

### 4.2 Role Definitions

#### `binary_analyst`
- **Tools bawaan**: `analyze_binary`, `extract_strings`, `scan_signature`, `view_file`, `read_file`, `write_file`
- **System Prompt**: Read-only analysis; identifikasi format, entropy, strings, signatures; jangan eksekusi target

#### `code_archaeologist`
- **Tools bawaan**: `analyze_dependencies`, `generate_call_graph`, `grep_search`, `view_file`, `read_file`, `write_file`
- **System Prompt**: Parse tree source code, mapping imports, function definitions → callers, dokumentasikan arsitektur

### 4.3 Built-in RE Tools

| Tool | Input | Output | CLI Fallback | Pure-Go Fallback |
|------|-------|--------|-------------|-----------------|
| `analyze_binary` | `file_path` | Format, arch, entropy, packer indicators | `file` command | Magic bytes + entropy calculation |
| `extract_strings` | `file_path`, `min_length`, `max_results` | List of strings | `strings -n` | Byte scanner for printable ASCII |
| `scan_signature` | `file_path`, `patterns[]` | Match count, offsets, confidence | — | Naive byte/regex search |
| `analyze_dependencies` | `source_path`, `language` | Internal + external dependency lists | — | Regex-based import extraction |
| `generate_call_graph` | `source_path`, `language`, `max_depth` | Function → callers outline | — | Regex-based function scan |

### 4.4 Skill Template

**File**: `internal/skill/templates/re_analyzer.json`

Multi-step recipe:
1. `analyze_binary` → identifikasi format dan entropy
2. `extract_strings` → ekstrak strings menarik
3. `scan_signature` → cocokkan dengan known patterns
4. `analyze_dependencies` → peta dependency source code
5. `generate_call_graph` → buat outline call graph

**Usage**: `smara start` dalam mode Plan/Rush dengan prompt "Analyze unknown firmware.bin and reconstruct its module map" akan memicu domain detection RE dan men-spawn agents yang relevan.

### 4.5 MCP Extensibility

User dapat menghubungkan MCP server khusus untuk disassembly lanjutan:
- `connect_mcp` dengan type `local` atau `remote`
- Contoh: Ghidra Bridge, radare2 MCP, Binary Ninja MCP
- Tools built-in RE tetap berfungsi sebagai layer awal (triage) sebelum analisis mendalam

---

## V. Functional Requirements

### FR-01: Binary Identification
- Deteksi magic bytes untuk ELF, PE, Mach-O, ZIP/GZIP/BZIP2/XZ, PNG, JPEG
- Bitness dan architecture hint (x86, x86-64, ARM, AArch64)
- Entropy calculation Shannon pada sample 8KB pertama

### FR-02: String Extraction
- ASCII printable (0x20-0x7E)
- UTF-8 lead byte support (0xC0-0xFD)
- Minimum length configurable (default 4)
- Result cap 2000 strings

### FR-03: Signature Scanning
- Hex pattern: `"48 89 E5"` → parsed menjadi byte sequence
- Regex pattern: `"regex:https?://.*"`
- Plain string match
- Confidence scoring: `hex-match` > `regex-match` > `plain-match`
- Sample offsets (max 5 per pattern)

### FR-04: Dependency Mapping
- **Go**: Parse `import` blocks, deteksi module prefix dari `go.mod`
- **JavaScript/TypeScript**: `import`, `require`, `from` statements
- **Python**: `import` dan `from ... import`
- Klasifikasi internal vs external dependencies

### FR-05: Call Graph Generation
- **Go**: `func Name(` definitions
- **JS/TS**: `function Name(`, `const Name = (...)`
- **Python**: `def Name(` definitions
- Naive caller detection via `Name(` regex scan
- Deduplicated caller list per function

---

## VI. Non-Functional Requirements

### NFR-01: Safety
- **NFR-01.1**: RE tools tidak pernah mengeksekusi file target
- **NFR-01.2**: `run_command` tidak dipanggil untuk binary yang dianalisis
- **NFR-01.3**: File size di-cap ke 50 MB; file lebih besar ditolak dengan error

### NFR-02: Portability
- **NFR-02.1**: Jika `file`/`strings` CLI tidak tersedia (e.g., Windows tanpa WSL), fallback ke pure-Go
- **NFR-02.2**: Semua tools dapat berjalan tanpa dependensi eksternal

### NFR-03: Performance
- **NFR-03.1**: Entropy dihitung dari sample 8KB, bukan seluruh file
- **NFR-03.2**: String extraction dan signature scan streaming (chunked read)
- **NFR-03.3**: Source analysis di-cap ke 500 file per operation

### NFR-04: Audit
- **NFR-04.1**: Semua tool execution di-log melalui `logCallback` yang ada
- **NFR-04.2**: Hasil analisis dapat disimpan ke file via `write_file`

---

## VII. Acceptance Criteria & Milestones

### Milestone 1: Domain & Roles ✅
- [x] `reverse_engineering` terdaftar di `DomainRegistry`
- [x] `binary_analyst` dan `code_archaeologist` roles terdefinisi
- [x] Keyword detection untuk RE domain berfungsi

### Milestone 2: Built-in Tools ✅
- [x] `analyze_binary` — magic bytes, entropy, format detection
- [x] `extract_strings` — ASCII/Unicode extraction dengan CLI fallback
- [x] `scan_signature` — hex/regex/plain pattern matching
- [x] `analyze_dependencies` — Go/JS/TS/Python dependency mapping
- [x] `generate_call_graph` — static call graph outline

### Milestone 3: Skill & Documentation ✅
- [x] Skill template `re_analyzer.json` tersedia
- [x] PRD dokumentasi lengkap

### Milestone 4: Verification & Testing
- [ ] Build sukses tanpa error
- [ ] `go test ./...` passing untuk package yang dimodifikasi
- [ ] Manual test: `analyze_binary` terhadap sample ELF/PE
- [ ] Manual test: `extract_strings` terhadap sample binary
- [ ] Manual test: `analyze_dependencies` terhadap codebase Go/JS

---

## VIII. Risks & Mitigations

| Risk | Impact | Mitigation |
|------|--------|------------|
| Large binary files | OOM, slow analysis | Cap 50 MB; stream reads; sample entropy |
| False positives in signature scan | Misleading report | Confidence scoring; sample offsets; never auto-delete |
| Cross-platform CLI missing | Tools fail on Windows | Graceful fallback to pure-Go implementation |
| Inaccurate call graph | Wrong architecture reconstruction | Document "naive/regex-based" limitation; suggest MCP server for advanced analysis |
| Execution of malware sample | Security incident | Strict read-only design; no `run_command` for binary; user education in system prompt |

---

## IX. Open Questions

1. Apakah perlu menambahkan tool `hex_dump` untuk melihat bytes per offset?
2. Apakah perlu integrasi dengan VirusTotal API via MCP untuk hash lookup?
3. Apakah perlu section-level entropy untuk PE/ELF (memerlukan parser lebih kompleks)?
4. Apakah perlu YARA rule engine integration (go-yara atau via MCP)?

---

*End of PRD — Reverse Engineering Domain v1.0.0*
