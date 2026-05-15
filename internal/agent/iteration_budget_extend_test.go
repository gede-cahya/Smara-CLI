package agent

import (
	"strings"
	"testing"
)

func TestIterationBudget_RequestExtension_HappyPath(t *testing.T) {
	b := NewIterationBudget(ModeRush, 0) // base 15, hardCap 40

	res := b.RequestExtension(20, "menyelesaikan multi-file refactor")
	if !res.Granted || res.GrantedAmount != 20 {
		t.Fatalf("expected granted 20, got %+v", res)
	}
	if res.NewHardCap != 60 || res.NewLimit < 35 {
		t.Fatalf("unexpected new caps: %+v", res)
	}
	if res.RemainingGrant != MaxManualExtRequests-1 {
		t.Fatalf("remaining grant accounting wrong: %d", res.RemainingGrant)
	}
}

func TestIterationBudget_RequestExtension_RequiresReason(t *testing.T) {
	b := NewIterationBudget(ModeAsk, 0)
	res := b.RequestExtension(5, "   ")
	if res.Granted {
		t.Fatalf("empty reason should be rejected: %+v", res)
	}
	if !strings.Contains(res.Denial, "alasan") {
		t.Fatalf("denial should mention alasan: %q", res.Denial)
	}
}

func TestIterationBudget_RequestExtension_RespectsMultiplierCeiling(t *testing.T) {
	b := NewIterationBudget(ModeAsk, 0) // hardCap 15, ceiling 45

	r1 := b.RequestExtension(20, "task butuh banyak step") // grants 20, hardCap=35
	if !r1.Granted || r1.GrantedAmount != 20 {
		t.Fatalf("first grant: %+v", r1)
	}
	r2 := b.RequestExtension(20, "lanjutan masih butuh") // can only add 10 (35→45)
	if !r2.Granted || r2.GrantedAmount != 10 {
		t.Fatalf("second grant should be clamped to 10: %+v", r2)
	}
	r3 := b.RequestExtension(5, "harusnya gagal") // already at ceiling
	if r3.Granted {
		t.Fatalf("third grant should be denied: %+v", r3)
	}
	if !strings.Contains(r3.Denial, "absolut") {
		t.Fatalf("denial should mention absolut ceiling: %q", r3.Denial)
	}
}

func TestIterationBudget_RequestExtension_RespectsRequestQuota(t *testing.T) {
	b := NewIterationBudget(ModeWorkflow, 0) // base 30, hardCap 80, ceiling 240
	for i := 0; i < MaxManualExtRequests; i++ {
		res := b.RequestExtension(2, "step kecil")
		if !res.Granted {
			t.Fatalf("iter %d should be granted: %+v", i, res)
		}
	}
	res := b.RequestExtension(2, "kelima sudah habis")
	if res.Granted {
		t.Fatalf("after %d grants, should be denied: %+v", MaxManualExtRequests, res)
	}
	if !strings.Contains(res.Denial, "kuota") {
		t.Fatalf("denial should mention kuota: %q", res.Denial)
	}
}

func TestIterationBudget_Snapshot_TracksManualExtensions(t *testing.T) {
	b := NewIterationBudget(ModeRush, 0)
	b.RequestExtension(7, "menyelesaikan task A")
	b.RequestExtension(3, "menyelesaikan task B")

	snap := b.Snapshot()
	if snap.ManualExtCount != 2 || snap.ManualExtTotal != 10 {
		t.Fatalf("snapshot mismatch: %+v", snap)
	}
	if snap.ManualExtRemaining != MaxManualExtRequests-2 {
		t.Fatalf("remaining wrong: %+v", snap)
	}
}
