package memory

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"pgregory.net/rapid"
)

type migrationMemorySnapshot struct {
	ID        int64
	Content   string
	Tags      string
	Source    string
	CreatedAt string
}

func TestCloudMigrationIdempotentRapid(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		n := rapid.IntRange(0, 80).Draw(t, "memory_count")
		k := rapid.IntRange(1, 8).Draw(t, "migration_runs")
		dir, err := os.MkdirTemp("", "smara-cloud-migration-*")
		if err != nil {
			t.Fatalf("temp dir: %v", err)
		}
		defer os.RemoveAll(dir)
		dbPath := filepath.Join(dir, "memory.db")

		store, err := NewSQLiteStore(dbPath)
		if err != nil {
			t.Fatalf("open store: %v", err)
		}
		defer store.Close()

		for i := 0; i < n; i++ {
			content := rapid.StringMatching(`[A-Za-z0-9 _.,:-]{0,64}`).Draw(t, fmt.Sprintf("content_%d", i))
			tags := rapid.StringMatching(`[a-z]{0,8}(,[a-z]{0,8}){0,3}`).Draw(t, fmt.Sprintf("tags_%d", i))
			source := rapid.StringMatching(`[a-z]{0,12}`).Draw(t, fmt.Sprintf("source_%d", i))
			if _, err := store.Save(content, tags, source, 0, nil); err != nil {
				t.Fatalf("save seed memory %d: %v", i, err)
			}
		}

		beforeRows, err := snapshotMigrationRows(store.db)
		if err != nil {
			t.Fatalf("snapshot rows before: %v", err)
		}
		if err := store.migrate(); err != nil {
			t.Fatalf("initial migrate: %v", err)
		}
		wantSchema, err := snapshotCloudSchema(store.db)
		if err != nil {
			t.Fatalf("snapshot schema after first migrate: %v", err)
		}

		for i := 0; i < k; i++ {
			if err := store.migrate(); err != nil {
				t.Fatalf("migrate run %d: %v", i, err)
			}
			gotSchema, err := snapshotCloudSchema(store.db)
			if err != nil {
				t.Fatalf("snapshot schema run %d: %v", i, err)
			}
			if !reflect.DeepEqual(wantSchema, gotSchema) {
				t.Fatalf("schema changed after repeated migrate\nwant=%#v\ngot=%#v", wantSchema, gotSchema)
			}
		}

		afterRows, err := snapshotMigrationRows(store.db)
		if err != nil {
			t.Fatalf("snapshot rows after: %v", err)
		}
		if !reflect.DeepEqual(beforeRows, afterRows) {
			t.Fatalf("rows changed after migration\nwant=%#v\ngot=%#v", beforeRows, afterRows)
		}
	})
}

