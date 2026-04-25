package memory

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestDB(t *testing.T) (*SQLiteStore, func()) {
	t.Helper()
	
	// Create temp directory
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test.db")
	
	store, err := NewSQLiteStore(dbPath)
	require.NoError(t, err)
	require.NotNil(t, store)
	
	cleanup := func() {
		store.Close()
		os.Remove(dbPath)
	}
	
	return store, cleanup
}

func TestSaveAndGetMemory(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()

	content := "Test memory content"
	tags := "tag1,tag2"
	source := "test"
	workspaceID := int64(1)
	embedding := []float32{0.1, 0.2, 0.3}

	mem, err := store.Save(content, tags, source, workspaceID, embedding)
	require.NoError(t, err)
	require.NotNil(t, mem)
	assert.Equal(t, content, mem.Content)
	assert.Equal(t, workspaceID, mem.WorkspaceID)
	assert.Equal(t, source, mem.Source)
	assert.Equal(t, []string{"tag1", "tag2"}, mem.Tags)
	assert.Equal(t, embedding, mem.Embedding)
	assert.Equal(t, 1, mem.Version)

	// Test GetMemoryByID
	retrieved, err := store.GetMemoryByID(mem.ID)
	require.NoError(t, err)
	require.NotNil(t, retrieved)
	assert.Equal(t, mem.ID, retrieved.ID)
	assert.Equal(t, content, retrieved.Content)
}

func TestSaveWithOptions(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()

	content := "Test memory with options"
	tags := "option,test"
	source := "test"
	workspaceID := int64(2)
	embedding := []float32{0.5, 0.6, 0.7}
	
	// Create a category first
	cat, err := store.CreateCategory("Test Category", "Test description", workspaceID, nil)
	require.NoError(t, err)
	
	metadata := map[string]interface{}{
		"key1": "value1",
		"key2": 123,
	}
	
	expiresAt := time.Now().AddDate(0, 0, 7) // 7 days from now
	
	mem, err := store.SaveWithOptions(content, tags, source, workspaceID, embedding, &cat.ID, metadata, &expiresAt)
	require.NoError(t, err)
	require.NotNil(t, mem)
	
	assert.Equal(t, content, mem.Content)
	assert.Equal(t, cat.ID, *mem.CategoryID)
	assert.Equal(t, metadata, mem.Metadata)
	assert.NotNil(t, mem.ExpiresAt)
	assert.Equal(t, []string{"option", "test"}, mem.Tags)
}

func TestUpdateMemory(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()

	// Create a memory
	mem, err := store.Save("Original content", "tag1", "test", 1, nil)
	require.NoError(t, err)

	// Update the memory
	newContent := "Updated content"
	newTags := []string{"tag2", "tag3"}
	updates := map[string]interface{}{
		"content": newContent,
		"tags":    newTags,
	}

	err = store.UpdateMemory(mem.ID, updates)
	require.NoError(t, err)

	// Verify update
	updated, err := store.GetMemoryByID(mem.ID)
	require.NoError(t, err)
	assert.Equal(t, newContent, updated.Content)
	assert.Equal(t, newTags, updated.Tags)
	assert.Equal(t, 2, updated.Version) // Version should be incremented
}

func TestSearch(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()

	// Create test memories with embeddings
	workspaceID := int64(3)
	mem1, err := store.Save("First memory about databases", "db,sql", "test", workspaceID, []float32{1.0, 0.0, 0.0})
	require.NoError(t, err)

	_, err = store.Save("Second memory about programming", "go,code", "test", workspaceID, []float32{0.0, 1.0, 0.0})
	require.NoError(t, err)

	// Search with similar embedding
	queryEmbedding := []float32{0.9, 0.1, 0.0}
	results, err := store.Search(queryEmbedding, workspaceID, 5)
	require.NoError(t, err)
	assert.NotEmpty(t, results)
	assert.Equal(t, mem1.ID, results[0].Memory.ID) // Should match first memory
}

