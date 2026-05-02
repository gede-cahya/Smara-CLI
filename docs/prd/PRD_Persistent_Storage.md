# 🌀 Smara CLI — PRD: Persistent Storage & Memory

> **Smara** (Sanskerta: स्मृति) — "Ingatan" | Autonomous Multi-Agent Terminal
> **Versi PRD**: 1.0.0 | **Tanggal**: 2026-04-25 | **Status**: Draft

---

## I. Executive Summary

PRD ini mendefinisikan pengembangan sistem **Persistent Storage & Memory** untuk Smara CLI — infrastruktur penyimpanan yang robust, terorganisir, dan mendukung kolaborasi tim. PRD ini membangun fondasi yang sudah ada (`internal/memory/` SQLite store) dan menambahkan fitur organisasi, pencarian tingkat lanjut, dan manajemen siklus hidup memori.

### Visi
> _"Memori tim yang cerdas, terorganisir, dan selalu sinkron — dari lokal hingga cloud."_

### Prinsip Desain
1. **Backward Compatible** — Enhancement tanpa breaking changes pada interface yang ada
2. **Search-First** — Hybrid search: semantic (vector) + keyword (full-text)
3. **Organization** — Tag, kategori, dan folder untuk struktur yang jelas
4. **Retention-Aware** — Kebijakan retensi cerdas dengan TTL dan auto-cleanup
5. **Collaboration-Ready** — Berbagi memori antar workspace dengan kontrol akses

### Success Metrics
| Metrik | Target | Measurement |
|--------|--------|---------------|
| Search Relevance | >85% | User feedback & click-through rate |
| Sync Latency | <5 menit | Delta sync completion time |
| Storage Efficiency | <10% overhead | DB size vs raw content |
| Zero Data Loss | 100% | Automated backup & recovery tests |
| Search Performance | <200ms (10K memories) | Benchmark dengan dataset realistis |

---

## II. Konteks & Motivasi

### 2.1 Current State Analysis

Smara CLI saat ini memiliki sistem memori yang terimplementasi di `internal/memory/`:

**Yang Sudah Berjalan:**
- SQLite-backed storage dengan WAL mode (`store.go:25`)
- Vector embeddings untuk semantic search (`search.go`)
- Workspace isolation (`store.go:104-115`)
- Delta-based cloud sync dengan `sync_log` table (`store.go:56-63`)
- CRUD operations: Save, List, Delete, Search (`store.go:121-176`)

**Struktur Data Saat Ini (`types.go:12-20`):**
```go
type Memory struct {
    ID          int64
    WorkspaceID int64
    Content     string
    Embedding   []float32 // BLOB in SQLite
    Tags        string    // Plain string, belum terstruktur
    Source      string    // "agent:worker-1", "user", "sync"
    CreatedAt   time.Time
}
```

### 2.2 Masalah & Pain Points

| Masalah | Dampak | Prioritas |
|---------|---------|-----------|
| **Tags tidak terstruktur** | Sulit filter berdasarkan multiple tags | P1 |
| **Tidak ada full-text search** | Pencarian kata kunci spesifik lambat | P1 |
| **Tidak ada retention policy** | DB membengkak, memori usang menumpuk | P1 |
| **Metadata terbatas** | Sulit track asal, konteks, dan relasi | P2 |
| **Tidak ada versioning** | Tidak bisa lihat/update history | P2 |
| **Export/import terbatas** | Sulit backup atau migrasi data | P2 |
| **Tidak ada kategori/folder** | Organisasi flat, sulit untuk tim besar | P1 |

### 2.3 User Stories

#### P0 — Must Have (Foundation)
| ID | Story | Acceptance Criteria |
|----|-------|---------------------|
| **US-01** | Sebagai agent, saya ingin menyimpan memori dengan metadata lengkap (updated_at, expires_at) | Field baru tersimpan dan ter-retrieve dengan benar |
| **US-02** | Sebagai user, saya ingin mencari memori dengan kombinasi keyword dan semantic search | Hybrid search mengembalikan hasil relevan <200ms |
| **US-03** | Sebagai team lead, saya ingin melihat statistik memori per workspace | Dashboard menampilkan count, size, top tags |

