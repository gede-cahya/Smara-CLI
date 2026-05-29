package workflow

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// ReportSeverity classifies how urgently a report item needs attention.
type ReportSeverity string

const (
	ReportSeverityInfo     ReportSeverity = "info"
	ReportSeverityWarning  ReportSeverity = "warning"
	ReportSeverityError    ReportSeverity = "error"
	ReportSeverityCritical ReportSeverity = "critical"
)

// ReportItem represents one notable subtask outcome in the aggregated report.
type ReportItem struct {
	SubtaskID string         `json:"subtask_id"`
	Title     string         `json:"title,omitempty"`
	Status    SubtaskStatus  `json:"status"`
	Severity  ReportSeverity `json:"severity"`
	Message   string         `json:"message,omitempty"`
	Duration  time.Duration  `json:"duration,omitempty"`
}

// ProgressSummary contains status counters for a plan execution.
type ProgressSummary struct {
	Total     int `json:"total"`
	Success   int `json:"success"`
	Failed    int `json:"failed"`
	Skipped   int `json:"skipped"`
	Cancelled int `json:"cancelled"`
	Pending   int `json:"pending"`
	Running   int `json:"running"`
	Waiting   int `json:"waiting_approval"`
}

// AggregatedReport is the final human/UI friendly orchestration report.
type AggregatedReport struct {
	ExecutionID string                 `json:"execution_id"`
	PlanID      string                 `json:"plan_id"`
	TaskTitle   string                 `json:"task_title,omitempty"`
	Status      SubtaskStatus          `json:"status"`
	Summary     ProgressSummary        `json:"summary"`
	Items       []ReportItem           `json:"items"`
	NextSteps   []string               `json:"next_steps"`
	StartedAt   time.Time              `json:"started_at"`
	EndedAt     time.Time              `json:"ended_at"`
	Duration    time.Duration          `json:"duration"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

// ReportAggregator builds concise reports from execution results.
type ReportAggregator struct{}

// NewReportAggregator creates an orchestration report aggregator.
func NewReportAggregator() *ReportAggregator {
	return &ReportAggregator{}
}

// Aggregate combines plan metadata and execution result into a final report.
func (a *ReportAggregator) Aggregate(plan ExecutionPlan, result PlanExecutionResult) AggregatedReport {
	subtasks := subtaskMap(plan.Subtasks)
	ids := make([]string, 0, len(result.Results))
	for id := range result.Results {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	summary := ProgressSummary{Total: len(plan.Subtasks)}
	items := make([]ReportItem, 0, len(ids))
	for _, id := range ids {
		res := result.Results[id]
		countStatus(&summary, res.Status)
		st := subtasks[id]
		if shouldIncludeReportItem(res.Status) {
			items = append(items, ReportItem{
				SubtaskID: id,
				Title:     st.Title,
				Status:    res.Status,
				Severity:  severityForStatus(res.Status, st.RiskLevel),
				Message:   firstReportText(res.Error, res.Stderr, res.Stdout),
				Duration:  res.Duration,
			})
		}
	}

	status := result.Status
	if status == "" {
		status = summarizePlanStatus(result.Results)
	}

	report := AggregatedReport{
		ExecutionID: executionID(result),
		PlanID:      firstReportText(result.PlanID, plan.ID),
		TaskTitle:   plan.Task.Title,
		Status:      status,
		Summary:     summary,
		Items:       items,
		NextSteps:   buildNextSteps(status, summary, items),
		StartedAt:   result.StartedAt,
		EndedAt:     result.EndedAt,
		Duration:    result.Duration,
		Metadata:    map[string]interface{}{"report_version": "phase-6"},
	}
	return report
}

// Markdown renders a concise user-facing final report.
func (r AggregatedReport) Markdown() string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Orchestration Report\n\n")
	fmt.Fprintf(&b, "- Execution ID: `%s`\n", r.ExecutionID)
	fmt.Fprintf(&b, "- Plan ID: `%s`\n", r.PlanID)
	if r.TaskTitle != "" {
		fmt.Fprintf(&b, "- Task: %s\n", r.TaskTitle)
	}
	fmt.Fprintf(&b, "- Status: **%s**\n", r.Status)
	fmt.Fprintf(&b, "- Duration: %s\n\n", r.Duration)
	fmt.Fprintf(&b, "## Summary\n\n")
	fmt.Fprintf(&b, "- Total: %d\n", r.Summary.Total)
	fmt.Fprintf(&b, "- Success: %d\n", r.Summary.Success)
	fmt.Fprintf(&b, "- Failed: %d\n", r.Summary.Failed)
	fmt.Fprintf(&b, "- Skipped: %d\n", r.Summary.Skipped)
	fmt.Fprintf(&b, "- Cancelled: %d\n", r.Summary.Cancelled)
	if len(r.Items) > 0 {
		fmt.Fprintf(&b, "\n## Notable Items\n\n")
		for _, item := range r.Items {
			label := item.SubtaskID
			if item.Title != "" {
				label = fmt.Sprintf("%s (%s)", item.Title, item.SubtaskID)
			}
			fmt.Fprintf(&b, "- **%s** — %s/%s", label, item.Status, item.Severity)
			if item.Message != "" {
				fmt.Fprintf(&b, ": %s", item.Message)
			}
			fmt.Fprintf(&b, "\n")
		}
	}
	if len(r.NextSteps) > 0 {
		fmt.Fprintf(&b, "\n## Next Steps\n\n")
		for _, step := range r.NextSteps {
			fmt.Fprintf(&b, "- %s\n", step)
		}
	}
	return strings.TrimSpace(b.String())
}

func countStatus(summary *ProgressSummary, status SubtaskStatus) {
	switch status {
	case StatusSuccess:
		summary.Success++
	case StatusFailed:
		summary.Failed++
	case StatusSkipped:
		summary.Skipped++
	case StatusCancelled:
		summary.Cancelled++
	case StatusPending:
		summary.Pending++
	case StatusRunning:
		summary.Running++
	case StatusWaitingApproval:
		summary.Waiting++
	}
}

func shouldIncludeReportItem(status SubtaskStatus) bool {
	return status == StatusFailed || status == StatusSkipped || status == StatusCancelled || status == StatusWaitingApproval
}

func severityForStatus(status SubtaskStatus, risk RiskLevel) ReportSeverity {
	if status == StatusFailed && (risk == RiskCritical || risk == RiskHigh) {
		return ReportSeverityCritical
	}
	switch status {
	case StatusFailed:
		return ReportSeverityError
	case StatusSkipped, StatusCancelled, StatusWaitingApproval:
		return ReportSeverityWarning
	default:
		return ReportSeverityInfo
	}
}

func buildNextSteps(status SubtaskStatus, summary ProgressSummary, items []ReportItem) []string {
	steps := []string{}
	if summary.Failed > 0 {
		steps = append(steps, "Perbaiki subtask yang gagal lalu jalankan ulang verification batch.")
	}
	if summary.Skipped > 0 {
		steps = append(steps, "Review dependency yang menyebabkan subtask di-skip.")
	}
	if summary.Cancelled > 0 {
		steps = append(steps, "Jalankan ulang task yang cancelled setelah penyebab cancel selesai.")
	}
	if summary.Waiting > 0 {
		steps = append(steps, "Berikan approval eksplisit untuk subtask high/critical risk jika aman.")
	}
	if len(steps) == 0 && status == StatusSuccess {
		steps = append(steps, "Semua subtask selesai sukses; lanjutkan ke phase berikutnya atau final verification.")
	}
	if len(steps) == 0 && len(items) > 0 {
		steps = append(steps, "Review notable items sebelum melanjutkan.")
	}
	return steps
}

func executionID(result PlanExecutionResult) string {
	if result.Metadata != nil {
		if id, ok := result.Metadata["execution_id"].(string); ok && id != "" {
			return id
		}
	}
	if result.PlanID != "" {
		return fmt.Sprintf("exec-%s", result.PlanID)
	}
	return "exec-unknown"
}

func firstReportText(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