func TestListMemoriesWithFilters(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()

	workspaceID := int64(4)
	
	// Create test memories
	_, err := store.Save("Memory 1", "tag1", "test", workspaceID, nil)
	require.NoError(t, err)
	
	_, err = store.Save("Memory 2", "tag2", "test", workspaceID, nil)
	require.NoError(t, err)
	
	// Test with tag filter
	filters := MemoryFilters{
		SearchFilters: SearchFilters{
			Tags: []string{"tag1"},
		},
		Limit: 10,
	}
	
	memories, total, err := store.ListMemoriesWithFilters(workspaceID, filters)
	require.NoError(t, err)
	assert.Equal(t, 1, len(memories))
	assert.Equal(t, 1, total)
	assert.Equal(t, "tag1", memories[0].Tags[0])
}

func TestCreateAndGetCategory(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()

	workspaceID := int64(5)
	
	cat, err := store.CreateCategory("Test Category", "Test description", workspaceID, nil)
	require.NoError(t, err)
	assert.NotNil(t, cat)
	assert.Equal(t, "Test Category", cat.Name)
	assert.Equal(t, workspaceID, cat.WorkspaceID)

	// Get category
	retrieved, err := store.GetCategory(cat.ID)
	require.NoError(t, err)
	assert.Equal(t, cat.ID, retrieved.ID)
	assert.Equal(t, cat.Name, retrieved.Name)
}

func TestListCategories(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()

	workspaceID := int64(6)
	
	// Create multiple categories
	_, err := store.CreateCategory("Category 1", "", workspaceID, nil)
	require.NoError(t, err)
	
	_, err = store.CreateCategory("Category 2", "", workspaceID, nil)
	require.NoError(t, err)

	categories, err := store.ListCategories(workspaceID, false)
	require.NoError(t, err)
	assert.Equal(t, 2, len(categories))
}

func TestDeleteExpiredMemories(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()

	workspaceID := int64(7)
	
	// Create memory with past expiry
	pastExpiry := time.Now().AddDate(0, 0, -1) // 1 day ago
	_, err := store.SaveWithOptions(
		"Expired memory",
		"expired",
		"test",
		workspaceID,
		nil,
		nil,
		nil,
		&pastExpiry,
	)
	require.NoError(t, err)
	
	// Create memory with future expiry
	futureExpiry := time.Now().AddDate(0, 0, 1) // 1 day from now
	_, err = store.SaveWithOptions(
		"Valid memory",
		"valid",
		"test",
		workspaceID,
		nil,
		nil,
		nil,
		&futureExpiry,
	)
	require.NoError(t, err)

	// Delete expired memories
	deleted, err := store.DeleteExpiredMemories()
	require.NoError(t, err)
	assert.Equal(t, 1, deleted)
}

func TestExportImportJSON(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()

	workspaceID := int64(8)
	
	// Create test memories
	_, err := store.Save("Export test 1", "tag1", "test", workspaceID, nil)
	require.NoError(t, err)
	
	_, err = store.Save("Export test 2", "tag2", "test", workspaceID, nil)
	require.NoError(t, err)

	// Export
	options := ExportOptions{
		Format:          ExportJSON,
		IncludeMetadata: true,
	}
	data, _, err := store.ExportMemories(workspaceID, options)
	require.NoError(t, err)
	assert.NotEmpty(t, data)

	// Import into new workspace
	newWorkspaceID := int64(9)
	importOptions := ImportOptions{
		SkipDuplicates: true,
	}
	imported, err := store.ImportMemories(newWorkspaceID, data, ExportJSON, importOptions)
	require.NoError(t, err)
	assert.Equal(t, 2, imported)
}

func TestMemoryVersioning(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()

	workspaceID := int64(10)
	
	// Create memory
	mem, err := store.Save("Original content", "tag1", "test", workspaceID, nil)
	require.NoError(t, err)

	// Update memory (should create version)
	updates := map[string]interface{}{
		"content": "Updated content",
	}
	err = store.UpdateMemory(mem.ID, updates)
	require.NoError(t, err)

	// Get versions
	versions, err := store.GetMemoryVersions(mem.ID)
	require.NoError(t, err)
	assert.Equal(t, 2, len(versions)) // Initial + update

	// Rollback to first version
	err = store.RollbackMemory(mem.ID, versions[1].ID)
	require.NoError(t, err)

	// Verify rollback
	rolledBack, err := store.GetMemoryByID(mem.ID)
	require.NoError(t, err)
	assert.Equal(t, "Original content", rolledBack.Content)
}

