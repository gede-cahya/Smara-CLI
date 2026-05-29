package imageflow

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"time"
)

type QueueStats struct {
	Total     int `json:"total"`
	Queued    int `json:"queued"`
	Running   int `json:"running"`
	Success   int `json:"success"`
	Failed    int `json:"failed"`
	Canceled  int `json:"canceled"`
	MaxActive int `json:"max_active"`
}

type Metrics struct {
	Jobs        QueueStats `json:"jobs"`
	Assets      int        `json:"assets"`
	Archived    int        `json:"archived_assets"`
	TotalBytes  int64      `json:"total_asset_bytes"`
	AuditEvents int        `json:"audit_events"`
}

type AuditEvent map[string]interface{}

type persistedJobRecord struct {
	Job      Job      `json:"job"`
	Workflow Workflow `json:"workflow"`
	RetryOf  string   `json:"retry_of,omitempty"`
}

func init() {
	_ = RecoverJobs()
}

func ListJobs() []Job {
	jobs.Lock()
	defer jobs.Unlock()
	out := make([]Job, 0, len(jobs.items))
	for _, rec := range jobs.items {
		out = append(out, cloneJob(rec.job))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt > out[j].CreatedAt })
	return out
}

func JobQueueStats() QueueStats {
	stats := QueueStats{MaxActive: maxConcurrentJobs}
	jobs.Lock()
	defer jobs.Unlock()
	stats.Total = len(jobs.items)
	for _, rec := range jobs.items {
		switch rec.job.Status {
		case "queued":
			stats.Queued++
		case "running":
			stats.Running++
		case "success":
			stats.Success++
		case "failed":
			stats.Failed++
		case "canceled":
			stats.Canceled++
		}
	}
	return stats
}

func UsageMetrics() Metrics {
	metrics := Metrics{Jobs: JobQueueStats()}
	if assets, err := ListAssets(); err == nil {
		metrics.Assets = len(assets)
		for _, asset := range assets {
			if asset.Archived {
				metrics.Archived++
			}
			metrics.TotalBytes += asset.SizeBytes
		}
	}
	if events, err := ReadAuditEvents(10000); err == nil {
		metrics.AuditEvents = len(events)
	}
	return metrics
}

func ReadAuditEvents(limit int) ([]AuditEvent, error) {
	if limit <= 0 {
		limit = 100
	}
	file, err := os.Open(auditLogPath())
	if err != nil {
		if os.IsNotExist(err) {
			return []AuditEvent{}, nil
		}
		return nil, err
	}
	defer file.Close()
	events := []AuditEvent{}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var event AuditEvent
		if err := json.Unmarshal(scanner.Bytes(), &event); err == nil {
			events = append(events, event)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	sort.SliceStable(events, func(i, j int) bool {
		ti, _ := time.Parse(time.RFC3339, stringField(events[i], "time"))
		tj, _ := time.Parse(time.RFC3339, stringField(events[j], "time"))
		return ti.After(tj)
	})
	if len(events) > limit {
		events = events[:limit]
	}
	return events, nil
}

func RecoverJobs() error {
	data, err := os.ReadFile(jobStorePath())
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	var persisted []persistedJobRecord
	if err := json.Unmarshal(data, &persisted); err != nil {
		return err
	}
	now := time.Now().Format(time.RFC3339)
	toStart := []string{}
	jobs.Lock()
	for _, item := range persisted {
		if item.Job.ID == "" {
			continue
		}
		// Jobs that were running at process shutdown cannot be safely resumed mid-provider call.
		if item.Job.Status == "running" {
			item.Job.Status = "failed"
			item.Job.Error = "recovered after process restart; previous running job marked failed"
			item.Job.Logs = append(item.Job.Logs, "Crash recovery: previous running job was marked failed. Use retry to run again.")
			item.Job.UpdatedAt = now
		}
		jobs.items[item.Job.ID] = &jobRecord{job: item.Job, workflow: item.Workflow, retryOf: item.RetryOf}
	}
	for len(toStart) < maxConcurrentJobs {
		nextID := nextQueuedJobIDLocked(toStart)
		if nextID == "" {
			break
		}
		jobs.running[nextID] = true
		toStart = append(toStart, nextID)
	}
	_ = persistJobsLocked()
	jobs.Unlock()
	if len(persisted) > 0 {
		appendAudit("jobs_recovered", map[string]interface{}{"count": len(persisted), "auto_started": len(toStart)})
	}
	for _, id := range toStart {
		go runJob(id)
	}
	return nil
}

func persistJobsLocked() error {
	if err := os.MkdirAll(assetDir(), 0o755); err != nil {
		return err
	}
	out := make([]persistedJobRecord, 0, len(jobs.items))
	for _, rec := range jobs.items {
		out = append(out, persistedJobRecord{Job: cloneJob(rec.job), Workflow: rec.workflow, RetryOf: rec.retryOf})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Job.CreatedAt > out[j].Job.CreatedAt })
	if len(out) > 200 {
		out = out[:200]
	}
	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(jobStorePath(), data, 0o644)
}

func jobStorePath() string {
	return filepath.Join(assetDir(), "jobs.json")
}

func stringField(m map[string]interface{}, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}
