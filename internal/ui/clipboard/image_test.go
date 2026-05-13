package clipboard

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestSmaraTempImagePath verifies the temp dir is created and accessible.
func TestSmaraTempImagePath(t *testing.T) {
	dir, err := SmaraTempImagePath()
	if err != nil {
		t.Fatalf("SmaraTempImagePath: %v", err)
	}
	if dir == "" {
		t.Fatal("expected non-empty path")
	}
	st, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("temp dir not accessible: %v", err)
	}
	if !st.IsDir() {
		t.Errorf("expected directory, got file")
	}
}

// TestPruneOldClipImages confirms the housekeeping logic removes stale files.
func TestPruneOldClipImages(t *testing.T) {
	dir := t.TempDir()

	// Create a stale file (2h old).
	stale := filepath.Join(dir, "stale.png")
	if err := os.WriteFile(stale, []byte("stale"), 0644); err != nil {
		t.Fatal(err)
	}
	twoHoursAgo := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(stale, twoHoursAgo, twoHoursAgo); err != nil {
		t.Fatal(err)
	}

	// Create a fresh file.
	fresh := filepath.Join(dir, "fresh.png")
	if err := os.WriteFile(fresh, []byte("fresh"), 0644); err != nil {
		t.Fatal(err)
	}

	pruneOldClipImages(dir)

	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Errorf("stale file should have been removed")
	}
	if _, err := os.Stat(fresh); err != nil {
		t.Errorf("fresh file should still exist: %v", err)
	}
}

// TestPruneClipsKeeps50 verifies the 50-file ceiling.
func TestPruneClipsKeeps50(t *testing.T) {
	dir := t.TempDir()

	// Create 60 files with staggered timestamps (within 1 hour so age won't trim them).
	for i := 0; i < 60; i++ {
		p := filepath.Join(dir, "f"+itoa(i)+".png")
		if err := os.WriteFile(p, []byte("x"), 0644); err != nil {
			t.Fatal(err)
		}
		mt := time.Now().Add(-time.Duration(i) * time.Minute)
		_ = os.Chtimes(p, mt, mt)
	}

	pruneOldClipImages(dir)

	entries, _ := os.ReadDir(dir)
	if len(entries) > 50 {
		t.Errorf("expected ≤50 files, got %d", len(entries))
	}
}

// Tiny inline itoa to keep this test self-contained without strconv import.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
