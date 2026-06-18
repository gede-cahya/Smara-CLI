package web

import (
	"context"
	"testing"
	"time"

)

func TestOrchestrationApprovalFlow(t *testing.T) {
	store := NewOrchestrationStatusStore()

	// Test 1: WaitApproval blocks until Decide is called
	t.Run("blocking approval", func(t *testing.T) {
		ctx := context.Background()
		subtaskID := "test-subtask-1"
		
		// Start approval wait in goroutine
		done := make(chan bool, 1)
		go func() {
			result := store.WaitApproval(ctx, subtaskID, 5*time.Second)
			done <- result
		}()

		// Give it time to register
		time.Sleep(50 * time.Millisecond)

		// Approve it
		success := store.Decide(subtaskID, true)
		if !success {
			t.Error("Decide should return true for pending approval")
		}

		// Verify approval was granted
		select {
		case result := <-done:
			if !result {
				t.Error("WaitApproval should return true when approved")
			}
		case <-time.After(1 * time.Second):
			t.Error("WaitApproval should unblock after Decide")
		}
	})

	// Test 2: Decide returns false for non-existent approval
	t.Run("decide non-existent", func(t *testing.T) {
		success := store.Decide("non-existent", true)
		if success {
			t.Error("Decide should return false for non-existent approval")
		}
	})

	// Test 3: Timeout behavior
	t.Run("timeout approval", func(t *testing.T) {
		ctx := context.Background()
		subtaskID := "test-subtask-timeout"
		
		result := store.WaitApproval(ctx, subtaskID, 100*time.Millisecond)
		if result {
			t.Error("WaitApproval should return false on timeout")
		}
	})

	// Test 4: Context cancellation
	t.Run("context cancel", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		subtaskID := "test-subtask-cancel"
		
		done := make(chan bool, 1)
		go func() {
			result := store.WaitApproval(ctx, subtaskID, 5*time.Second)
			done <- result
		}()

		time.Sleep(50 * time.Millisecond)
		cancel()

		select {
		case result := <-done:
			if result {
				t.Error("WaitApproval should return false when context cancelled")
			}
		case <-time.After(1 * time.Second):
			t.Error("WaitApproval should unblock when context cancelled")
		}
	})

	// Test 5: Multiple approvals
	t.Run("multiple approvals", func(t *testing.T) {
		ctx := context.Background()
		ids := []string{"subtask-a", "subtask-b", "subtask-c"}
		
		results := make(chan bool, 3)
		for _, id := range ids {
			go func(subtaskID string) {
				result := store.WaitApproval(ctx, subtaskID, 5*time.Second)
				results <- result
			}(id)
		}

		time.Sleep(100 * time.Millisecond)

		// Approve all
		for _, id := range ids {
			store.Decide(id, true)
		}

		// Verify all were approved
		for i := 0; i < 3; i++ {
			select {
			case result := <-results:
				if !result {
					t.Error("All approvals should succeed")
				}
			case <-time.After(1 * time.Second):
				t.Error("All approvals should complete")
			}
		}
	})

	// Test 6: Deny approval
	t.Run("deny approval", func(t *testing.T) {
		ctx := context.Background()
		subtaskID := "test-subtask-deny"
		
		done := make(chan bool, 1)
		go func() {
			result := store.WaitApproval(ctx, subtaskID, 5*time.Second)
			done <- result
		}()

		time.Sleep(50 * time.Millisecond)
		store.Decide(subtaskID, false)

		select {
		case result := <-done:
			if result {
				t.Error("WaitApproval should return false when denied")
			}
		case <-time.After(1 * time.Second):
			t.Error("WaitApproval should unblock after denial")
		}
	})
}

func TestApprovalTimeout(t *testing.T) {
	timeout := approvalTimeout()
	if timeout != 5*time.Minute {
		t.Errorf("approvalTimeout should return 5 minutes, got %v", timeout)
	}
}
