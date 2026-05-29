package workflow

import (
	"strings"
	"testing"
	"time"
)

func TestReportAggregatorSummarizesExecution(t *testing.T) {
	plan := ExecutionPlan{
		ID:   "plan-1",
		Task: OrchestrationTask{Title: "Implement feature"},
		Subtasks: []Subtask{
			{ID: "a", Title: "Analyze", Status: StatusPending, RiskLevel: RiskLow},
			{ID: "b", Title: "Apply", Status: StatusPending, RiskLevel: RiskHigh, DependsOn: []string{"a"}},
			{ID: "c", Title: "Verify", Status: StatusPending, RiskLevel: RiskLow, DependsOn: []string{"b"}},
		},
	}
	result := PlanExecutionResult{
		PlanID: "plan-1",
		Status: StatusFailed,
		Results: map[string]ExecutionResult{
			"a": {SubtaskID: "a", Status: StatusSuccess, Duration: time.Millisecond},
			"b": {SubtaskID: "b", Status: StatusFailed, Error: "edit failed", Duration: 2 * time.Millisecond},
			"c": {SubtaskID: "c", Status: StatusSkipped, Error: "dependency failed", Duration: time.Millisecond},
		},
		Duration: 4 * time.Millisecond,
		Metadata: map[string]interface{}{"execution_id": "exec-123"},
	}

	report := NewReportAggregator().Aggregate(plan, result)
	if report.ExecutionID != "exec-123" {
		t.Fatalf("expected execution ID from metadata, got %q", report.ExecutionID)
	}
	if report.Summary.Total != 3 || report.Summary.Success != 1 || report.Summary.Failed != 1 || report.Summary.Skipped != 1 {
		t.Fatalf("unexpected summary: %+v", report.Summary)
	}
	if len(report.Items) != 2 {
		t.Fatalf("expected 2 notable items, got %d", len(report.Items))
	}
	if report.Items[0].SubtaskID != "b" || report.Items[0].Severity != ReportSeverityCritical {
		t.Fatalf("expected high-risk failure to be critical, got %+v", report.Items[0])
	}
	if len(report.NextSteps) < 2 {
		t.Fatalf("expected remediation next steps, got %+v", report.NextSteps)
	}
}

func TestAggregatedReportMarkdown(t *testing.T) {
	report := AggregatedReport{
		ExecutionID: "exec-plan-1",
		PlanID:      "plan-1",
		TaskTitle:   "Read repo",
		Status:      StatusSuccess,
		Summary:     ProgressSummary{Total: 1, Success: 1},
		NextSteps:   []string{"Semua subtask selesai sukses; lanjutkan."},
		Duration:    time.Second,
	}

	md := report.Markdown()
	for _, want := range []string{"# Orchestration Report", "Execution ID", "Status: **success**", "Success: 1", "Next Steps"} {
		if !strings.Contains(md, want) {
			t.Fatalf("markdown missing %q:\n%s", want, md)
		}
	}
}

func TestReportAggregatorAddsSuccessNextStep(t *testing.T) {
	plan := ExecutionPlan{ID: "plan-ok", Subtasks: []Subtask{{ID: "a", RiskLevel: RiskLow}}}
	result := PlanExecutionResult{
		PlanID: "plan-ok",
		Status: StatusSuccess,
		Results: map[string]ExecutionResult{
			"a": {SubtaskID: "a", Status: StatusSuccess},
		},
	}

	report := NewReportAggregator().Aggregate(plan, result)
	if len(report.NextSteps) != 1 || !strings.Contains(report.NextSteps[0], "selesai sukses") {
		t.Fatalf("expected success next step, got %+v", report.NextSteps)
	}
}
