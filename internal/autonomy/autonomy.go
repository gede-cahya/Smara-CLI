// Package autonomy implements the Multi-Timeframe Autonomy Loop (Heartbeat)
// with Hold State capability for Smara CLI.
package autonomy

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// LoopState represents the state of the autonomy loop.
type LoopState string

const (
	StateObserving LoopState = "observing"
	StateThinking  LoopState = "thinking"
	StateActing    LoopState = "acting"
	StateHolding   LoopState = "holding"
	StateIdle      LoopState = "idle"
)

// Action represents a decision from the autonomy loop.
type Action string

const (
	ActionExecute Action = "execute"
	ActionHold    Action = "hold"
	ActionAlert   Action = "alert"
)

// Timeframe defines an observation interval and its conditions.
type Timeframe struct {
	Name        string        `json:"name"`
	Interval    time.Duration `json:"interval"`
	Description string        `json:"description"`
	Enabled     bool          `json:"enabled"`
}

// DefaultTimeframes provides sensible defaults for autonomous monitoring.
var DefaultTimeframes = []Timeframe{
	{Name: "error_log", Interval: 1 * time.Minute, Description: "Monitor error logs", Enabled: true},
	{Name: "health_check", Interval: 5 * time.Minute, Description: "System health check", Enabled: true},
	{Name: "dependency_update", Interval: 1 * time.Hour, Description: "Check dependency updates", Enabled: false},
	{Name: "memory_cleanup", Interval: 30 * time.Minute, Description: "Compact old memories", Enabled: true},
}

// ConditionChecker is a function that evaluates whether an action should be taken.
type ConditionChecker func() (bool, map[string]interface{})

// ActionExecutor is a function that executes an autonomous action.
type ActionExecutor func(context.Context, map[string]interface{}) error

// Engine runs the autonomous heartbeat loop.
type Engine struct {
	timeframes    []Timeframe
	checkers      map[string]ConditionChecker
	executors     map[string]ActionExecutor
	state         LoopState
	lastRun       map[string]time.Time
	cancel        context.CancelFunc
	wg            sync.WaitGroup
	mu            sync.RWMutex
	onStateChange func(LoopState)
	metrics       map[string]int // execution counts per timeframe
}

// NewEngine creates a new autonomy engine with default timeframes.
func NewEngine() *Engine {
	e := &Engine{
		timeframes: make([]Timeframe, len(DefaultTimeframes)),
		checkers:   make(map[string]ConditionChecker),
		executors:  make(map[string]ActionExecutor),
		state:      StateIdle,
		lastRun:    make(map[string]time.Time),
		metrics:    make(map[string]int),
	}
	copy(e.timeframes, DefaultTimeframes)
	return e
}

// RegisterChecker registers a condition checker for a timeframe.
func (e *Engine) RegisterChecker(timeframeName string, checker ConditionChecker) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.checkers[timeframeName] = checker
}

// RegisterExecutor registers an action executor for a timeframe.
func (e *Engine) RegisterExecutor(timeframeName string, executor ActionExecutor) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.executors[timeframeName] = executor
}

// AddTimeframe adds a custom timeframe.
func (e *Engine) AddTimeframe(tf Timeframe) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.timeframes = append(e.timeframes, tf)
}

// Start begins the autonomous heartbeat loop.
func (e *Engine) Start(ctx context.Context) {
	e.mu.Lock()
	if e.cancel != nil {
		e.cancel()
	}
	ctx, e.cancel = context.WithCancel(ctx)
	e.mu.Unlock()

	e.wg.Add(1)
	go func() {
		defer e.wg.Done()
		e.runLoop(ctx)
	}()
}

// Stop gracefully shuts down the autonomy loop.
func (e *Engine) Stop() {
	e.mu.Lock()
	if e.cancel != nil {
		e.cancel()
	}
	e.mu.Unlock()
	e.wg.Wait()
	e.setState(StateIdle)
}

func (e *Engine) runLoop(ctx context.Context) {
	// Run each timeframe on its own ticker
	for i := range e.timeframes {
		tf := e.timeframes[i]
		if !tf.Enabled {
			continue
		}
		e.wg.Add(1)
		go func(t Timeframe) {
			defer e.wg.Done()
			ticker := time.NewTicker(t.Interval)
			defer ticker.Stop()

			// Run immediately on start
			e.runCycle(ctx, t)

			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					e.runCycle(ctx, t)
				}
			}
		}(tf)
	}

	<-ctx.Done()
}

func (e *Engine) runCycle(ctx context.Context, tf Timeframe) {
	e.setState(StateObserving)

	e.mu.Lock()
	checker, hasChecker := e.checkers[tf.Name]
	executor, hasExecutor := e.executors[tf.Name]
	e.mu.Unlock()

	if !hasChecker || !hasExecutor {
		return
	}

	// Observe → Think
	e.setState(StateThinking)
	shouldAct, context := checker()

	if !shouldAct {
		// Hold state: conditions not met, save compute
		e.setState(StateHolding)
		return
	}

	// Act
	e.setState(StateActing)
	err := executor(ctx, context)

	e.mu.Lock()
	e.lastRun[tf.Name] = time.Now()
	if err == nil {
		e.metrics[tf.Name]++
	}
	e.mu.Unlock()

	e.setState(StateIdle)
}

func (e *Engine) setState(state LoopState) {
	e.mu.Lock()
	e.state = state
	cb := e.onStateChange
	e.mu.Unlock()
	if cb != nil {
		cb(state)
	}
}

// GetState returns the current loop state.
func (e *Engine) GetState() LoopState {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.state
}

// GetMetrics returns execution counts per timeframe.
func (e *Engine) GetMetrics() map[string]int {
	e.mu.RLock()
	defer e.mu.RUnlock()
	result := make(map[string]int, len(e.metrics))
	for k, v := range e.metrics {
		result[k] = v
	}
	return result
}

// GetLastRuns returns the last execution time per timeframe.
func (e *Engine) GetLastRuns() map[string]time.Time {
	e.mu.RLock()
	defer e.mu.RUnlock()
	result := make(map[string]time.Time, len(e.lastRun))
	for k, v := range e.lastRun {
		result[k] = v
	}
	return result
}

// SetStateChangeCallback sets a callback for state changes.
func (e *Engine) SetStateChangeCallback(fn func(LoopState)) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.onStateChange = fn
}

// EnableTimeframe enables or disables a timeframe.
func (e *Engine) EnableTimeframe(name string, enabled bool) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	for i := range e.timeframes {
		if e.timeframes[i].Name == name {
			e.timeframes[i].Enabled = enabled
			return nil
		}
	}
	return fmt.Errorf("timeframe '%s' tidak ditemukan", name)
}

// ListTimeframes returns all configured timeframes.
func (e *Engine) ListTimeframes() []Timeframe {
	e.mu.RLock()
	defer e.mu.RUnlock()
	result := make([]Timeframe, len(e.timeframes))
	copy(result, e.timeframes)
	return result
}