#### P1 — Should Have (Core Features)
| ID | Story | Acceptance Criteria |
|----|-------|---------------------|
| **US-04** | Sebagai user, saya ingin mengorganisir memori dengan tags yang terstruktur | Tags tersimpan sebagai array JSON, bisa filter multiple |
| **US-05** | Sebagai user, saya ingin membuat kategori/folder dalam workspace | Kategori bisa di-assign ke memory, list by category |
| **US-06** | Sebagai admin, saya ingin mengatur retention policy (TTL) per workspace | Memory dengan expires_at < now otomatis dihapus |
| **US-07** | Sebagai user, saya ingin filter memori berdasarkan tanggal, source, tags | Query parameter diterima di List/Search operation |
| **US-08** | Sebagai developer, saya ingin full-text search dengan SQLite FTS5 | Keyword search <50ms untuk 10K records |

#### P2 — Nice to Have (Advanced)
| ID | Story | Acceptance Criteria |
|----|-------|---------------------|
| **US-09** | Sebagai user, saya ingin export/import memori ke JSON/Markdown | Export menghasilkan file terstruktur, import restore dengan benar |
| **US-10** | Sebagai team lead, saya ingin berbagi memori antar workspace | Cross-workspace link dengan permission check |
| **US-11** | Sebagai user, saya ingin melihat version history dari memory | Update history tersimpan, bisa rollback |
| **US-12** | Sebagai admin, saya ingin menghapus memori secara bulk dengan filter | Bulk delete dengan preview count |

---

## III. Spesifikasi Teknis

### 3.1 Enhanced Data Models

**Memory Struct (enhanced dari `types.go:12-20`):**
```go
type Memory struct {
    ID          int64                  `json:"id"`
    WorkspaceID int64                  `json:"workspace_id"`
    CategoryID  *int64                 `json:"category_id,omitempty"` // NEW
    Content     string                 `json:"content"`
    Embedding   []float32              `json:"-"` // BLOB storage
    Tags        []string               `json:"tags"` // CHANGED: from string to []string
    Source      string                 `json:"source"`
    Metadata    map[string]interface{} `json:"metadata,omitempty"` // NEW
    CreatedAt   time.Time              `json:"created_at"`
    UpdatedAt   time.Time              `json:"updated_at"` // NEW
    ExpiresAt   *time.Time             `json:"expires_at,omitempty"` // NEW: TTL
    Version     int                    `json:"version"` // NEW: for versioning
}
```

**New: Category Struct:**
```go
type Category struct {
    ID          int64     `json:"id"`
    WorkspaceID int64     `json:"workspace_id"`
    Name        string    `json:"name"`
    Description string    `json:"description,omitempty"`
    ParentID    *int64    `json:"parent_id,omitempty"` // Hierarchical
    CreatedAt   time.Time `json:"created_at"`
}
```

**New: MemoryVersion Struct (untuk US-11):**
```go
type MemoryVersion struct {
    ID        int64     `json:"id"`
    MemoryID  int64     `json:"memory_id"`
    Content   string    `json:"content"`
    ChangedBy string    `json:"changed_by"`
    Reason    string    `json:"reason,omitempty"`
    CreatedAt time.Time `json:"created_at"`
}
```

### 3.2 Database Schema Changes

**Existing tables** (`store.go:40-78`) akan dimigrasi dengan:

```sql
-- 1. Enhance 'memories' table
ALTER TABLE memories ADD COLUMN updated_at DATETIME DEFAULT CURRENT_TIMESTAMP;
ALTER TABLE memories ADD COLUMN category_id INTEGER REFERENCES categories(id);
ALTER TABLE memories ADD COLUMN expires_at DATETIME;
ALTER TABLE memories ADD COLUMN version INTEGER DEFAULT 1;
ALTER TABLE memories ADD COLUMN metadata TEXT DEFAULT '{}'; -- JSON

-- 2. Create 'categories' table (NEW)
CREATE TABLE IF NOT EXISTS categories (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    workspace_id INTEGER NOT NULL,
    name TEXT NOT NULL,
    description TEXT DEFAULT '',
    parent_id INTEGER,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (workspace_id) REFERENCES workspaces(id) ON DELETE CASCADE,
    FOREIGN KEY (parent_id) REFERENCES categories(id) ON DELETE SET NULL
);

-- 3. Create 'memory_versions' table (NEW - untuk versioning)
CREATE TABLE IF NOT EXISTS memory_versions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    memory_id INTEGER NOT NULL,
    content TEXT NOT NULL,
    metadata TEXT DEFAULT '{}',
    changed_by TEXT DEFAULT '',
    reason TEXT DEFAULT '',
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (memory_id) REFERENCES memories(id) ON DELETE CASCADE
);

-- 4. Create FTS5 virtual table (NEW - untuk US-08)
CREATE VIRTUAL TABLE IF NOT EXISTS memories_fts USING fts5(
    content,
    tags,
    source,
    content='memories',
    content_rowid='id'
);

-- 5. Triggers untuk sync FTS5 with memories
CREATE TRIGGER IF NOT EXISTS memories_ai AFTER INSERT ON memories BEGIN
    INSERT INTO memories_fts(rowid, content, tags, source) VALUES (new.id, new.content, new.tags, new.source);
END;

CREATE TRIGGER IF NOT EXISTS memories_au AFTER UPDATE ON memories BEGIN
    INSERT INTO memories_fts(memories_fts, rowid, content, tags, source) VALUES ('delete', old.id, old.content, old.tags, old.source);
    INSERT INTO memories_fts(rowid, content, tags, source) VALUES (new.id, new.content, new.tags, new.source);
END;

CREATE TRIGGER IF NOT EXISTS memories_ad AFTER DELETE ON memories BEGIN
    INSERT INTO memories_fts(memories_fts, rowid, content, tags, source) VALUES ('delete', old.id, old.content, old.tags, old.source);
END;

-- 6. Indexes for new fields
CREATE INDEX IF NOT EXISTS idx_memories_category ON memories(category_id);
CREATE INDEX IF NOT EXISTS idx_memories_updated ON memories(updated_at DESC);
CREATE INDEX IF NOT EXISTS idx_memories_expires ON memories(expires_at) WHERE expires_at IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_categories_workspace ON categories(workspace_id);
```

### 3.3 Enhanced Storage Interface

**Current interface** (`types.go:68-102`) akan ditambahkan method-method berikut:

```go
type MemoryStore interface {
    // --- Existing methods (unchanged) ---
    Init() error
    Save(content, tags, source string, workspaceID int64, embedding []float32) (*Memory, error)
    Search(embedding []float32, workspaceID int64, topK int) ([]SearchResult, error)
    List(workspaceID int64, limit int) ([]Memory, error)
    Delete(id int64) error
    Clear() error
    GetUnsyncedMemories() ([]Memory, error)
    MarkSynced(memoryID int64, deltaHash string) error

    // --- NEW: Enhanced operations ---
    UpdateMemory(id int64, updates map[string]interface{}) error
    GetMemoryByID(id int64) (*Memory, error)
    SearchHybrid(query string, embedding []float32, workspaceID int64, topK int) ([]SearchResult, error)
    SearchFullText(query string, workspaceID int64, filters SearchFilters) ([]Memory, error)

    // --- NEW: Category operations ---
    CreateCategory(cat *Category) (*Category, error)
    GetCategory(id int64) (*Category, error)
    ListCategories(workspaceID int64) ([]Category, error)
    UpdateCategory(id int64, updates map[string]interface{}) error
    DeleteCategory(id int64) error

    // --- NEW: Advanced listing with filters ---
    ListMemoriesWithFilters(workspaceID int64, filters MemoryFilters) ([]Memory, int, error)

    // --- NEW: Retention ---
    DeleteExpiredMemories() (int, error)
    SetRetentionPolicy(workspaceID int64, ttlDays int) error

    // --- NEW: Export/Import ---
    ExportMemories(workspaceID int64, format string, filters MemoryFilters) ([]byte, error)
    ImportMemories(workspaceID int64, data []byte, format string) (int, error)

    // --- NEW: Versioning ---
    GetMemoryVersions(memoryID int64) ([]MemoryVersion, error)
    RollbackMemory(memoryID int64, versionID int64) error

    // Close() remains
    Close() error
}
```