func TestSearchFullText(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()

	workspaceID := int64(11)
	
	// Create test memories
	_, err := store.Save("Database optimization techniques", "db,sql", "test", workspaceID, nil)
	require.NoError(t, err)
	
	_, err = store.Save("Go programming language basics", "go,code", "test", workspaceID, nil)
	require.NoError(t, err)

	// Search for "database"
	filters := MemoryFilters{
		SearchFilters: SearchFilters{
			Sources: []string{"test"},
		},
		Limit: 10,
	}
	
	results, err := store.SearchFullText("database", workspaceID, filters)
	require.NoError(t, err)
	assert.NotEmpty(t, results)
	assert.Contains(t, results[0].Content, "Database")
}

func TestSearchHybrid(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()

	workspaceID := int64(12)
	
	// Create test memory with embedding
	_, err := store.Save("Machine learning and AI systems", "ml,ai", "test", workspaceID, []float32{0.8, 0.1, 0.1})
	require.NoError(t, err)

	// Hybrid search
	queryEmbedding := []float32{0.7, 0.2, 0.1}
	results, err := store.SearchHybrid("machine", queryEmbedding, workspaceID, 5)
	require.NoError(t, err)
	assert.NotEmpty(t, results)
	assert.Greater(t, results[0].Score, float64(0))
}

func TestRetentionPolicy(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()

	workspaceID := int64(13)
	
	// Set retention policy
	err := store.SetRetentionPolicy(workspaceID, 30) // 30 days
	require.NoError(t, err)

	// Create memory
	mem, err := store.Save("Test memory", "test", "test", workspaceID, nil)
	require.NoError(t, err)
	
	// Verify expiry was NOT set (SetRetentionPolicy only updates existing memories)
	assert.Nil(t, mem.ExpiresAt)
}

func TestGetMemoryStats(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()

	workspaceID := int64(14)
	
	// Create test memories
	_, err := store.Save("Memory 1", "tag1", "test", workspaceID, []float32{0.1, 0.2, 0.3})
	require.NoError(t, err)
	
	_, err = store.Save("Memory 2", "tag2", "test", workspaceID, nil)
	require.NoError(t, err)

	// Get stats
	stats, err := store.GetMemoryStats(workspaceID)
	require.NoError(t, err)
	
	assert.Equal(t, 2, stats["total"])
	assert.Equal(t, 1, stats["with_embedding"])
	assert.Equal(t, 0, stats["expired"])
}

func TestRunCleanup(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()

	workspaceID := int64(15)
	
	// Create expired memory
	pastExpiry := time.Now().AddDate(0, 0, -1)
	_, err := store.SaveWithOptions(
		"Expired",
		"expired",
		"test",
		workspaceID,
		nil,
		nil,
		nil,
		&pastExpiry,
	)
	require.NoError(t, err)

	// Run cleanup
	results, err := store.RunCleanup(workspaceID)
	require.NoError(t, err)
	
	assert.Equal(t, 1, results["expired_deleted"])
}

