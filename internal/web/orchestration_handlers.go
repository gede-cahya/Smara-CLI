package web

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gede-cahya/Smara-CLI/internal/agent"
	"github.com/gede-cahya/Smara-CLI/internal/agent/workflow"
	"github.com/gede-cahya/Smara-CLI/internal/config"
)

// OrchestrationRunSnapshot is the UI-facing state for the latest/active parallel orchestration run.
type OrchestrationRunSnapshot struct {
	RunID     string                              `json:"run_id"`
	PlanID    string                              `json:"plan_id"`
	TaskTitle string                              `json:"task_title"`
	Status    workflow.SubtaskStatus              `json:"status"`
	StartedAt time.Time                           `json:"started_at,omitempty"`
	EndedAt   time.Time                           `json:"ended_at,omitempty"`
	Batches   []workflow.ExecutionBatch           `json:"batches"`
	Subtasks  []workflow.Subtask                  `json:"subtasks"`
	Results   map[string]workflow.ExecutionResult `json:"results,omitempty"`
	Report    string                              `json:"report,omitempty"`
	Error     string                              `json:"error,omitempty"`
	UpdatedAt time.Time                           `json:"updated_at"`
}

type orchestrationUIStatus struct {
	Active         bool                               `json:"active"`
	Status         string                             `json:"status"`
	RunID          string                             `json:"run_id"`
	PlanID         string                             `json:"plan_id,omitempty"`
	Title          string                             `json:"title"`
	StartedAt      time.Time                          `json:"started_at,omitempty"`
	EndedAt        time.Time                          `json:"ended_at,omitempty"`
	UpdatedAt      time.Time                          `json:"updated_at"`
	Config         config.ParallelOrchestrationConfig `json:"config"`
	Summary        orchestrationUISummary             `json:"summary"`
	Agents         []orchestrationUIAgent             `json:"agents"`
	Tasks          []orchestrationUISubtask           `json:"tasks"`
	Batches        []orchestrationUIBatch             `json:"batches"`
	Events         []orchestrationUIEvent             `json:"events"`
	ReportMarkdown string                             `json:"report_markdown,omitempty"`
	Error          string                             `json:"error,omitempty"`
}

type orchestrationUISummary struct {
	Total   int `json:"total"`
	Running int `json:"running"`
	Success int `json:"success"`
	Failed  int `json:"failed"`
	Skipped int `json:"skipped"`
	Gated   int `json:"gated"`
}
type orchestrationUIAgent struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Role          string `json:"role"`
	Status        string `json:"status"`
	CurrentTaskID string `json:"current_task_id,omitempty"`
	Completed     int    `json:"completed"`
	Total         int    `json:"total"`
}

type orchestrationUISubtask struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Description string   `json:"description,omitempty"`
	Status      string   `json:"status"`
	AgentID     string   `json:"agent_id"`
	Kind        string   `json:"kind,omitempty"`
	Risk        string   `json:"risk,omitempty"`
	DependsOn   []string `json:"depends_on,omitempty"`
	DurationMS  int64    `json:"duration_ms,omitempty"`
	Output      string   `json:"output,omitempty"`
	Error       string   `json:"error,omitempty"`
	Progress    int      `json:"progress"`
}

type orchestrationUIBatch struct {
	ID       string                   `json:"id"`
	Name     string                   `json:"name"`
	Mode     string                   `json:"mode"`
	Status   string                   `json:"status"`
	Subtasks []orchestrationUISubtask `json:"subtasks"`
}

type orchestrationUIEvent struct {
	ID      string    `json:"id"`
	Time    time.Time `json:"time"`
	AgentID string    `json:"agent_id,omitempty"`
	TaskID  string    `json:"task_id,omitempty"`
	Type    string    `json:"type"`
	Message string    `json:"message"`
	Status  string    `json:"status,omitempty"`
}

// OrchestrationStatusStore keeps the latest orchestration snapshot for the web UI.
type OrchestrationStatusStore struct {
	mu          sync.RWMutex
	snapshot    OrchestrationRunSnapshot
	subscribers map[chan OrchestrationRunSnapshot]struct{}
}

