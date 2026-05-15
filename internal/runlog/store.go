package runlog

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/gede-cahya/Smara-CLI/internal/config"
)

type Run struct {
	ID        string            `json:"id"`
	Kind      string            `json:"kind"`
	Name      string            `json:"name"`
	Status    string            `json:"status"`
	StartedAt time.Time         `json:"started_at"`
	UpdatedAt time.Time         `json:"updated_at"`
	EndedAt   *time.Time        `json:"ended_at,omitempty"`
	Provider  string            `json:"provider,omitempty"`
	Model     string            `json:"model,omitempty"`
	Project   string            `json:"project,omitempty"`
	Summary   string            `json:"summary,omitempty"`
	Error     string            `json:"error,omitempty"`
	Metadata  map[string]string `json:"metadata,omitempty"`
	Events    []Event           `json:"events"`
}

type Event struct {
	Time    time.Time         `json:"time"`
	Type    string            `json:"type"`
	Message string            `json:"message"`
	Data    map[string]string `json:"data,omitempty"`
}

type StartOptions struct {
	Kind     string
	Name     string
	Provider string
	Model    string
	Project  string
	Metadata map[string]string
}

func Start(opts StartOptions) (*Run, error) {
	now := time.Now()
	r := &Run{
		ID:        newID(),
		Kind:      opts.Kind,
		Name:      opts.Name,
		Status:    "running",
		StartedAt: now,
		UpdatedAt: now,
		Provider:  opts.Provider,
		Model:     opts.Model,
		Project:   opts.Project,
		Metadata:  opts.Metadata,
		Events: []Event{{
			Time:    now,
			Type:    "start",
			Message: fmt.Sprintf("started %s %s", opts.Kind, opts.Name),
		}},
	}
	return r, Save(r)
}

func AddEvent(id, typ, message string, data map[string]string) error {
	r, err := Load(id)
	if err != nil {
		return err
	}
	r.Events = append(r.Events, Event{Time: time.Now(), Type: typ, Message: message, Data: data})
	r.UpdatedAt = time.Now()
	return Save(r)
}

func Finish(id, status, summary string, runErr error) error {
	r, err := Load(id)
	if err != nil {
		return err
	}
	now := time.Now()
	r.Status = status
	r.Summary = summary
	r.UpdatedAt = now
	r.EndedAt = &now
	msg := summary
	if runErr != nil {
		r.Error = runErr.Error()
		msg = runErr.Error()
	}
	r.Events = append(r.Events, Event{Time: now, Type: status, Message: msg})
	return Save(r)
}

func Save(r *Run) error {
	if err := ensureDir(); err != nil {
		return err
	}
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return fmt.Errorf("gagal marshal run log: %w", err)
	}
	path := filePath(r.ID)
	tmp, err := os.CreateTemp(dir(), ".run-*.tmp")
	if err != nil {
		return fmt.Errorf("gagal buat temp run log: %w", err)
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return fmt.Errorf("gagal tulis run log: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("gagal simpan run log: %w", err)
	}
	return nil
}

func Load(id string) (*Run, error) {
	data, err := os.ReadFile(filePath(id))
	if err != nil {
		return nil, fmt.Errorf("run '%s' tidak ditemukan: %w", id, err)
	}
	var r Run
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, fmt.Errorf("gagal parse run log: %w", err)
	}
	return &r, nil
}

func List(limit int) ([]*Run, error) {
	if err := ensureDir(); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir())
	if err != nil {
		return nil, err
	}
	var runs []*Run
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		id := strings.TrimSuffix(entry.Name(), ".json")
		r, err := Load(id)
		if err == nil {
			runs = append(runs, r)
		}
	}
	sort.Slice(runs, func(i, j int) bool {
		return runs[i].StartedAt.After(runs[j].StartedAt)
	})
	if limit > 0 && len(runs) > limit {
		runs = runs[:limit]
	}
	return runs, nil
}

func dir() string {
	cfg := config.Get()
	return filepath.Join(filepath.Dir(cfg.DBPath), "runs")
}

func ensureDir() error {
	return os.MkdirAll(dir(), 0755)
}

func filePath(id string) string {
	safe := strings.NewReplacer("/", "_", "\\", "_", "..", "_").Replace(id)
	return filepath.Join(dir(), safe+".json")
}

func newID() string {
	var b [6]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("run-%d", time.Now().UnixNano())
	}
	return fmt.Sprintf("run-%s", hex.EncodeToString(b[:]))
}