**New Filter Types:**
```go
type SearchFilters struct {
    Tags       []string
    Sources    []string
    DateFrom   *time.Time
    DateTo     *time.Time
    CategoryID *int64
    MinScore   float64
}

type MemoryFilters struct {
    SearchFilters
    Limit  int
    Offset int
    SortBy string // "created_at", "updated_at", "relevance"
    SortDir string // "ASC", "DESC"
}
```

### 3.4 Hybrid Search Architecture

Kombinasi vector search (existing) dan full-text search (NEW):

```go
// SearchHybrid combines semantic and keyword search
func (s *SQLiteStore) SearchHybrid(query string, embedding []float32, workspaceID int64, topK int) ([]SearchResult, error) {
    // 1. Get vector search results (existing logic from search.go)
    vectorResults := s.Search(embedding, workspaceID, topK*2)

    // 2. Get full-text search results (NEW - FTS5)
    ftsResults := s.SearchFullText(query, workspaceID, SearchFilters{})

    // 3. Merge and score: 0.6 * vector_similarity + 0.4 * text_relevance
    merged := mergeResults(vectorResults, ftsResults, 0.6, 0.4)

    // 4. Re-rank and limit to topK
    sort.Slice(merged, func(i, j int) bool { return merged[i].Score > merged[j].Score })
    if len(merged) > topK {
        merged = merged[:topK]
    }

    return merged, nil
}
```

### 3.5 Package Structure (Enhanced)

```
internal/memory/
├── store.go            # (ENHANCE) Add new fields to Save, Update operations
├── search.go           # (ENHANCE) Add SearchFullText, enhance Search
├── types.go            # (ENHANCE) New structs: Category, MemoryVersion, Filters
├── category.go         # (NEW) Category CRUD operations
├── fts.go              # (NEW) Full-text search with FTS5
├── retention.go        # (NEW) TTL, cleanup, retention policies
├── export.go           # (NEW) Export/Import (JSON, Markdown)
├── version.go          # (NEW) Memory versioning & rollback
├── migrate.go          # (NEW) Database migration scripts
└── memory_test.go      # (NEW) Comprehensive tests

cmd/smara/
├── memory.go           # (ENHANCE) Add new CLI subcommands
└── category.go         # (NEW) Category management commands
```

---

## IV. Arsitektur Sistem

### 4.1 High-Level Architecture

```
┌──────────────────────────────────────────────────────────────────┐
│                      Smara CLI App                              │
├──────────────────────────────────────────────────────────────────┤
│                                                                │
│  ┌──────────────┐    ┌──────────────┐    ┌──────────────┐    │
│  │   Agent      │    │   CLI        │    │   Dashboard  │    │
│  │   Supervisor │    │   (memory)   │    │   (memory    │    │
│  │              │    │              │    │    metrics)   │    │
│  └──────┬───────┘    └──────┬───────┘    └──────┬───────┘    │
│         │                    │                    │              │
│         └────────────────────┼────────────────────┘              │
│                              │                                   │
│                    ┌─────────▼──────────┐                        │
│                    │  MemoryStore       │                        │
│                    │  Interface         │                        │
│                    └─────────┬──────────┘                        │
│                              │                                   │
│         ┌────────────────────┼────────────────────┐              │
│         │                    │                    │              │
│    ┌────▼─────┐      ┌──────▼──────┐     ┌──────▼──────┐      │
│    │ SQLite   │      │  FTS5       │     │  Vector     │      │
│    │  Store   │◄────►│  Search    │     │  Search     │      │
│    │          │      │  (NEW)      │     │  (Enhanced) │      │
│    └────┬─────┘      └─────────────┘     └─────────────┘      │
│         │                                                        │
│    ┌────▼─────────────────────────────────────────────┐          │
│    │          SQLite Database (~/.smara/memory.db)     │          │
│    │  ┌──────────┐ ┌──────────┐ ┌───────────────┐  │          │
│    │  │ memories │ │categories│ │memory_versions │  │          │
│    │  │ (enhanced│ │  (NEW)   │ │    (NEW)       │  │          │
│    │  └──────────┘ └──────────┘ └───────────────┘  │          │
│    │  ┌──────────┐ ┌──────────┐ ┌───────────────┐  │          │
│    │  │ sync_log │ │ workspaces│ │  memories_fts │  │          │
│    │  │          │ │           │ │   (NEW/FTS5)  │  │          │
│    │  └──────────┘ └──────────┘ └───────────────┘  │          │
│    └──────────────────────────────────────────────────┘          │
│                                                                │
└──────────────────────────────────────────────────────────────────┘
```