func NewOrchestrationStatusStore() *OrchestrationStatusStore {
	return &OrchestrationStatusStore{snapshot: OrchestrationRunSnapshot{Status: workflow.StatusPending, Batches: []workflow.ExecutionBatch{}, Subtasks: []workflow.Subtask{}, Results: map[string]workflow.ExecutionResult{}, UpdatedAt: time.Now()}, subscribers: map[chan OrchestrationRunSnapshot]struct{}{}}
}

func cloneOrchestrationSnapshot(src OrchestrationRunSnapshot) OrchestrationRunSnapshot {
	cp := src
	cp.Batches = append([]workflow.ExecutionBatch(nil), src.Batches...)
	cp.Subtasks = append([]workflow.Subtask(nil), src.Subtasks...)
	cp.Results = make(map[string]workflow.ExecutionResult, len(src.Results))
	for k, v := range src.Results {
		cp.Results[k] = v
	}
	return cp
}

func (s *OrchestrationStatusStore) Snapshot() OrchestrationRunSnapshot {
	if s == nil {
		return NewOrchestrationStatusStore().Snapshot()
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return cloneOrchestrationSnapshot(s.snapshot)
}

func (s *OrchestrationStatusStore) Subscribe() (<-chan OrchestrationRunSnapshot, func()) {
	ch := make(chan OrchestrationRunSnapshot, 8)
	if s == nil {
		close(ch)
		return ch, func() {}
	}
	s.mu.Lock()
	if s.subscribers == nil {
		s.subscribers = map[chan OrchestrationRunSnapshot]struct{}{}
	}
	s.subscribers[ch] = struct{}{}
	initial := cloneOrchestrationSnapshot(s.snapshot)
	s.mu.Unlock()
	ch <- initial
	return ch, func() {
		s.mu.Lock()
		if _, ok := s.subscribers[ch]; ok {
			delete(s.subscribers, ch)
			close(ch)
		}
		s.mu.Unlock()
	}
}

func (s *OrchestrationStatusStore) broadcastLocked() {
	snap := cloneOrchestrationSnapshot(s.snapshot)
	for ch := range s.subscribers {
		select {
		case ch <- snap:
		default:
		}
	}
}

func (s *OrchestrationStatusStore) Start(runID string, plan workflow.ExecutionPlan) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	s.snapshot = OrchestrationRunSnapshot{RunID: runID, PlanID: plan.ID, TaskTitle: plan.Task.Title, Status: workflow.StatusRunning, StartedAt: now, Batches: append([]workflow.ExecutionBatch(nil), plan.Batches...), Subtasks: append([]workflow.Subtask(nil), plan.Subtasks...), Results: map[string]workflow.ExecutionResult{}, UpdatedAt: now}
	s.broadcastLocked()
}

func (s *OrchestrationStatusStore) UpdateResult(result workflow.ExecutionResult) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.snapshot.Results == nil {
		s.snapshot.Results = map[string]workflow.ExecutionResult{}
	}
	s.snapshot.Results[result.SubtaskID] = result
	for i := range s.snapshot.Subtasks {
		if s.snapshot.Subtasks[i].ID == result.SubtaskID {
			s.snapshot.Subtasks[i].Status = result.Status
			break
		}
	}
	s.snapshot.UpdatedAt = time.Now()
	s.broadcastLocked()
}

func (s *OrchestrationStatusStore) UpdateSubtaskStatus(id string, status workflow.SubtaskStatus, output, errText string, duration time.Duration) {
	s.UpdateResult(workflow.ExecutionResult{SubtaskID: id, Status: status, Stdout: output, Error: errText, Duration: duration})
}

func (s *OrchestrationStatusStore) MarkAll(status workflow.SubtaskStatus, output string) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.snapshot.Results == nil {
		s.snapshot.Results = map[string]workflow.ExecutionResult{}
	}
	for i := range s.snapshot.Subtasks {
		st := &s.snapshot.Subtasks[i]
		if st.Status == workflow.StatusSuccess || st.Status == workflow.StatusFailed {
			continue
		}
		st.Status = status
		s.snapshot.Results[st.ID] = workflow.ExecutionResult{SubtaskID: st.ID, Status: status, Stdout: output}
	}
	s.snapshot.UpdatedAt = time.Now()
	s.broadcastLocked()
}

func (s *OrchestrationStatusStore) Complete(status workflow.SubtaskStatus, report string, errText string) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	s.snapshot.Status = status
	s.snapshot.Report = report
	s.snapshot.Error = errText
	s.snapshot.EndedAt = now
	s.snapshot.UpdatedAt = now
	s.broadcastLocked()
}

