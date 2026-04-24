package metrics

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// WriteMetrics atomically writes metrics to a JSON file.
// It writes to a temp file first, then renames to avoid partial reads.
func WriteMetrics(path string, m *Metrics) error {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Errorf("gagal marshal metrics: %w", err)
	}

	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".metrics-*.tmp")
	if err != nil {
		return fmt.Errorf("gagal buat temp file: %w", err)
	}
	tmpName := tmp.Name()

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return fmt.Errorf("gagal tulis temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return err
	}

	if err := os.Rename(tmpName, path); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("gagal rename metrics file: %w", err)
	}

	return nil
}
