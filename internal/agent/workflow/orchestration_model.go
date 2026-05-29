package workflow

import "time"

// RiskLevel describes the potential impact of executing a task or subtask.
type RiskLevel string

const (
	RiskLow      RiskLevel = "low"
	RiskMedium   RiskLevel = "medium"
	RiskHigh     RiskLevel = "high"
	RiskCritical RiskLevel = "critical"
)

// SubtaskStatus represents the lifecycle state of a subtask during orchestration.
type SubtaskStatus string

const (
	StatusPending         SubtaskStatus = "pending"
	StatusRunning         SubtaskStatus = "running"
	StatusSuccess         SubtaskStatus = "success"
	StatusFailed          SubtaskStatus = "failed"
	StatusSkipped         SubtaskStatus = "skipped"
	StatusCancelled       SubtaskStatus = "cancelled"
	StatusWaitingApproval SubtaskStatus = "waiting_approval"
)

// TaskKind classifies a task so the planner/scheduler can apply safe execution rules.
type TaskKind string

const (
	TaskKindReadOnly            TaskKind = "read_only"
	TaskKindMutating            TaskKind = "mutating"
	TaskKindDestructive         TaskKind = "destructive"
	TaskKindRemote              TaskKind = "remote"
	TaskKindProductionImpacting TaskKind = "production_impacting"
)

// DependencyType describes why a subtask depends on another subtask.
type DependencyType string

const (
	DependencyRequires DependencyType = "requires"
	DependencyBlocks   DependencyType = "blocks"
	DependencyAfter    DependencyType = "after"
)

// BatchMode describes how a batch should be executed.
type BatchMode string

const (
	BatchModeParallel BatchMode = "parallel"
	BatchModeSerial   BatchMode = "serial"
	BatchModeGated    BatchMode = "gated"
)

// RetryPolicy defines retry behavior for a subtask execution.
type RetryPolicy struct {
	MaxAttempts int           `json:"max_attempts"`
	Backoff     time.Duration `json:"backoff"`
}

// OrchestrationTask is the top-level user request normalized for orchestration.
// It is intentionally separate from blueprint.go Task to avoid breaking the
// existing agent blueprint contract.
type OrchestrationTask struct {
	ID          string                 `json:"id"`
	Title       string                 `json:"title"`
	Description string                 `json:"description"`
	Kind        TaskKind               `json:"kind"`
	RiskLevel   RiskLevel              `json:"risk_level"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

// Subtask is the smallest schedulable unit in the orchestration DAG.
type Subtask struct {
	ID          string                 `json:"id"`
	Title       string                 `json:"title"`
	Description string                 `json:"description"`
	Kind        TaskKind               `json:"kind"`
	DependsOn   []string               `json:"depends_on"`
	CanParallel bool                   `json:"can_parallel"`
	RiskLevel   RiskLevel              `json:"risk_level"`
	Status      SubtaskStatus          `json:"status"`
	Timeout     time.Duration          `json:"timeout,omitempty"`
	RetryPolicy RetryPolicy            `json:"retry_policy,omitempty"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

// Dependency represents an explicit edge in the orchestration DAG.
type Dependency struct {
	FromID string         `json:"from_id"`
	ToID   string         `json:"to_id"`
	Type   DependencyType `json:"type"`
	Reason string         `json:"reason,omitempty"`
}

// ExecutionPlan is the planner output consumed by scheduler/executor/reporter.
type ExecutionPlan struct {
	ID           string                 `json:"id"`
	Task         OrchestrationTask      `json:"task"`
	Subtasks     []Subtask              `json:"subtasks"`
	Dependencies []Dependency           `json:"dependencies"`
	Batches      []ExecutionBatch       `json:"batches,omitempty"`
	Metadata     map[string]interface{} `json:"metadata,omitempty"`
}

// ExecutionBatch groups subtasks that share an execution mode/wave.
type ExecutionBatch struct {
	ID               string                 `json:"id"`
	Name             string                 `json:"name"`
	Mode             BatchMode              `json:"mode"`
	SubtaskIDs       []string               `json:"subtask_ids"`
	MaxConcurrency   int                    `json:"max_concurrency,omitempty"`
	RequiresApproval bool                   `json:"requires_approval,omitempty"`
	Metadata         map[string]interface{} `json:"metadata,omitempty"`
}

// ExecutionResult captures the result of one subtask execution.
type ExecutionResult struct {
	SubtaskID string                 `json:"subtask_id"`
	Status    SubtaskStatus          `json:"status"`
	Stdout    string                 `json:"stdout,omitempty"`
	Stderr    string                 `json:"stderr,omitempty"`
	Error     string                 `json:"error,omitempty"`
	Duration  time.Duration          `json:"duration"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

// NewSubtask creates a subtask with safe Phase-1 defaults.
func NewSubtask(id, title, description string) Subtask {
	return Subtask{
		ID:          id,
		Title:       title,
		Description: description,
		Kind:        TaskKindReadOnly,
		DependsOn:   []string{},
		CanParallel: true,
		RiskLevel:   RiskLow,
		Status:      StatusPending,
	}
}