func workflowBlueprintExecutionPlan(runID, prompt string, bp workflow.Blueprint, waves [][]string) workflow.ExecutionPlan {
	roleSet := map[string]workflow.AgentSpec{}
	subtasks := make([]workflow.Subtask, 0, len(bp.Agents))
	dependencies := []workflow.Dependency{}
	for _, spec := range bp.Agents {
		role := strings.TrimSpace(spec.Role)
		if role == "" {
			continue
		}
		roleSet[role] = spec
	}
	for _, spec := range bp.Agents {
		role := strings.TrimSpace(spec.Role)
		if role == "" {
			continue
		}
		dependsOn := make([]string, 0, len(spec.DependsOn))
		for _, dep := range spec.DependsOn {
			dep = strings.TrimSpace(dep)
			if dep == "" {
				continue
			}
			if _, ok := roleSet[dep]; ok {
				dependsOn = append(dependsOn, dep)
				dependencies = append(dependencies, workflow.Dependency{FromID: dep, ToID: role, Type: workflow.DependencyRequires, Reason: "blueprint dependency"})
			}
		}
		subtasks = append(subtasks, workflow.Subtask{ID: role, Title: firstNonEmpty(spec.Description, role), Description: fmt.Sprintf("%d task(s) assigned to %s", len(spec.Tasks), role), Kind: workflow.TaskKindReadOnly, DependsOn: dependsOn, CanParallel: true, RiskLevel: workflow.RiskLow, Status: workflow.StatusPending})
	}
	batches := make([]workflow.ExecutionBatch, 0, len(waves))
	for i, wave := range waves {
		ids := make([]string, 0, len(wave))
		for _, role := range wave {
			if _, ok := roleSet[role]; ok {
				ids = append(ids, role)
			}
		}
		if len(ids) == 0 {
			continue
		}
		mode := workflow.BatchModeParallel
		if len(ids) == 1 {
			mode = workflow.BatchModeSerial
		}
		batches = append(batches, workflow.ExecutionBatch{ID: fmt.Sprintf("wave-%d", i+1), Name: fmt.Sprintf("Wave %d", i+1), Mode: mode, SubtaskIDs: ids, MaxConcurrency: len(ids)})
	}
	return workflow.ExecutionPlan{ID: "plan-" + runID, Task: workflow.OrchestrationTask{ID: runID, Title: firstNonEmpty(bp.ProjectName, prompt), Description: prompt, Kind: workflow.TaskKindReadOnly, RiskLevel: workflow.RiskLow}, Subtasks: subtasks, Dependencies: dependencies, Batches: batches, Metadata: map[string]interface{}{"source": "workflow_blueprint"}}
}

func taskResultStatus(result agent.TaskResult) workflow.SubtaskStatus {
	if result.Status == agent.TaskCompleted {
		return workflow.StatusSuccess
	}
	if result.Status == agent.TaskFailed {
		return workflow.StatusFailed
	}
	return workflow.StatusRunning
}

func (s *Server) runWorkflowWithLiveStatus(ctx context.Context, prompt string) (*workflow.WorkflowResult, error) {
	return s.runWorkflowWithLiveStatusAndProgress(ctx, prompt, nil)
}

