package cloud

import (
	"sync"

	"github.com/gede-cahya/Smara-CLI/internal/audit"
)

// stateBox is used with atomic.Pointer because State is a string alias and
// taking addresses of shared package-level values is safer through a stable box.
type stateBox struct{ v State }

func boxedState(s State) *stateBox { return &stateBox{v: s} }

// CurrentState returns the manager's current state. Nil managers are disabled.
func (m *SyncManager) CurrentState() State {
	if m == nil {
		return StateDisabled
	}
	if b := m.state.Load(); b != nil {
		return b.v
	}
	return StateIdle
}

// Subscribe registers fn for state transitions and returns an unsubscribe func.
// The callback is invoked asynchronously for future transitions only.
func (m *SyncManager) Subscribe(fn func(State)) func() {
	if m == nil || fn == nil {
		return func() {}
	}
	m.subsMu.Lock()
	defer m.subsMu.Unlock()
	if m.subscribers == nil {
		m.subscribers = make(map[uint64]func(State))
	}
	id := m.nextSubID
	m.nextSubID++
	m.subscribers[id] = fn
	var once sync.Once
	return func() {
		once.Do(func() {
			m.subsMu.Lock()
			delete(m.subscribers, id)
			m.subsMu.Unlock()
		})
	}
}

func (m *SyncManager) transitionTo(next State) {
	if m == nil {
		return
	}
	prev := m.CurrentState()
	if prev == next {
		return
	}
	m.state.Store(boxedState(next))

	// Audit every state transition so operators can trace the sync lifecycle
	// without parsing goroutine stack traces or log-level filters.
	_ = audit.LogCloudOp("state", true, m.cfg.Provider, map[string]any{
		"from": string(prev),
		"to":   string(next),
	})

	m.subsMu.RLock()
	callbacks := make([]func(State), 0, len(m.subscribers))
	for _, fn := range m.subscribers {
		callbacks = append(callbacks, fn)
	}
	m.subsMu.RUnlock()
	// Synchronous dispatch so observers see transitions in order. Callbacks
	// must not block; that would stall sync flow. Subscribers needing async
	// processing should fan out themselves.
	for _, fn := range callbacks {
		fn(next)
	}
}
