package main

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/gede-cahya/Smara-CLI/internal/config"
	"github.com/gede-cahya/Smara-CLI/internal/skill"
)

func TestPhase6LogSkillRunToDefaultTrackerSuccess(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "skill-history.db")
	oldDBPath := config.Get().DBPath
	oldApprove := skillRunApprove
	config.Get().DBPath = dbPath
	skillRunApprove = true
	t.Cleanup(func() {
		config.Get().DBPath = oldDBPath
		skillRunApprove = oldApprove
	})

	start := time.Now().Add(-25 * time.Millisecond)
	result := &skill.RunResult{
		SkillName: "phase6-success",
		Success:   true,
		Summary:   "ok",
		StepResults: []skill.StepResult{
			{Tool: "echo", Output: "done"},
		},
	}

	if err := logSkillRunToDefaultTracker("phase6-success", result, start); err != nil {
		t.Fatalf("logSkillRunToDefaultTracker error: %v", err)
	}

	tracker, closeFn, err := openDefaultSkillTracker()
	if err != nil {
		t.Fatalf("openDefaultSkillTracker error: %v", err)
	}
	defer closeFn()

	runs, err := tracker.GetTimeline("phase6-success", 10)
	if err != nil {
		t.Fatalf("GetTimeline error: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("runs len = %d, want 1", len(runs))
	}
	run := runs[0]
	if !run.Success || run.Status != "success" {
		t.Fatalf("run status = success:%v status:%q, want true/success", run.Success, run.Status)
	}
	if !run.ApprovalGranted {
		t.Fatalf("approval_granted = false, want true")
	}
	if run.DurationMs <= 0 {
		t.Fatalf("duration_ms = %d, want > 0", run.DurationMs)
	}
	if len(run.StepResults) != 1 || !run.StepResults[0].Success || run.StepResults[0].Tool != "echo" {
		t.Fatalf("unexpected step results: %#v", run.StepResults)
	}
}

func TestPhase6LogSkillRunToDefaultTrackerFailureAndStats(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "skill-history.db")
	oldDBPath := config.Get().DBPath
	oldApprove := skillRunApprove
	config.Get().DBPath = dbPath
	skillRunApprove = false
	t.Cleanup(func() {
		config.Get().DBPath = oldDBPath
		skillRunApprove = oldApprove
	})

	start := time.Now().Add(-50 * time.Millisecond)
	boom := errors.New("boom at step two")
	result := &skill.RunResult{
		SkillName: "phase6-failure",
		Success:   false,
		Summary:   "failed",
		StepResults: []skill.StepResult{
			{Tool: "first", Output: "ok"},
			{Tool: "second", Error: boom},
		},
	}

	if err := logSkillRunToDefaultTracker("phase6-failure", result, start); err != nil {
		t.Fatalf("logSkillRunToDefaultTracker error: %v", err)
	}

	tracker, closeFn, err := openDefaultSkillTracker()
	if err != nil {
		t.Fatalf("openDefaultSkillTracker error: %v", err)
	}
	defer closeFn()

	runs, err := tracker.GetTimeline("phase6-failure", 10)
	if err != nil {
		t.Fatalf("GetTimeline error: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("runs len = %d, want 1", len(runs))
	}
	run := runs[0]
	if run.Success || run.Status != "failed" {
		t.Fatalf("run status = success:%v status:%q, want false/failed", run.Success, run.Status)
	}
	if run.FailedStep != 2 {
		t.Fatalf("failed_step = %d, want 2", run.FailedStep)
	}
	if run.ErrorMessage != boom.Error() {
		t.Fatalf("error_message = %q, want %q", run.ErrorMessage, boom.Error())
	}
	if len(run.StepResults) != 2 || run.StepResults[1].Success || run.StepResults[1].Error != boom.Error() {
		t.Fatalf("unexpected step results: %#v", run.StepResults)
	}

	total, success, _, _, err := tracker.GetStats("phase6-failure")
	if err != nil {
		t.Fatalf("GetStats error: %v", err)
	}
	if total != 1 || success != 0 {
		t.Fatalf("stats total/success = %d/%d, want 1/0", total, success)
	}
}