func (s *Server) runWorkflowWithLiveStatusAndProgress(ctx context.Context, prompt string, onProgress func(step, status string)) (*workflow.WorkflowResult, error) {
	runID := fmt.Sprintf("web-%d", time.Now().UnixNano())
	projectDir := filepath.Join(os.TempDir(), fmt.Sprintf("smara-workflow-%s", runID))
	started := map[string]time.Time{}
	var progressMu sync.Mutex
	result, err := workflow.RunWorkflowWithDirAndSetupContext(ctx, s.Supervisor, s.Supervisor.GetProvider(), prompt, projectDir, func(orch *workflow.Orchestrator) {
		orch.OnProgress = onProgress
		orch.OnBlueprintReady = func(bp workflow.Blueprint, waves [][]string) {
			progressMu.Lock()
			defer progressMu.Unlock()
			s.OrchestrationStore.Start(runID, workflowBlueprintExecutionPlan(runID, prompt, bp, waves))
		}
		orch.OnRoleStart = func(role string) {
			progressMu.Lock()
			started[role] = time.Now()
			progressMu.Unlock()
			s.OrchestrationStore.UpdateSubtaskStatus(role, workflow.StatusRunning, "", "", 0)
		}
		orch.OnTaskComplete = func(role, taskID string, taskResult agent.TaskResult) {
			progressMu.Lock()
			duration := time.Duration(0)
			if start := started[role]; !start.IsZero() {
				duration = time.Since(start)
			}
			progressMu.Unlock()
			s.OrchestrationStore.UpdateSubtaskStatus(role, taskResultStatus(taskResult), strings.TrimSpace(taskResult.Output), taskResult.Error, duration)
		}
	})
	if err != nil {
		s.OrchestrationStore.Complete(workflow.StatusFailed, "", err.Error())
		return nil, err
	}
	report := "Workflow selesai tanpa ringkasan tambahan."
	if result != nil && strings.TrimSpace(result.FinalSummary) != "" {
		report = result.FinalSummary
	}
	s.OrchestrationStore.MarkAll(workflow.StatusSuccess, report)
	s.OrchestrationStore.Complete(workflow.StatusSuccess, report, "")
	return result, nil
}

func (srv *Server) handleOrchestrationStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		errorResponse(w, http.StatusMethodNotAllowed, "only GET")
		return
	}
	jsonResponse(w, http.StatusOK, buildOrchestrationUIStatus(srv.OrchestrationStore.Snapshot(), srv.Cfg.ParallelOrchestration))
}

func (srv *Server) handleOrchestrationEvents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		errorResponse(w, http.StatusMethodNotAllowed, "only GET")
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		errorResponse(w, http.StatusInternalServerError, "streaming unsupported")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	updates, unsubscribe := srv.OrchestrationStore.Subscribe()
	defer unsubscribe()
	enc := json.NewEncoder(w)
	for {
		select {
		case <-r.Context().Done():
			return
		case snap, ok := <-updates:
			if !ok {
				return
			}
			_, _ = w.Write([]byte("event: snapshot\ndata: "))
			if err := enc.Encode(buildOrchestrationUIStatus(snap, srv.Cfg.ParallelOrchestration)); err != nil {
				return
			}
			_, _ = w.Write([]byte("\n"))
			flusher.Flush()
		}
	}
}
func buildOrchestrationUIStatus(s OrchestrationRunSnapshot, cfg config.ParallelOrchestrationConfig) orchestrationUIStatus {
	tasks := make([]orchestrationUISubtask, 0, len(s.Subtasks))
	byID := map[string]orchestrationUISubtask{}
	agents := map[string]*orchestrationUIAgent{}
	events := []orchestrationUIEvent{}
	for _, st := range s.Subtasks {
		agentID, agentName, role := inferAgent(st)
		res := s.Results[st.ID]
		status := string(st.Status)
		if res.Status != "" {
			status = string(res.Status)
		}
		ui := orchestrationUISubtask{ID: st.ID, Title: st.Title, Description: st.Description, Status: status, AgentID: agentID, Kind: string(st.Kind), Risk: string(st.RiskLevel), DependsOn: st.DependsOn, DurationMS: int64(res.Duration / time.Millisecond), Output: strings.TrimSpace(res.Stdout), Error: firstNonEmpty(res.Error, res.Stderr), Progress: progressForStatus(status)}
		tasks = append(tasks, ui)
		byID[ui.ID] = ui
		a := agents[agentID]
		if a == nil {
			a = &orchestrationUIAgent{ID: agentID, Name: agentName, Role: role, Status: "idle"}
			agents[agentID] = a
		}
		a.Total++
		if status == "success" || status == "skipped" {
			a.Completed++
		}
		if status == "running" {
			a.Status = "running"
			a.CurrentTaskID = st.ID
		} else if a.Status == "idle" && status == "waiting_approval" {
			a.Status = "waiting"
		}
		events = append(events, orchestrationUIEvent{ID: "evt-" + st.ID, Time: s.UpdatedAt, AgentID: agentID, TaskID: st.ID, Type: "task", Message: st.Title, Status: status})
	}
	agentList := make([]orchestrationUIAgent, 0, len(agents))
	for _, a := range agents {
		if a.Status == "idle" && a.Total > 0 && a.Completed == a.Total {
			a.Status = "done"
		}
		agentList = append(agentList, *a)
	}
	sort.Slice(agentList, func(i, j int) bool { return agentList[i].ID < agentList[j].ID })
	batches := make([]orchestrationUIBatch, 0, len(s.Batches))
	summary := orchestrationUISummary{Total: len(tasks)}
	for _, t := range tasks {
		switch t.Status {
		case "running":
			summary.Running++
		case "success", "completed":
			summary.Success++
		case "failed", "error":
			summary.Failed++
		case "skipped":
			summary.Skipped++
		case "waiting_approval", "gated":
			summary.Gated++
		}
	}
	for _, b := range s.Batches {
		ub := orchestrationUIBatch{ID: b.ID, Name: b.Name, Mode: string(b.Mode), Status: batchStatus(b.SubtaskIDs, byID)}
		for _, id := range b.SubtaskIDs {
			if t, ok := byID[id]; ok {
				ub.Subtasks = append(ub.Subtasks, t)
			}
		}
		batches = append(batches, ub)
	}
	if s.RunID != "" {
		events = append([]orchestrationUIEvent{{ID: "evt-start", Time: s.StartedAt, Type: "run", Message: "Orchestration started", Status: string(s.Status)}}, events...)
	}
	title := s.TaskTitle
	if title == "" {
		title = "Belum ada orchestration aktif"
	}
	return orchestrationUIStatus{Active: s.Status == workflow.StatusRunning, Status: string(s.Status), RunID: s.RunID, PlanID: s.PlanID, Title: title, StartedAt: s.StartedAt, EndedAt: s.EndedAt, UpdatedAt: s.UpdatedAt, Config: cfg, Summary: summary, Agents: agentList, Tasks: tasks, Batches: batches, Events: events, ReportMarkdown: s.Report, Error: s.Error}
}