### 4.2 Data Flow: Save & Search

**Save Memory Flow:**
```
Agent/User
    │
    ▼
MemoryStore.Save(content, tags[], source, workspaceID, embedding)
    │
    ├─> Validate input & metadata
    ├─> Generate embedding (if not provided)
    ├─> INSERT into memories table (with new fields)
    ├─> INSERT into memories_fts (trigger auto-fires)
    ├─> INSERT into sync_log (for cloud sync)
    └─> Return Memory object
```

**Hybrid Search Flow:**
```
User Query: "optimasi database SQLite"
    │
    ├─> Generate embedding from query text
    │
    ▼
MemoryStore.SearchHybrid(query, embedding, workspaceID, topK)
    │
    ├─> [Vector Search] Find top K*2 by embedding similarity
    │   └─> SELECT ... FROM memories ORDER BY cosine_similarity(embedding, ?)
    │
    ├─> [FTS5 Search] Find by keyword relevance
    │   └─> SELECT ... FROM memories_fts WHERE memories_fts MATCH ?
    │
    ├─> [Merge & Score] Combined score = 0.6*vector + 0.4*text
    │
    └─> [Re-rank & Limit] Sort by combined score, return top K
```

### 4.3 CLI Command Enhancements

**Existing:** `smara memory` (`cmd/smara/memory.go`)

**New subcommands:**
```bash
# Enhanced list dengan filters
smara memory list --workspace default --tags "go,sqlite" --source "agent" --limit 50

# Search hybrid
smara memory search "optimasi database" --semantic --keyword

# Create with category & metadata
smara memory save "Content here" --tags "go,db" --category "Backend" --ttl 30d

# Category management
smara memory category create "Backend" --description "Backend-related memories"
smara memory category list
smara memory category delete <id>

# Export/Import
smara memory export --workspace default --format json > backup.json
smara memory import backup.json --workspace default

# Retention
smara memory retention set --workspace default --ttl 90d
smara memory cleanup --dry-run

# Version history
smara memory history <memory-id>
smara memory rollback <memory-id> --to-version 3
```

---

## V. Roadmap & Milestones

### Phase 1: Foundation (MVP) — 2 Weeks

**Week 1: Database & Schema**
- [ ] Create `migrate.go` dengan schema migration scripts
- [ ] Enhance `types.go` dengan Category, MemoryVersion structs
- [ ] Add `updated_at`, `expires_at`, `category_id`, `metadata` fields
- [ ] Create `categories` table dan FTS5 virtual table
- [ ] Implement triggers untuk sync FTS5

**Week 2: Core Operations**
- [ ] Implement `category.go` — Category CRUD
- [ ] Enhance `store.go` — UpdateMemory, GetMemoryByID
- [ ] Implement `fts.go` — SearchFullText method
- [ ] Add `ListMemoriesWithFilters` dengan filter support
- [ ] Unit tests untuk new operations

**Deliverables:**
- Enhanced database schema dengan migration
- Category management working
- Full-text search functional
- Basic filter support on List operation

---

### Phase 2: Core Features — 3 Weeks

**Week 3: Hybrid Search**
- [ ] Implement `SearchHybrid` method
- [ ] Merge logic dengan configurable weights
- [ ] Benchmark: vector vs keyword vs hybrid
- [ ] Update `search.go` untuk support hybrid mode

**Week 4: Retention & Cleanup**
- [ ] Implement `retention.go`
- [ ] TTL support pada Save operation
- [ ] Background cleanup goroutine (run every hour)
- [ ] `SetRetentionPolicy` per workspace
- [ ] `DeleteExpiredMemories` dengan dry-run mode

**Week 5: Export/Import**
- [ ] Implement `export.go`
- [ ] JSON export/import (preserve metadata, embeddings optional)
- [ ] Markdown export (human-readable)
- [ ] Bulk import dengan deduplication check
- [ ] Progress indicator untuk large datasets

