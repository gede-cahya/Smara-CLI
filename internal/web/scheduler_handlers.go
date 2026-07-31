package web

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/gede-cahya/Smara-CLI/internal/scheduler"
)

// handleSchedulerJobs handles GET/POST/DELETE for scheduled jobs.
func (s *Server) handleSchedulerJobs(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	switch r.Method {
	case http.MethodGet:
		jobs, err := scheduler.List()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"jobs": jobs})

	case http.MethodPost:
		var payload struct {
			Action      string `json:"action"`
			ID          string `json:"id"`
			Spec        string `json:"spec"`
			Workflow    string `json:"workflow"`
			Retries     int    `json:"retries"`
			IntervalSec int    `json:"retry_interval_sec"`
			DependsOn   string `json:"depends_on"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}

		if payload.Action == "run" {
			if payload.ID == "" {
				http.Error(w, "id is required", http.StatusBadRequest)
				return
			}
			jobs, _ := scheduler.List()
			var target *scheduler.Job
			for _, j := range jobs {
				if j.ID == payload.ID {
					target = j
					break
				}
			}
			if target == nil {
				http.Error(w, "job not found", http.StatusNotFound)
				return
			}
			_ = scheduler.MarkRun(target.ID, "success", time.Now())
			json.NewEncoder(w).Encode(map[string]interface{}{"status": "triggered", "id": target.ID})
			return
		}

		if payload.Spec == "" || payload.Workflow == "" {
			http.Error(w, "spec and workflow are required", http.StatusBadRequest)
			return
		}

		job, err := scheduler.Add(payload.Spec, payload.Workflow)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		if payload.Retries > 0 || payload.DependsOn != "" {
			job.MaxRetries = payload.Retries
			job.RetryIntervalSec = payload.IntervalSec
			job.DependsOn = payload.DependsOn
			jobs, _ := scheduler.List()
			for i, j := range jobs {
				if j.ID == job.ID {
					jobs[i] = job
					break
				}
			}
			_ = scheduler.SaveAll(jobs)
		}

		json.NewEncoder(w).Encode(map[string]interface{}{"status": "created", "job": job})

	case http.MethodDelete:
		id := r.URL.Query().Get("id")
		if id == "" {
			http.Error(w, "id is required", http.StatusBadRequest)
			return
		}
		if err := scheduler.Remove(id); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"status": "deleted", "id": id})

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}
