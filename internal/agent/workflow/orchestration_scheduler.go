package workflow

import (
	"fmt"
	"sort"
)

// SchedulerConfig controls how an execution plan is grouped into batches.
type SchedulerConfig struct {
	// MaxConcurrency caps parallel batch workers. Values <= 0 default to the number of subtasks in the batch.
	MaxConcurrency int
}

// Scheduler converts a validated execution plan into deterministic execution batches.
type Scheduler struct {
	config SchedulerConfig
}

// NewScheduler creates a scheduler with safe defaults.
func NewScheduler(config SchedulerConfig) *Scheduler {
	return &Scheduler{config: config}
}

// Schedule validates a plan and populates deterministic execution batches.
func (s *Scheduler) Schedule(plan ExecutionPlan) (ExecutionPlan, error) {
	if err := ValidateExecutionPlan(plan); err != nil {
		return ExecutionPlan{}, err
	}

	waves, err := BuildExecutionWaves(plan)
	if err != nil {
		return ExecutionPlan{}, err
	}

	batches := make([]ExecutionBatch, 0, len(waves))
	for i, wave := range waves {
		batch := s.batchFromWave(i+1, wave, plan)
		batches = append(batches, batch)
	}

	plan.Batches = batches
	return plan, nil
}

// BuildExecutionWaves returns topological waves of subtask IDs. Each wave contains only
// subtasks whose dependencies are satisfied by previous waves. IDs are sorted for determinism.
func BuildExecutionWaves(plan ExecutionPlan) ([][]string, error) {
	if err := ValidateExecutionPlan(plan); err != nil {
		return nil, err
	}

	remaining := map[string]Subtask{}
	done := map[string]bool{}
	for _, st := range plan.Subtasks {
		remaining[st.ID] = st
	}

	waves := [][]string{}
	for len(remaining) > 0 {
		ready := []string{}
		ids := make([]string, 0, len(remaining))
		for id := range remaining {
			ids = append(ids, id)
		}
		sort.Strings(ids)

		for _, id := range ids {
			st := remaining[id]
			depsSatisfied := true
			for _, dep := range st.DependsOn {
				if !done[dep] {
					depsSatisfied = false
					break
				}
			}
			if depsSatisfied {
				ready = append(ready, id)
			}
		}

		if len(ready) == 0 {
			return nil, fmt.Errorf("circular dependency detected among subtasks")
		}

		waves = append(waves, ready)
		for _, id := range ready {
			done[id] = true
			delete(remaining, id)
		}
	}

	return waves, nil
}

func (s *Scheduler) batchFromWave(number int, subtaskIDs []string, plan ExecutionPlan) ExecutionBatch {
	mode := BatchModeParallel
	requiresApproval := false

	for _, id := range subtaskIDs {
		st, ok := findScheduledSubtask(plan.Subtasks, id)
		if !ok {
			continue
		}
		if !st.CanParallel {
			mode = BatchModeSerial
		}
		if st.Status == StatusWaitingApproval || st.RiskLevel == RiskHigh || st.RiskLevel == RiskCritical {
			mode = BatchModeGated
			requiresApproval = true
		}
	}

	maxConcurrency := len(subtaskIDs)
	if mode != BatchModeParallel {
		maxConcurrency = 1
	}
	if s.config.MaxConcurrency > 0 && maxConcurrency > s.config.MaxConcurrency {
		maxConcurrency = s.config.MaxConcurrency
	}
	if maxConcurrency < 1 {
		maxConcurrency = 1
	}

	return ExecutionBatch{
		ID:               fmt.Sprintf("batch-%02d", number),
		Name:             fmt.Sprintf("Batch %d", number),
		Mode:             mode,
		SubtaskIDs:       append([]string{}, subtaskIDs...),
		MaxConcurrency:   maxConcurrency,
		RequiresApproval: requiresApproval,
		Metadata: map[string]interface{}{
			"wave": number,
		},
	}
}

func findScheduledSubtask(subtasks []Subtask, id string) (Subtask, bool) {
	for _, st := range subtasks {
		if st.ID == id {
			return st, true
		}
	}
	return Subtask{}, false
}
