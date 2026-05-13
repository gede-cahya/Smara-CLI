package skill

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"
)

// TreeExport is the top-level envelope written to / read from an export
// file. It contains every saved skill plus enough metadata about the
// pattern-tracking table to preserve the auto-capture lineage across
// machines.
type TreeExport struct {
	Schema     string          `json:"schema"`      // always "smara.skill-tree/v1"
	Version    int             `json:"version"`     // bumped on breaking changes
	ExportedAt time.Time       `json:"exported_at"`
	Skills     []Skill         `json:"skills"`
	Patterns   []PatternExport `json:"patterns,omitempty"`
	Source     string          `json:"source,omitempty"` // optional identifier of origin machine
}

// PatternExport mirrors one row of auto_skill_patterns for export. Counts
// and timestamps are preserved so import can restore auto-capture history
// without re-triggering skill creation.
type PatternExport struct {
	Fingerprint    string      `json:"fingerprint"`
	Count          int         `json:"count"`
	FirstSeen      time.Time   `json:"first_seen"`
	LastSeen       time.Time   `json:"last_seen"`
	TraceSteps     []TraceStep `json:"trace_steps"`
	SamplePrompt   string      `json:"sample_prompt,omitempty"`
	CapturedSkill  string      `json:"captured_skill,omitempty"`
}

const exportSchema = "smara.skill-tree/v1"

// ExportAll collects every skill on disk plus the auto-skill pattern
// table (if the db is provided) into a TreeExport envelope.
//
// Skills are sorted by name for reproducible output.
func ExportAll(db *sql.DB, source string) (*TreeExport, error) {
	names, err := List()
	if err != nil {
		return nil, fmt.Errorf("gagal membaca daftar skill: %w", err)
	}
	sort.Strings(names)

	skills := make([]Skill, 0, len(names))
	for _, n := range names {
		sk, err := Load(n)
		if err != nil {
			// Skip skills that can't be loaded but note it in the log.
			fmt.Fprintf(os.Stderr, "[export] skip %s: %v\n", n, err)
			continue
		}
		// Clear derived fields so round-trips stay stable.
		sk.Children = nil
		skills = append(skills, *sk)
	}

	patterns, err := exportPatterns(db)
	if err != nil {
		// Non-fatal: lineage loss is acceptable if the pattern table is
		// unavailable.
		fmt.Fprintf(os.Stderr, "[export] patterns unavailable: %v\n", err)
	}

	return &TreeExport{
		Schema:     exportSchema,
		Version:    1,
		ExportedAt: time.Now(),
		Skills:     skills,
		Patterns:   patterns,
		Source:     source,
	}, nil
}

// WriteExport serializes an export envelope to JSON and writes it to w.
func WriteExport(w io.Writer, e *TreeExport) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(e)
}

// ExportToFile is a convenience wrapper that writes to a path.
func ExportToFile(db *sql.DB, path, source string) (int, error) {
	e, err := ExportAll(db, source)
	if err != nil {
		return 0, err
	}
	f, err := os.Create(path)
	if err != nil {
		return 0, fmt.Errorf("gagal membuat file: %w", err)
	}
	defer f.Close()
	if err := WriteExport(f, e); err != nil {
		return 0, fmt.Errorf("gagal menulis JSON: %w", err)
	}
	return len(e.Skills), nil
}

