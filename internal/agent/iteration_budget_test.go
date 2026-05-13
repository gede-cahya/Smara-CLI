package agent

import "testing"

func TestIterationBudget_ModeDefaults(t *testing.T) {
	cases := []struct {
		mode      Mode
		wantBase  int
		wantHard  int
	}{
		{ModeAsk, 5, 15},
		{ModeRush, 15, 40},
		{ModePlan, 12, 30},
		{ModeWorkflow, 30, 80},
		{ModeTest, 10, 25},
	}
	for _, c := range cases {
		t.Run(string(c.mode), func(t *testing.T) {
			b := NewIterationBudget(c.mode, 0)
			if b.Limit() != c.wantBase {
				t.Errorf("Limit() = %d, want %d", b.Limit(), c.wantBase)
			}
			if b.HardCap() != c.wantHard {
				t.Errorf("HardCap() = %d, want %d", b.HardCap(), c.wantHard)
			}
		})
	}
}

func TestIterationBudget_UserOverride(t *testing.T) {
	b := NewIterationBudget(ModeAsk, 100)
	if b.Limit() != 100 {
		t.Errorf("user override should be respected; got %d", b.Limit())
	}
	if b.HardCap() != 100 {
		t.Errorf("override should set hard cap = %d, got %d", 100, b.HardCap())
	}
}

func TestIterationBudget_StuckLoopDetection(t *testing.T) {
	b := NewIterationBudget(ModeWorkflow, 0)
	args := map[string]interface{}{"host": "vps", "cmd": "systemctl status hermes"}

	// First call
	if stuck := b.RecordToolCalls("ssh_exec", args); stuck {
		t.Errorf("first call should not be stuck")
	}
	// Second call (same args)
	if stuck := b.RecordToolCalls("ssh_exec", args); stuck {
		t.Errorf("second call should not be stuck yet")
	}
	// Third call → stuck loop
	if stuck := b.RecordToolCalls("ssh_exec", args); !stuck {
		t.Errorf("third identical call MUST flag stuck loop")
	}
}

func TestIterationBudget_DifferentArgsNotStuck(t *testing.T) {
	b := NewIterationBudget(ModeWorkflow, 0)

	for i := 0; i < 5; i++ {
		args := map[string]interface{}{"path": "file" + itoa(int64(i))}
		if stuck := b.RecordToolCalls("read_file", args); stuck {
			t.Errorf("call %d with unique args should not be stuck", i)
		}
	}
}

func TestIterationBudget_OscillatingLoop(t *testing.T) {
	b := NewIterationBudget(ModeWorkflow, 0)
	a := map[string]interface{}{"x": "A"}
	bb := map[string]interface{}{"x": "B"}

	// Alternate A B A B A B A B → 5 of A, 5 of B in window of 8.
	stuck := false
	for i := 0; i < 8; i++ {
		args := a
		if i%2 == 1 {
			args = bb
		}
		if b.RecordToolCalls("tool", args) {
			stuck = true
		}
	}
	if !stuck {
		t.Errorf("oscillating A/B/A/B should eventually flag stuck (5+ in window)")
	}
}

func TestIterationBudget_ProgressExtension(t *testing.T) {
	b := NewIterationBudget(ModeRush, 0) // base 15, hard 40

	// Fill recent with diverse calls (progress = high uniqueness)
	for i := 0; i < 8; i++ {
		args := map[string]interface{}{"i": i}
		_ = b.RecordToolCalls("step_"+itoa(int64(i)), args)
	}

	// At iteration = 14 still within base, ok.
	if !b.ShouldContinue(14) {
		t.Errorf("iter 14 should be within base 15")
	}
	// At iteration = 15 (boundary), with progress, budget should extend.
	if !b.ShouldContinue(15) {
		t.Errorf("iter 15 with diverse history should trigger extension")
	}
	if b.Limit() <= 15 {
		t.Errorf("budget should have extended beyond 15; got %d", b.Limit())
	}
	if b.Limit() > 40 {
		t.Errorf("budget should not exceed hard cap 40; got %d", b.Limit())
	}
}

func TestIterationBudget_NoExtensionWhenStuck(t *testing.T) {
	b := NewIterationBudget(ModeRush, 0)

	// Fill with same call repeatedly (no progress)
	args := map[string]interface{}{"x": "y"}
	for i := 0; i < 8; i++ {
		_ = b.RecordToolCalls("repeat", args)
	}

	// Past base, should NOT extend (no progress).
	if b.ShouldContinue(15) {
		t.Errorf("repetitive history should NOT extend the budget")
	}
}
