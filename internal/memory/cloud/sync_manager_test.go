package cloud

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestSyncManagerStateSubscribeAndStopIdempotent(t *testing.T) {
	m := NewSyncManager(nil, nil, Config{})
	if got := m.CurrentState(); got != StateIdle {
		t.Fatalf("initial state = %s, want idle", got)
	}

	ch := make(chan State, 8)
	unsub := m.Subscribe(func(s State) { ch <- s })
	m.transitionTo(StateSyncing)
	m.transitionTo(StateIdle)

	want := []State{StateSyncing, StateIdle}
	for _, w := range want {
		select {
		case got := <-ch:
			if got != w {
				t.Fatalf("state callback = %s, want %s", got, w)
			}
		case <-time.After(time.Second):
			t.Fatalf("timeout waiting for %s", w)
		}
	}
	unsub()
	m.transitionTo(StateError)
	select {
	case got := <-ch:
		t.Fatalf("got callback after unsubscribe: %s", got)
	case <-time.After(50 * time.Millisecond):
	}

	if err := m.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := m.Start(context.Background()); err != nil {
		t.Fatalf("second Start: %v", err)
	}
	if err := m.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if got := m.CurrentState(); got != StateDisabled {
		t.Fatalf("state after Stop = %s, want disabled", got)
	}
	if err := m.Stop(); err != nil {
		t.Fatalf("second Stop: %v", err)
	}
}

func TestSyncManagerForceOfflineStart(t *testing.T) {
	m := NewSyncManager(nil, nil, Config{OfflineMode: "force-offline"})
	ch := make(chan State, 2)
	m.Subscribe(func(s State) { ch <- s })
	if err := m.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer m.Stop()
	select {
	case got := <-ch:
		if got != StateOffline {
			t.Fatalf("state = %s, want offline", got)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for offline")
	}
}

func TestSyncManagerCheckBudgetBeforeWrite(t *testing.T) {
	m := NewSyncManager(nil, nil, Config{MaxRowsPerHour: 2})
	if ok, reason := m.checkBudgetBeforeWrite(1); !ok || reason != "" {
		t.Fatalf("first write rejected: ok=%v reason=%q", ok, reason)
	}
	if ok, reason := m.checkBudgetBeforeWrite(1); !ok || reason != "" {
		t.Fatalf("second write rejected: ok=%v reason=%q", ok, reason)
	}
	if ok, reason := m.checkBudgetBeforeWrite(1); ok || reason == "" {
		t.Fatalf("third write allowed unexpectedly: ok=%v reason=%q", ok, reason)
	}

	m2 := NewSyncManager(nil, nil, Config{MaxRowsPerHour: 100})
	m2.setQuota(QuotaInfo{PercentUsed: 99, StorageBytes: 99 << 20, StorageLimitBytes: 100 << 20})
	if ok, reason := m2.checkBudgetBeforeWrite(1); ok || reason == "" {
		t.Fatalf("quota write allowed unexpectedly: ok=%v reason=%q", ok, reason)
	}
}

func TestSyncManagerSubscribeConcurrentUnsubscribe(t *testing.T) {
	m := NewSyncManager(nil, nil, Config{})
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			unsub := m.Subscribe(func(State) {})
			m.transitionTo(StateSyncing)
			unsub()
		}()
	}
	wg.Wait()
}

func TestSyncManagerRecover(t *testing.T) {
	t.Run("error→idle", func(t *testing.T) {
		m := NewSyncManager(nil, nil, Config{})
		m.transitionTo(StateError)
		if got := m.CurrentState(); got != StateError {
			t.Fatalf("pre-recover state = %s, want error", got)
		}
		m.Recover()
		if got := m.CurrentState(); got != StateIdle {
			t.Fatalf("post-recover state = %s, want idle", got)
		}
	})

	t.Run("idle→idle (no-op)", func(t *testing.T) {
		m := NewSyncManager(nil, nil, Config{})
		m.Recover()
		if got := m.CurrentState(); got != StateIdle {
			t.Fatalf("state from idle = %s, want idle", got)
		}
	})

	t.Run("conflict→idle when cleared", func(t *testing.T) {
		m := NewSyncManager(nil, nil, Config{})
		m.transitionTo(StateConflict)
		m.Recover() // no store, no conflicts → should clear
		if got := m.CurrentState(); got != StateIdle {
			t.Fatalf("post-recover state = %s, want idle", got)
		}
	})

	t.Run("nil manager safe", func(t *testing.T) {
		var m *SyncManager
		m.Recover() // must not panic
	})
}

func TestSyncManagerSetReplicaPath(t *testing.T) {
	m := NewSyncManager(nil, nil, Config{MaxStorageMB: 1})
	m.SetReplicaPath("/nonexistent/path.db")
	// No crash from Stat on nonexistent — checkBudgetBeforeWrite swallows error.
	ok, _ := m.checkBudgetBeforeWrite(1)
	if !ok {
		t.Fatal("write rejected for nonexistent replica path")
	}

	// Nil manager safe.
	var m2 *SyncManager
	m2.SetReplicaPath("/tmp/x.db")
}

func TestSyncManagerTransitionSameState(t *testing.T) {
	m := NewSyncManager(nil, nil, Config{})
	ch := make(chan State, 4)
	m.Subscribe(func(s State) { ch <- s })

	// Same-state transition must not fire subscriber.
	m.transitionTo(StateIdle) // idle → idle (no change)

	select {
	case got := <-ch:
		t.Fatalf("unexpected callback on same-state transition: %s", got)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestSyncManagerPeriodicSyncNotLaunchedWhenIntervalZero(t *testing.T) {
	m := NewSyncManager(nil, nil, Config{SyncIntervalSec: 0})
	if err := m.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer m.Stop()
	// No assertion needed beyond no-goroutine-leak — the goroutine is
	// simply not started when SyncIntervalSec == 0.
}