// exportPatterns reads every row of auto_skill_patterns.
func exportPatterns(db *sql.DB) ([]PatternExport, error) {
	if db == nil {
		return nil, nil
	}
	if err := EnsureAutoDetectTable(db); err != nil {
		return nil, err
	}
	rows, err := db.Query(`
		SELECT fingerprint, count, first_seen, last_seen, trace_json,
		       COALESCE(sample_prompt, ''), COALESCE(captured_skill, '')
		FROM auto_skill_patterns
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []PatternExport
	for rows.Next() {
		var p PatternExport
		var traceJSON string
		if err := rows.Scan(&p.Fingerprint, &p.Count, &p.FirstSeen, &p.LastSeen,
			&traceJSON, &p.SamplePrompt, &p.CapturedSkill); err != nil {
			continue
		}
		var steps []TraceStep
		if err := json.Unmarshal([]byte(traceJSON), &steps); err == nil {
			p.TraceSteps = steps
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// ImportConflictMode controls how ImportAll handles skills that already
// exist when the import runs.
type ImportConflictMode string

const (
	// ConflictSkip keeps the existing skill and ignores the imported one.
	ConflictSkip ImportConflictMode = "skip"
	// ConflictOverwrite replaces the existing skill with the imported one
	// and appends the old version to the imported skill's lineage.
	ConflictOverwrite ImportConflictMode = "overwrite"
	// ConflictRename imports the new skill with a numbered suffix so both
	// versions coexist. Parent/dependency references to the renamed skill
	// are *not* updated.
	ConflictRename ImportConflictMode = "rename"
)

// ImportResult summarizes what ImportAll did.
type ImportResult struct {
	Created        []string        `json:"created"`
	Overwritten    []string        `json:"overwritten"`
	Skipped        []string        `json:"skipped"`
	Renamed        map[string]string `json:"renamed,omitempty"` // original -> new
	PatternsLoaded int             `json:"patterns_loaded"`
	Warnings       []string        `json:"warnings,omitempty"`
}

// ImportAll restores an export envelope. If dryRun is true, the result
// describes what *would* happen without actually writing anything.
//
// Order: patterns are written first so that parent lookup during skill
// import can use them; but actually skills themselves carry their own
// ParentID which we trust directly.
func ImportAll(db *sql.DB, e *TreeExport, mode ImportConflictMode, dryRun bool) (*ImportResult, error) {
	if e == nil {
		return nil, fmt.Errorf("envelope kosong")
	}
	if e.Schema != "" && e.Schema != exportSchema {
		return nil, fmt.Errorf("schema tidak dikenal: %q (expected %q)", e.Schema, exportSchema)
	}
	if mode == "" {
		mode = ConflictOverwrite
	}

	res := &ImportResult{Renamed: map[string]string{}}

	// Pre-scan: build a map of existing skill names to check conflicts.
	existing := map[string]*Skill{}
	if names, err := List(); err == nil {
		for _, n := range names {
			if sk, err := Load(n); err == nil {
				existing[n] = sk
			}
		}
	}

	for _, sk := range e.Skills {
		// Work on a local copy so we can mutate fields safely.
		imported := sk
		if err := imported.Validate(); err != nil {
			res.Warnings = append(res.Warnings, fmt.Sprintf("skip invalid skill %q: %v", sk.Name, err))
			res.Skipped = append(res.Skipped, sk.Name)
			continue
		}

		prior, conflict := existing[imported.Name]
		if !conflict {
			if !dryRun {
				if err := Save(&imported, db); err != nil {
					res.Warnings = append(res.Warnings, fmt.Sprintf("save %s: %v", imported.Name, err))
					continue
				}
			}
			res.Created = append(res.Created, imported.Name)
			existing[imported.Name] = &imported
			continue
		}

		// Conflict handling
		switch mode {
		case ConflictSkip:
			res.Skipped = append(res.Skipped, imported.Name)

		case ConflictRename:
			newName := nextAvailableName(imported.Name, existing)
			imported.Name = newName
			if !dryRun {
				if err := Save(&imported, db); err != nil {
					res.Warnings = append(res.Warnings, fmt.Sprintf("save %s: %v", newName, err))
					continue
				}
			}
			res.Renamed[sk.Name] = newName
			existing[newName] = &imported

		case ConflictOverwrite:
			// Preserve prior version inside lineage so the refine history
			// is not lost when re-importing on an existing machine.
			AttachLineage(&imported, prior, "import")
			if imported.Version <= prior.Version {
				imported.Version = prior.Version + 1
			}
			if !dryRun {
				if err := Save(&imported, db); err != nil {
					res.Warnings = append(res.Warnings, fmt.Sprintf("save %s: %v", imported.Name, err))
					continue
				}
			}
			res.Overwritten = append(res.Overwritten, imported.Name)
			existing[imported.Name] = &imported

		default:
			res.Warnings = append(res.Warnings, fmt.Sprintf("unknown mode %q, skipping %s", mode, imported.Name))
			res.Skipped = append(res.Skipped, imported.Name)
		}
	}

	// Patterns
	if !dryRun && db != nil && len(e.Patterns) > 0 {
		n, err := importPatterns(db, e.Patterns, mode)
		if err != nil {
			res.Warnings = append(res.Warnings, fmt.Sprintf("patterns: %v", err))
		}
		res.PatternsLoaded = n
	}

	return res, nil
}

// nextAvailableName finds "foo", "foo-2", "foo-3", ... that doesn't collide.
func nextAvailableName(base string, existing map[string]*Skill) string {
	if _, ok := existing[base]; !ok {
		return base
	}
	for i := 2; i < 10000; i++ {
		candidate := fmt.Sprintf("%s-%d", base, i)
		if _, ok := existing[candidate]; !ok {
			return candidate
		}
	}
	// Absolute fallback: append a timestamp.
	return fmt.Sprintf("%s-%d", base, time.Now().UnixNano())
}

// importPatterns restores auto_skill_patterns rows. Returns the number of
// rows written. ConflictSkip preserves existing rows; Overwrite/Rename
// always upsert.
func importPatterns(db *sql.DB, patterns []PatternExport, mode ImportConflictMode) (int, error) {
	if err := EnsureAutoDetectTable(db); err != nil {
		return 0, err
	}
	tx, err := db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	written := 0
	for _, p := range patterns {
		traceJSON, err := json.Marshal(p.TraceSteps)
		if err != nil {
			continue
		}
		if mode == ConflictSkip {
			// Only insert if not present
			var count int
			_ = tx.QueryRow(`SELECT count FROM auto_skill_patterns WHERE fingerprint = ?`, p.Fingerprint).Scan(&count)
			if count > 0 {
				continue
			}
		}
		_, err = tx.Exec(`
			INSERT INTO auto_skill_patterns (fingerprint, count, first_seen, last_seen, trace_json, sample_prompt, captured_skill)
			VALUES (?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(fingerprint) DO UPDATE SET
				count = excluded.count,
				last_seen = excluded.last_seen,
				trace_json = excluded.trace_json,
				sample_prompt = excluded.sample_prompt,
				captured_skill = excluded.captured_skill
		`, p.Fingerprint, p.Count, p.FirstSeen, p.LastSeen, string(traceJSON), p.SamplePrompt, p.CapturedSkill)
		if err != nil {
			continue
		}
		written++
	}
	return written, tx.Commit()
}

// ReadExport decodes an export envelope from r.
func ReadExport(r io.Reader) (*TreeExport, error) {
	var e TreeExport
	if err := json.NewDecoder(r).Decode(&e); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}
	if e.Schema != "" && e.Schema != exportSchema {
		return nil, fmt.Errorf("schema tidak didukung: %q", e.Schema)
	}
	return &e, nil
}

// ImportFromFile reads and applies an export file atomically.
func ImportFromFile(db *sql.DB, path string, mode ImportConflictMode, dryRun bool) (*ImportResult, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("gagal buka file: %w", err)
	}
	defer f.Close()
	e, err := ReadExport(f)
	if err != nil {
		return nil, err
	}
	return ImportAll(db, e, mode, dryRun)
}

// ValidateImportModeString turns a loose user input ("skip", "OVERWRITE",
// " rename ") into a canonical ImportConflictMode or returns an error.
func ValidateImportModeString(s string) (ImportConflictMode, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "overwrite":
		return ConflictOverwrite, nil
	case "skip":
		return ConflictSkip, nil
	case "rename":
		return ConflictRename, nil
	default:
		return "", fmt.Errorf("mode '%s' tidak dikenal (pilih: overwrite, skip, rename)", s)
	}
}
