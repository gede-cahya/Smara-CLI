package memory

import (
	"encoding/json"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"pgregory.net/rapid"
)

func TestCommandBehaviorEquivalenceCloudVsLocalPBT(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		contents := rapid.SliceOfN(rapid.StringMatching(`[a-zA-Z0-9 _.-]{1,64}`), 1, 30).Draw(rt, "contents")
		queries := rapid.SliceOfN(rapid.IntRange(0, len(contents)-1), 1, 10).Draw(rt, "queries")

		local, err := NewSQLiteStore(filepath.Join(t.TempDir(), "local.db"))
		require.NoError(t, err)
		defer local.Close()
		cloud, err := NewSQLiteStoreWithDSN(filepath.Join(t.TempDir(), "cloud.db")+"?_journal_mode=WAL&_busy_timeout=5000", StoreOptions{DeviceID: "equiv-device", CloudEnabled: true})
		require.NoError(t, err)
		defer cloud.Close()

		const wsID int64 = 1
		for _, content := range contents {
			lm, err := local.Save(content, "equiv", "pbt", wsID, nil)
			require.NoError(t, err)
			cm, err := cloud.Save(content, "equiv", "pbt", wsID, nil)
			require.NoError(t, err)
			require.NotZero(t, lm.ID)
			require.NotZero(t, cm.ID)
		}

		localList, err := local.List(wsID, len(contents)+5)
		require.NoError(t, err)
		cloudList, err := cloud.List(wsID, len(contents)+5)
		require.NoError(t, err)
		require.Equal(t, memoryContents(localList), memoryContents(cloudList))

		for _, idx := range queries {
			query := contents[idx]
			lf := MemoryFilters{Limit: len(contents) + 5}
			cf := MemoryFilters{Limit: len(contents) + 5}
			localFiltered, totalL, err := local.ListMemoriesWithFilters(wsID, lf)
			require.NoError(t, err)
			cloudFiltered, totalC, err := cloud.ListMemoriesWithFilters(wsID, cf)
			require.NoError(t, err)
			require.Equal(t, totalL, totalC)
			require.Equal(t, memoryContents(localFiltered), memoryContents(cloudFiltered))

			// Search(nil) models command read behavior without contacting an embedding provider.
			ls, err := local.Search(nil, wsID, 10)
			require.NoError(t, err)
			cs, err := cloud.Search(nil, wsID, 10)
			require.NoError(t, err)
			require.Equal(t, searchContents(ls), searchContents(cs))
			require.NotEmpty(t, query)
		}

		localExport, _, err := local.ExportMemories(wsID, ExportOptions{Format: ExportJSON, IncludeMetadata: true})
		require.NoError(t, err)
		cloudExport, _, err := cloud.ExportMemories(wsID, ExportOptions{Format: ExportJSON, IncludeMetadata: true})
		require.NoError(t, err)
		require.NotContains(t, string(localExport), "cloud_id")
		require.NotContains(t, string(localExport), "device_id")
		require.NotContains(t, string(cloudExport), "cloud_id")
		require.NotContains(t, string(cloudExport), "device_id")
		require.Equal(t, exportedContents(localExport), exportedContents(cloudExport), "exports must match after ignoring timestamps/cloud-only columns")
	})
}

func exportedContents(data []byte) []string {
	var rows []struct {
		Content string   `json:"content"`
		Source  string   `json:"source"`
		Tags    []string `json:"tags"`
		Version int      `json:"version"`
	}
	_ = json.Unmarshal(data, &rows)
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		sort.Strings(r.Tags)
		out = append(out, r.Content+"|"+r.Source+"|"+strings.Join(r.Tags, ",")+"|"+strconv.Itoa(r.Version))
	}
	sort.Strings(out)
	return out
}
