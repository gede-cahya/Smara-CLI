package scheduler

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

type Job struct {
	ID          string     `json:"id"`
	Spec        string     `json:"spec"`
	Workflow    string     `json:"workflow"`
	Enabled     bool       `json:"enabled"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	NextRunAt   time.Time  `json:"next_run_at"`
	LastRunAt   *time.Time `json:"last_run_at,omitempty"`
	LastStatus  string     `json:"last_status,omitempty"`
	IntervalSec int64      `json:"interval_sec,omitempty"`
	DailyAt     string     `json:"daily_at,omitempty"`
}

func Add(spec, workflow string) (*Job, error) {
	job := &Job{ID: newID(), Spec: spec, Workflow: workflow, Enabled: true, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	if err := applySpec(job, time.Now()); err != nil {
		return nil, err
	}
	jobs, err := List()
	if err != nil {
		return nil, err
	}
	jobs = append(jobs, job)
	return job, SaveAll(jobs)
}

func Remove(id string) error {
	jobs, err := List()
	if err != nil {
		return err
	}
	filtered := jobs[:0]
	found := false
	for _, job := range jobs {
		if job.ID == id {
			found = true
			continue
		}
		filtered = append(filtered, job)
	}
	if !found {
		return fmt.Errorf("schedule '%s' tidak ditemukan", id)
	}
	return SaveAll(filtered)
}

func List() ([]*Job, error) {
	if err := ensureDir(); err != nil {
		return nil, err
	}
	path := filePath()
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return []*Job{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("gagal baca schedule: %w", err)
	}
	var jobs []*Job
	if err := json.Unmarshal(data, &jobs); err != nil {
		return nil, fmt.Errorf("gagal parse schedule: %w", err)
	}
	sort.Slice(jobs, func(i, j int) bool { return jobs[i].NextRunAt.Before(jobs[j].NextRunAt) })
	return jobs, nil
}

func SaveAll(jobs []*Job) error {
	if err := ensureDir(); err != nil {
		return err
	}
	for _, job := range jobs {
		job.UpdatedAt = time.Now()
	}
	data, err := json.MarshalIndent(jobs, "", "  ")
	if err != nil {
		return fmt.Errorf("gagal marshal schedule: %w", err)
	}
	tmp, err := os.CreateTemp(dir(), ".schedules-*.tmp")
	if err != nil {
		return fmt.Errorf("gagal buat temp schedule: %w", err)
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return fmt.Errorf("gagal tulis schedule: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	if err := os.Rename(tmpName, filePath()); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("gagal simpan schedule: %w", err)
	}
	return nil
}

func Due(now time.Time) ([]*Job, error) {
	jobs, err := List()
	if err != nil {
		return nil, err
	}
	var due []*Job
	for _, job := range jobs {
		if job.Enabled && !job.NextRunAt.After(now) {
			due = append(due, job)
		}
	}
	return due, nil
}

func MarkRun(id, status string, ranAt time.Time) error {
	jobs, err := List()
	if err != nil {
		return err
	}
	for _, job := range jobs {
		if job.ID != id {
			continue
		}
		job.LastRunAt = &ranAt
		job.LastStatus = status
		if err := applySpec(job, ranAt); err != nil {
			return err
		}
		return SaveAll(jobs)
	}
	return fmt.Errorf("schedule '%s' tidak ditemukan", id)
}

func applySpec(job *Job, from time.Time) error {
	spec := strings.TrimSpace(strings.ToLower(job.Spec))
	if strings.HasPrefix(spec, "every ") {
		d, err := time.ParseDuration(strings.TrimSpace(strings.TrimPrefix(spec, "every ")))
		if err != nil {
			return fmt.Errorf("format interval tidak valid: %w", err)
		}
		if d < time.Minute {
			return fmt.Errorf("interval minimal 1m")
		}
		job.IntervalSec = int64(d.Seconds())
		job.DailyAt = ""
		job.NextRunAt = from.Add(d)
		return nil
	}
	if strings.HasPrefix(spec, "daily ") {
		clock := strings.TrimSpace(strings.TrimPrefix(spec, "daily "))
		parsed, err := time.Parse("15:04", clock)
		if err != nil {
			return fmt.Errorf("format daily harus HH:MM")
		}
		next := time.Date(from.Year(), from.Month(), from.Day(), parsed.Hour(), parsed.Minute(), 0, 0, from.Location())
		if !next.After(from) {
			next = next.Add(24 * time.Hour)
		}
		job.IntervalSec = 0
		job.DailyAt = clock
		job.NextRunAt = next
		return nil
	}
	return fmt.Errorf("spec harus berbentuk 'every 15m' atau 'daily 09:00'")
}

func dir() string {
	cfg := config.Get()
	return filepath.Join(filepath.Dir(cfg.DBPath), "schedules")
}

func ensureDir() error {
	return os.MkdirAll(dir(), 0755)
}

func filePath() string {
	return filepath.Join(dir(), "workflows.json")
}

func newID() string {
	var b [5]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("sch-%d", time.Now().UnixNano())
	}
	return "sch-" + hex.EncodeToString(b[:])
}
