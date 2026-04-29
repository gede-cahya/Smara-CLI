package autonomy

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewEngine_Defaults(t *testing.T) {
	e := NewEngine()
	require.NotNil(t, e)
	assert.Equal(t, StateIdle, e.GetState())
	assert.NotEmpty(t, e.ListTimeframes())
	assert.Empty(t, e.GetMetrics())
	assert.Empty(t, e.GetLastRuns())
}

func TestRegisterCheckerAndExecutor(t *testing.T) {
	e := NewEngine()
	e.RegisterChecker("error_log", func() (bool, map[string]interface{}) {
		return true, nil
	})
	e.RegisterExecutor("error_log", func(ctx context.Context, data map[string]interface{}) error {
		return nil
	})
	assert.NotNil(t, e.checkers["error_log"])
	assert.NotNil(t, e.executors["error_log"])
}

func TestAddTimeframe(t *testing.T) {
	e := NewEngine()
	tf := Timeframe{Name: "custom", Interval: 1 * time.Second, Enabled: true}
	e.AddTimeframe(tf)
	list := e.ListTimeframes()
	found := false
	for _, item := range list {
		if item.Name == "custom" {
			found = true
			assert.Equal(t, 1*time.Second, item.Interval)
		}
	}
	assert.True(t, found)
}

func TestEnableTimeframe(t *testing.T) {
	e := NewEngine()
	err := e.EnableTimeframe("error_log", false)
	require.NoError(t, err)
	list := e.ListTimeframes()
	for _, tf := range list {
		if tf.Name == "error_log" {
			assert.False(t, tf.Enabled)
		}
	}
}

func TestEnableTimeframe_NotFound(t *testing.T) {
	e := NewEngine()
	err := e.EnableTimeframe("nonexistent", false)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "nonexistent")
}

func TestStartStop_Lifecycle(t *testing.T) {
	e := NewEngine()
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	e.Start(ctx)
	// Should not panic or deadlock
	time.Sleep(50 * time.Millisecond)
	e.Stop()
	assert.Equal(t, StateIdle, e.GetState())
}

func TestRunCycle_Execute(t *testing.T) {
	e := NewEngine()
	var executed atomic.Bool
	e.RegisterChecker("error_log", func() (bool, map[string]interface{}) {
		return true, map[string]interface{}{"key": "val"}
	})
	e.RegisterExecutor("error_log", func(ctx context.Context, data map[string]interface{}) error {
		executed.Store(true)
		return nil
	})

	// Manually trigger a cycle using a short Start/Stop
	tf := Timeframe{Name: "error_log", Interval: 10 * time.Millisecond, Enabled: true}
	e.AddTimeframe(tf)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	e.Start(ctx)
	time.Sleep(100 * time.Millisecond)
	e.Stop()

	assert.True(t, executed.Load(), "executor should have been called")
	metrics := e.GetMetrics()
	assert.GreaterOrEqual(t, metrics["error_log"], 1)
	lastRuns := e.GetLastRuns()
	assert.NotZero(t, lastRuns["error_log"])
}

func TestRunCycle_Hold(t *testing.T) {
	e := NewEngine()
	var executed atomic.Bool
	e.RegisterChecker("health_check", func() (bool, map[string]interface{}) {
		return false, nil // hold
	})
	e.RegisterExecutor("health_check", func(ctx context.Context, data map[string]interface{}) error {
		executed.Store(true)
		return nil
	})

	tf := Timeframe{Name: "health_check", Interval: 10 * time.Millisecond, Enabled: true}
	e.AddTimeframe(tf)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	e.Start(ctx)
	time.Sleep(100 * time.Millisecond)
	e.Stop()

	assert.False(t, executed.Load(), "executor should NOT have been called when checker returns false")
}

func TestStateChangeCallback(t *testing.T) {
	e := NewEngine()
	var states []LoopState
	e.SetStateChangeCallback(func(s LoopState) {
		states = append(states, s)
	})

	e.RegisterChecker("error_log", func() (bool, map[string]interface{}) {
		return true, nil
	})
	e.RegisterExecutor("error_log", func(ctx context.Context, data map[string]interface{}) error {
		return nil
	})

	tf := Timeframe{Name: "error_log", Interval: 10 * time.Millisecond, Enabled: true}
	e.AddTimeframe(tf)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	e.Start(ctx)
	time.Sleep(80 * time.Millisecond)
	e.Stop()

	assert.NotEmpty(t, states)
}

func TestListTimeframes_Copy(t *testing.T) {
	e := NewEngine()
	list := e.ListTimeframes()
	require.NotEmpty(t, list)
	list[0].Enabled = false
	// Original should remain unchanged
	orig := e.ListTimeframes()
	for _, tf := range orig {
		if tf.Name == list[0].Name {
			assert.True(t, tf.Enabled, "modifying returned slice should not affect internal state")
		}
	}
}
