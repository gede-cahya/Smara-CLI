package memory

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"sort"
	"testing"

	"github.com/stretchr/testify/require"
	"pgregory.net/rapid"
)

func TestWriteDurabilityBeforeRemoteAckPBT(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		dbPath := filepath.Join(t.TempDir(), "memory.db")
		deviceID := "device-durability"
		store, err := NewSQLiteStoreWithDSN(dbPath+"?_journal_mode=WAL&_busy_timeout=5000", StoreOptions{DeviceID: deviceID, CloudEnabled: true})
		require.NoError(t, err)

		contents := rapid.SliceOfN(rapid.StringMatching(`[a-zA-Z0-9 _.-]{1,80}`), 1, 40).Draw(rt, "contents")
		ids := make([]int64, 0, len(contents))
		for i, content := range contents {
			mem, err := store.Save(content, "pbt,durability", fmt.Sprintf("src-%d", i), 1, nil)
			require.NoError(t, err)
			require.NotZero(t, mem.ID)
			ids = append(ids, mem.ID)
			got, err := store.GetMemoryByID(mem.ID)
			require.NoError(t, err)
			require.NotNil(t, got)
			require.Equal(t, content, got.Content)
		}
		require.NoError(t, store.Close())

		reopened, err := NewSQLiteStoreWithDSN(dbPath+"?_journal_mode=WAL&_busy_timeout=5000", StoreOptions{DeviceID: deviceID, CloudEnabled: true})
		require.NoError(t, err)
		defer reopened.Close()
		for i, id := range ids {
			got, err := reopened.GetMemoryByID(id)
			require.NoError(t, err)
			require.NotNil(t, got)
			require.Equal(t, contents[i], got.Content)
		}
	})
}

func TestLocalFirstReadAvailabilityPBT(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		contents := rapid.SliceOfN(rapid.StringMatching(`[a-zA-Z0-9 _.-]{1,60}`), 1, 30).Draw(rt, "contents")
		localPath := filepath.Join(t.TempDir(), "local.db")
		cloudPath := filepath.Join(t.TempDir(), "cloud.db")
		localStore, err := NewSQLiteStore(localPath)
		require.NoError(t, err)
		defer localStore.Close()
		cloudStore, err := NewSQLiteStoreWithDSN(cloudPath+"?_journal_mode=WAL&_busy_timeout=5000", StoreOptions{DeviceID: "offline-device", CloudEnabled: true})
		require.NoError(t, err)
		defer cloudStore.Close()

		for _, content := range contents {
			_, err := localStore.Save(content, "same", "pbt", 7, nil)
			require.NoError(t, err)
			_, err = cloudStore.Save(content, "same", "pbt", 7, nil)
			require.NoError(t, err)
		}

		localList, err := localStore.List(7, len(contents)+5)
		require.NoError(t, err)
		cloudList, err := cloudStore.List(7, len(contents)+5)
		require.NoError(t, err)
		require.Equal(t, memoryContents(localList), memoryContents(cloudList))

		localSearch, err := localStore.Search(nil, 7, 5)
		require.NoError(t, err)
		cloudSearch, err := cloudStore.Search(nil, 7, 5)
		require.NoError(t, err)
		require.Equal(t, searchContents(localSearch), searchContents(cloudSearch))
	})
}

func TestWorkspaceIsolationPBT(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		store, err := NewSQLiteStore(filepath.Join(t.TempDir(), "memory.db"))
		require.NoError(t, err)
		defer store.Close()

		workspaceIDs := []int64{1, 2, 3}
		for _, ws := range workspaceIDs {
			_, err := store.db.Exec(`INSERT OR IGNORE INTO workspaces (id, name) VALUES (?, ?)`, ws, fmt.Sprintf("ws-%d", ws))
			require.NoError(t, err)
		}
		for i := 0; i < rapid.IntRange(1, 60).Draw(rt, "n"); i++ {
			ws := rapid.SampledFrom(workspaceIDs).Draw(rt, fmt.Sprintf("ws-%d", i))
			content := fmt.Sprintf("ws-%d-row-%d-%s", ws, i, rapid.StringMatching(`[a-z0-9]{1,12}`).Draw(rt, fmt.Sprintf("content-%d", i)))
			_, err := store.Save(content, "iso", "pbt", ws, nil)
			require.NoError(t, err)
		}

		seenByWorkspace := map[int64]map[int64]struct{}{}
		for _, ws := range workspaceIDs {
			list, err := strictListWorkspace(store.db, ws)
			require.NoError(t, err)
			seenByWorkspace[ws] = map[int64]struct{}{}
			for _, id := range list {
				seenByWorkspace[ws][id] = struct{}{}
			}
		}
		for i, a := range workspaceIDs {
			for _, b := range workspaceIDs[i+1:] {
				for id := range seenByWorkspace[a] {
					_, overlap := seenByWorkspace[b][id]
					require.False(t, overlap, "memory %d appears in both workspace %d and %d", id, a, b)
				}
			}
		}

		_, err = store.db.Exec(`INSERT INTO cloud_databases (workspace_id, provider, db_name, db_url, region) VALUES (?, 'turso', 'db-one', 'libsql://one', 'sea')`, workspaceIDs[0])
		require.NoError(t, err)
		_, err = store.db.Exec(`INSERT INTO cloud_databases (workspace_id, provider, db_name, db_url, region) VALUES (?, 'turso', 'db-two', 'libsql://two', 'sea')`, workspaceIDs[0])
		require.Error(t, err)
	})
}

func memoryContents(memories []Memory) []string {
	out := make([]string, 0, len(memories))
	for _, m := range memories {
		out = append(out, m.Content)
	}
	sort.Strings(out)
	return out
}

func searchContents(results []SearchResult) []string {
	out := make([]string, 0, len(results))
	for _, r := range results {
		out = append(out, r.Memory.Content)
	}
	sort.Strings(out)
	return out
}

func strictListWorkspace(db *sql.DB, workspaceID int64) ([]int64, error) {
	rows, err := db.Query(`SELECT id FROM memories WHERE workspace_id = ? ORDER BY id`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}