func TestBackfillCloudFieldsRapid(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		n := rapid.IntRange(0, 120).Draw(t, "memory_count")
		deviceID := rapid.StringMatching(`[a-z0-9-]{8,40}`).Draw(t, "device_id")
		dir, err := os.MkdirTemp("", "smara-cloud-backfill-*")
		if err != nil {
			t.Fatalf("temp dir: %v", err)
		}
		defer os.RemoveAll(dir)
		dbPath := filepath.Join(dir, "memory.db")

		store, err := NewSQLiteStore(dbPath)
		if err != nil {
			t.Fatalf("open store: %v", err)
		}
		defer store.Close()

		for i := 0; i < n; i++ {
			content := rapid.StringMatching(`[A-Za-z0-9 _.,:-]{0,64}`).Draw(t, fmt.Sprintf("content_%d", i))
			tags := rapid.StringMatching(`[a-z]{0,8}(,[a-z]{0,8}){0,3}`).Draw(t, fmt.Sprintf("tags_%d", i))
			source := rapid.StringMatching(`[a-z]{0,12}`).Draw(t, fmt.Sprintf("source_%d", i))
			if _, err := store.Save(content, tags, source, 0, nil); err != nil {
				t.Fatalf("save seed memory %d: %v", i, err)
			}
		}

		beforeRows, err := snapshotMigrationRows(store.db)
		if err != nil {
			t.Fatalf("snapshot rows before: %v", err)
		}

		for i := 0; i < 3; i++ {
			changed, err := store.BackfillCloudFields(deviceID)
			if err != nil {
				t.Fatalf("backfill run %d: %v", i, err)
			}
			if i == 0 && changed != n {
				t.Fatalf("first backfill count: got %d want %d", changed, n)
			}
			if i > 0 && changed != 0 {
				t.Fatalf("backfill should be idempotent on run %d: got %d", i, changed)
			}
		}

		afterRows, err := snapshotMigrationRows(store.db)
		if err != nil {
			t.Fatalf("snapshot rows after: %v", err)
		}
		if !reflect.DeepEqual(beforeRows, afterRows) {
			t.Fatalf("non-cloud row fields changed\nwant=%#v\ngot=%#v", beforeRows, afterRows)
		}

		rows, err := store.db.Query(`SELECT id, content, cloud_id, device_id, content_hash FROM memories ORDER BY id`)
		if err != nil {
			t.Fatalf("query cloud fields: %v", err)
		}
		defer rows.Close()

		seenCloudID := map[string]bool{}
		count := 0
		for rows.Next() {
			var id int64
			var content, cloudID, gotDeviceID, gotHash string
			if err := rows.Scan(&id, &content, &cloudID, &gotDeviceID, &gotHash); err != nil {
				t.Fatalf("scan cloud fields: %v", err)
			}
			if cloudID == "" {
				t.Fatalf("row %d has empty cloud_id", id)
			}
			if seenCloudID[cloudID] {
				t.Fatalf("duplicate cloud_id %q", cloudID)
			}
			seenCloudID[cloudID] = true
			if gotDeviceID != deviceID {
				t.Fatalf("row %d device_id got %q want %q", id, gotDeviceID, deviceID)
			}
			sum := sha256.Sum256([]byte(content))
			wantHash := hex.EncodeToString(sum[:])
			if gotHash != wantHash {
				t.Fatalf("row %d content_hash got %q want %q", id, gotHash, wantHash)
			}
			count++
		}
		if err := rows.Err(); err != nil {
			t.Fatalf("iterate cloud fields: %v", err)
		}
		if count != n {
			t.Fatalf("verified row count got %d want %d", count, n)
		}

		var nullCloudIDs int
		if err := store.db.QueryRow(`SELECT COUNT(*) FROM memories WHERE cloud_id IS NULL OR device_id IS NULL OR content_hash IS NULL`).Scan(&nullCloudIDs); err != nil {
			t.Fatalf("count null cloud fields: %v", err)
		}
		if nullCloudIDs != 0 {
			t.Fatalf("cloud fields still null for %d rows", nullCloudIDs)
		}
	})
}

func snapshotMigrationRows(db *sql.DB) ([]migrationMemorySnapshot, error) {
	rows, err := db.Query(`SELECT id, content, tags, source, created_at FROM memories ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []migrationMemorySnapshot
	for rows.Next() {
		var snap migrationMemorySnapshot
		if err := rows.Scan(&snap.ID, &snap.Content, &snap.Tags, &snap.Source, &snap.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, snap)
	}
	return out, rows.Err()
}

func snapshotCloudSchema(db *sql.DB) (map[string][]string, error) {
	tables := []string{"memories", "cloud_databases", "cloud_conflicts", "sync_log"}
	out := make(map[string][]string, len(tables))
	for _, table := range tables {
		rows, err := db.Query(fmt.Sprintf(`PRAGMA table_info(%s)`, table))
		if err != nil {
			return nil, err
		}
		var cols []string
		for rows.Next() {
			var cid int
			var name, typ string
			var notNull int
			var defaultValue sql.NullString
			var pk int
			if err := rows.Scan(&cid, &name, &typ, &notNull, &defaultValue, &pk); err != nil {
				rows.Close()
				return nil, err
			}
			cols = append(cols, fmt.Sprintf("%d:%s:%s:%d:%s:%d", cid, name, typ, notNull, defaultValue.String, pk))
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return nil, err
		}
		rows.Close()
		out[table] = cols
	}
	return out, nil
}