**Deliverables:**
- Hybrid search production-ready
- Retention policy working
- Export/Import functional
- Enhanced CLI commands

---

### Phase 3: Advanced Features — 4 Weeks

**Week 6-7: Versioning**
- [ ] Implement `version.go`
- [ ] Auto-versioning on UpdateMemory
- [ ] GetMemoryVersions & RollbackMemory
- [ ] Version diff (text comparison)
- [ ] CLI: `memory history` dan `memory rollback`

**Week 8-9: Collaboration (Sharing)**
- [ ] Cross-workspace memory linking
- [ ] Permission model (owner, editor, viewer)
- [ ] ShareMemory, UnshareMemory operations
- [ ] CLI: `memory share`, `memory permissions`
- [ ] Sync protocol update untuk shared memories

**Deliverables:**
- Memory versioning system
- Sharing & collaboration features
- Complete CLI interface
- Documentation & tutorials

---

## VI. Testing Strategy

### 6.1 Unit Tests
| Component | Coverage Target | Focus Areas |
|-----------|-----------------|-------------|
| `store.go` | 90% | New fields, UpdateMemory, filters |
| `search.go` | 85% | Hybrid search, merge logic |
| `fts.go` | 95% | FTS5 queries, ranking |
| `category.go` | 90% | CRUD, hierarchical categories |
| `retention.go` | 85% | TTL, cleanup, policies |
| `export.go` | 80% | JSON/MD export, import dedup |

### 6.2 Integration Tests
- End-to-end save → search → retrieve flow
- Migration from old schema to new schema
- Hybrid search dengan dataset 10K memories
- Concurrent operations (multiple agents saving)

### 6.3 Performance Benchmarks
```go
// Benchmark targets
BenchmarkSearch_Hybrid_10KMemories < 200ms
BenchmarkSave_WithEmbedding < 50ms
BenchmarkFullTextSearch < 50ms
BenchmarkList_WithFilters_1KResults < 100ms
```

---

## VII. Risks & Mitigation

| Risk | Impact | Likelihood | Mitigation |
|------|--------|------------|------------|
| **FTS5 not available in modernc/sqlite** | High | Medium | Fallback to LIKE query, or switch to mattn/go-sqlite3 |
| **Migration fails on large DB** | High | Low | Backup before migration, step-by-step migration |
| **Hybrid search slower than expected** | Medium | Medium | Tune weights, add caching, limit result sets |
| **Embedding dimension mismatch** | High | Low | Version embeddings, re-embed on dimension change |
| **Breaking changes for existing agents** | High | Low | Maintain backward compatibility, deprecation warnings |

---

## VIII. Success Criteria (Definition of Done)

### Phase 1 (MVP):
- [ ] All new database tables created successfully
- [ ] Category CRUD operations working
- [ ] Full-text search returning relevant results
- [ ] Filters (tags, date, source) working on List
- [ ] Unit test coverage >85% for new code
- [ ] Migration from old schema tested

### Phase 2 (Core):
- [ ] Hybrid search with tunable weights
- [ ] Retention policy auto-cleanup working
- [ ] Export/Import with metadata preservation
- [ ] CLI commands documented and functional
- [ ] Performance benchmarks met

### Phase 3 (Advanced):
- [ ] Version history viewable and rollback-able
- [ ] Cross-workspace sharing implemented
- [ ] All user stories (US-01 to US-12) implemented
- [ ] Integration tests passing
- [ ] Documentation complete

---

## IX. Appendices

### A. References
- Existing code: `internal/memory/` package
- PRD Template: `PRD_Dashboard.md`
- Tech stack: Go 1.26.1, modernc.org/sqlite, spf13/cobra
- Vector search: `internal/memory/search.go`

### B. Future Considerations
- **Graph-based memory**: Relationship antara memories (knowledge graph)
- **Multi-modal embeddings**: Support gambar, kode, dokumen
- **Distributed storage**: Sharding untuk very large datasets
- **Real-time sync**: WebSocket ganti polling untuk sync
- **Memory summarization**: LLM-assisted auto-summary untuk long memories

### C. Change Log
| Date | Version | Changes |
|------|---------|---------|
| 2026-04-25 | 1.0.0 | Initial PRD draft |

---

**Status**: Ready for Review & Implementation