func inferAgent(st workflow.Subtask) (string, string, string) {
	k := string(st.Kind)
	if strings.Contains(k, "mutating") {
		return "coder", "Coder Agent", "Implementasi perubahan file/kode"
	}
	if strings.Contains(k, "destructive") || strings.Contains(k, "production") {
		return "safety", "Safety Agent", "Guardrail dan approval risiko"
	}
	if strings.Contains(k, "remote") {
		return "ops", "Ops Agent", "Eksekusi remote/server"
	}
	if strings.Contains(strings.ToLower(st.Title), "test") || strings.Contains(strings.ToLower(st.Title), "verify") {
		return "tester", "Tester Agent", "Build, test, dan verifikasi"
	}
	return "researcher", "Research Agent", "Discovery dan analisis read-only"
}
func progressForStatus(s string) int {
	switch s {
	case "success", "completed", "skipped":
		return 100
	case "running":
		return 60
	case "failed", "error":
		return 100
	case "waiting_approval", "gated":
		return 35
	default:
		return 0
	}
}
func batchStatus(ids []string, tasks map[string]orchestrationUISubtask) string {
	if len(ids) == 0 {
		return "pending"
	}
	running, failed, done := false, false, 0
	for _, id := range ids {
		t := tasks[id]
		if t.Status == "running" {
			running = true
		}
		if t.Status == "failed" || t.Status == "error" {
			failed = true
		}
		if t.Status == "success" || t.Status == "completed" || t.Status == "skipped" {
			done++
		}
	}
	if failed {
		return "failed"
	}
	if running {
		return "running"
	}
	if done == len(ids) {
		return "success"
	}
	return "pending"
}
func firstNonEmpty(v ...string) string {
	for _, s := range v {
		if strings.TrimSpace(s) != "" {
			return s
		}
	}
	return ""
}

func (srv *Server) handleOrchestrationConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		jsonResponse(w, http.StatusOK, srv.Cfg.ParallelOrchestration)
	case http.MethodPost:
		var next config.ParallelOrchestrationConfig
		if err := json.NewDecoder(r.Body).Decode(&next); err != nil {
			errorResponse(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		if next.MaxConcurrency < 1 {
			next.MaxConcurrency = 1
		}
		srv.Cfg.ParallelOrchestration = next
		config.SetParallelOrchestration(next)
		jsonResponse(w, http.StatusOK, next)
	default:
		errorResponse(w, http.StatusMethodNotAllowed, "only GET/POST")
	}
}
