package imageflow

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

type cacheEntry struct {
	Key       string    `json:"key"`
	CreatedAt time.Time `json:"created_at"`
	Workflow  Workflow  `json:"workflow"`
	Result    RunResult `json:"result"`
}

func cachedRunResult(wf *Workflow) (*RunResult, bool) {
	key, err := workflowCacheKey(wf)
	if err != nil {
		return nil, false
	}
	data, err := os.ReadFile(filepath.Join(cacheDir(), key+".json"))
	if err != nil {
		return nil, false
	}
	var entry cacheEntry
	if err := json.Unmarshal(data, &entry); err != nil || entry.Key != key {
		return nil, false
	}
	result := entry.Result
	result.Cached = true
	result.Logs = append([]string{"Cache hit: reused intermediate output for identical workflow."}, result.Logs...)
	return &result, true
}

func saveRunResultCache(wf *Workflow, result *RunResult) {
	if wf == nil || result == nil || result.Status != "success" {
		return
	}
	key, err := workflowCacheKey(wf)
	if err != nil {
		return
	}
	_ = os.MkdirAll(cacheDir(), 0o755)
	copyResult := *result
	copyResult.Cached = false
	entry := cacheEntry{Key: key, CreatedAt: time.Now(), Workflow: *wf, Result: copyResult}
	data, err := json.MarshalIndent(entry, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(filepath.Join(cacheDir(), key+".json"), data, 0o644)
}

func workflowCacheKey(wf *Workflow) (string, error) {
	clone := *wf
	clone.Metadata = Metadata{}
	data, err := json.Marshal(clone)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

func cacheDir() string {
	return filepath.Join(dir(), "cache")
}