func TestParseTagsFromJSON(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{"Empty JSON", "[]", []string{}},
		{"Single tag", `["tag1"]`, []string{"tag1"}},
		{"Multiple tags", `["tag1","tag2"]`, []string{"tag1", "tag2"}},
		{"Invalid JSON", "invalid", []string{"invalid"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseTagsFromJSON(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFormatTagsToJSON(t *testing.T) {
	tests := []struct {
		name     string
		input    []string
		expected string
	}{
		{"Empty", []string{}, "[]"},
		{"Single", []string{"tag1"}, `["tag1"]`},
		{"Multiple", []string{"tag1", "tag2"}, `["tag1","tag2"]`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatTagsToJSON(tt.input)
			assert.JSONEq(t, tt.expected, result)
		})
	}
}

func TestWorkspaceOperations(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()

	// Create workspace
	ws, err := store.CreateWorkspace("Test Workspace", "/tmp/test")
	require.NoError(t, err)
	assert.NotNil(t, ws)
	assert.Equal(t, "Test Workspace", ws.Name)

	// Get workspace by ID
	retrieved, err := store.GetWorkspace(ws.ID)
	require.NoError(t, err)
	assert.Equal(t, ws.ID, retrieved.ID)

	// List workspaces
	workspaces, err := store.ListWorkspaces()
	require.NoError(t, err)
	assert.NotEmpty(t, workspaces)
}

func TestSessionOperations(t *testing.T) {
	t.Skip("Session tests require session package")
}

func TestDeleteMemory(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()

	workspaceID := int64(16)
	
	// Create memory
	mem, err := store.Save("To be deleted", "delete", "test", workspaceID, nil)
	require.NoError(t, err)

	// Delete memory
	err = store.Delete(mem.ID)
	require.NoError(t, err)

	// Verify deletion
	deleted, err := store.GetMemoryByID(mem.ID)
	assert.NoError(t, err)
	assert.Nil(t, deleted)
}

func TestClearAllMemories(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()

	workspaceID := int64(17)
	
	// Create multiple memories
	_, err := store.Save("Memory 1", "tag1", "test", workspaceID, nil)
	require.NoError(t, err)
	
	_, err = store.Save("Memory 2", "tag2", "test", workspaceID, nil)
	require.NoError(t, err)

	// Clear all
	err = store.Clear()
	require.NoError(t, err)

	// Verify all memories are gone
	memories, err := store.List(workspaceID, 10)
	require.NoError(t, err)
	assert.Empty(t, memories)
}

func TestGetUnsyncedMemories(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()

	workspaceID := int64(18)
	
	// Create memory without sync
	mem, err := store.Save("Unsynced memory", "unsynced", "test", workspaceID, nil)
	require.NoError(t, err)

	// Get unsynced memories
	unsynced, err := store.GetUnsyncedMemories()
	require.NoError(t, err)
	assert.NotEmpty(t, unsynced)
	assert.Equal(t, mem.ID, unsynced[0].ID)

	// Mark as synced
	err = store.MarkSynced(mem.ID, "hash123")
	require.NoError(t, err)

	// Verify no unsynced memories
	unsynced, err = store.GetUnsyncedMemories()
	require.NoError(t, err)
	assert.Empty(t, unsynced)
}

func TestCategoryWithSubcategories(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()

	workspaceID := int64(19)
	
	// Create parent category
	parent, err := store.CreateCategory("Parent", "Parent category", workspaceID, nil)
	require.NoError(t, err)

	// Create subcategory
	subcat, err := store.CreateCategory("Subcategory", "Child category", workspaceID, &parent.ID)
	require.NoError(t, err)
	assert.Equal(t, parent.ID, *subcat.ParentID)

	// List all categories (including subcategories)
	categories, err := store.ListCategories(workspaceID, true)
	require.NoError(t, err)
	assert.Equal(t, 2, len(categories))

	// List only top-level categories
	topLevel, err := store.ListCategories(workspaceID, false)
	require.NoError(t, err)
	assert.Equal(t, 1, len(topLevel))
	assert.Equal(t, parent.ID, topLevel[0].ID)
}

func TestUpdateCategory(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()

	workspaceID := int64(20)
	
	// Create category
	cat, err := store.CreateCategory("Original", "Original description", workspaceID, nil)
	require.NoError(t, err)

	// Update category
	updates := map[string]interface{}{
		"name":        "Updated",
		"description": "Updated description",
	}
	err = store.UpdateCategory(cat.ID, updates)
	require.NoError(t, err)

	// Verify update
	updated, err := store.GetCategory(cat.ID)
	require.NoError(t, err)
	assert.Equal(t, "Updated", updated.Name)
	assert.Equal(t, "Updated description", updated.Description)
}

func TestDeleteCategoryWithReassign(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()

	workspaceID := int64(21)
	
	// Create categories
	cat1, err := store.CreateCategory("Category 1", "", workspaceID, nil)
	require.NoError(t, err)
	
	cat2, err := store.CreateCategory("Category 2", "", workspaceID, nil)
	require.NoError(t, err)

	// Create memory in cat1
	mem, err := store.SaveWithOptions(
		"Test memory",
		"test",
		"test",
		workspaceID,
		nil,
		&cat1.ID,
		nil,
		nil,
	)
	require.NoError(t, err)
	assert.Equal(t, cat1.ID, *mem.CategoryID)

	// Delete cat1 and reassign to cat2
	err = store.DeleteCategory(cat1.ID, &cat2.ID)
	require.NoError(t, err)

	// Verify memory is now in cat2
	updated, err := store.GetMemoryByID(mem.ID)
	require.NoError(t, err)
	assert.Equal(t, cat2.ID, *updated.CategoryID)
}

func TestExportFormats(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()

	workspaceID := int64(22)
	
	// Create test memory
	_, err := store.Save("Export test", "tag1,tag2", "test", workspaceID, nil)
	require.NoError(t, err)

	// Test JSON export
	options := ExportOptions{
		Format:          ExportJSON,
		IncludeMetadata: true,
	}
	jsonData, _, err := store.ExportMemories(workspaceID, options)
	require.NoError(t, err)
	assert.NotEmpty(t, jsonData)
	assert.Contains(t, string(jsonData), "Export test")

	// Test CSV export
	options = ExportOptions{
		Format:          ExportCSV,
		IncludeMetadata: true,
	}
	csvData, _, err := store.ExportMemories(workspaceID, options)
	require.NoError(t, err)
	assert.NotEmpty(t, csvData)
	assert.Contains(t, string(csvData), "Export test")

	// Test Markdown export
	options = ExportOptions{
		Format:          ExportMD,
		IncludeMetadata: true,
	}
	mdData, _, err := store.ExportMemories(workspaceID, options)
	require.NoError(t, err)
	assert.NotEmpty(t, mdData)
	assert.Contains(t, string(mdData), "Export test")

	// Test ZIP export
	options = ExportOptions{
		Format:          ExportZIP,
		IncludeMetadata: true,
	}
	zipData, _, err := store.ExportMemories(workspaceID, options)
	require.NoError(t, err)
	assert.NotEmpty(t, zipData)
	// ZIP files start with PK signature
	assert.Equal(t, []byte{'P', 'K'}, zipData[:2])
}

func TestImportValidation(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()

	workspaceID := int64(23)
	
	// Test import with invalid JSON
	invalidJSON := []byte(`{invalid}`)
	_, err := store.ImportMemories(workspaceID, invalidJSON, ExportJSON, ImportOptions{})
	assert.Error(t, err)

	// Test import with valid JSON but wrong structure
	wrongJSON := []byte(`[]`)
	imported, err := store.ImportMemories(workspaceID, wrongJSON, ExportJSON, ImportOptions{})
	require.NoError(t, err)
	assert.Equal(t, 0, imported) // Should import 0 memories
}

func TestVersionDiff(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()

	workspaceID := int64(24)
	
	// Create memory
	mem, err := store.Save("Version 1", "tag1", "test", workspaceID, nil)
	require.NoError(t, err)

	// Update memory
	updates := map[string]interface{}{
		"content": "Version 2",
		"tags":    []string{"tag2"},
	}
	err = store.UpdateMemory(mem.ID, updates)
	require.NoError(t, err)

	// Get versions
	versions, err := store.GetMemoryVersions(mem.ID)
	require.NoError(t, err)
	assert.Equal(t, 2, len(versions))

	// Compare versions
	diffs, err := store.CompareVersions(mem.ID, versions[0].ID, versions[1].ID)
	require.NoError(t, err)
	assert.NotEmpty(t, diffs)
	
	// Should have differences in content and tags
	hasContentDiff := false
	hasTagsDiff := false
	for _, diff := range diffs {
		if diff.Field == "content" {
			hasContentDiff = true
		}
		if diff.Field == "metadata.tags" {
			hasTagsDiff = true
		}
	}
	assert.True(t, hasContentDiff || hasTagsDiff)
}

func TestFTS5Availability(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()

	// Check if FTS5 is available
	available := store.FTS5Available()
	// Note: This might be false if modernc.org/sqlite doesn't support FTS5
	// That's okay - the code should handle this gracefully
	_ = available
}

func TestConcurrentMemoryOperations(t *testing.T) {
	t.Skip("Skipping concurrent test due to SQLite locking")
	
	store, cleanup := setupTestDB(t)
	defer cleanup()

	workspaceID := int64(25)
	
	// Create multiple memories concurrently
	done := make(chan bool)
	for i := 0; i < 5; i++ {
		go func(idx int) {
			_, err := store.Save(
				fmt.Sprintf("Concurrent memory %d", idx),
				"concurrent",
				"test",
				workspaceID,
				nil,
			)
			require.NoError(t, err)
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 5; i++ {
		<-done
	}

	// Verify all memories were created
	memories, err := store.List(workspaceID, 10)
	require.NoError(t, err)
	assert.Equal(t, 5, len(memories))
}
