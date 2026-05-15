package cloud

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gede-cahya/Smara-CLI/internal/audit"
)

// SyncManagerStore is the storage contract used by SyncManager without importing internal/memory.
type SyncManagerStore interface {
	DB() *sql.DB
	GetMemoryByCloudID(cloudID string) (*MemoryRow, error)
	UpdateMemoryFromConflict(memID int64, winner MemoryRow, loser *MemoryVersionRow) error
	InsertCloudConflict(c CloudConflict) error
	ListUnresolvedConflicts() ([]CloudConflict, error)
	MarkConflictResolved(id int64, resolution string) error
}

// CloudConflict represents one unresolved divergent cloud-memory row pair.
type CloudConflict struct {
	ID              int64
	MemoryID        int64
	CloudID         string
	LocalVersion    int
	RemoteVersion   int
	LocalContent    string
	RemoteContent   string
	LocalDeviceID   string
	RemoteDeviceID  string
	LocalUpdatedAt  time.Time
	RemoteUpdatedAt time.Time
	DetectedAt      time.Time
}

// SyncManager coordinates background replication and explicit sync commands.
type SyncManager struct {
	provider Provider
	store    SyncManagerStore
	cfg      Config

	state       atomic.Pointer[stateBox]
	subsMu      sync.RWMutex
	subscribers map[uint64]func(State)
	nextSubID   uint64

	startStopMu sync.Mutex
	started     bool
	ctx         context.Context
	cancel      context.CancelFunc

	rowsThisHour       atomic.Int64
	hourlyResetTicker  *time.Ticker
	quotaMu            sync.RWMutex
	lastQuota          QuotaInfo
	quotaRefreshTicker *time.Ticker
	replicaPath        string
}

// NewSyncManager builds a SyncManager. The state machine starts in StateIdle;
// callers must invoke Start to launch background tickers.
func NewSyncManager(p Provider, store SyncManagerStore, cfg Config) *SyncManager {
	m := &SyncManager{
		provider:    p,
		store:       store,
		cfg:         cfg,
		subscribers: make(map[uint64]func(State)),
	}
	m.state.Store(boxedState(StateIdle))
	return m
}

// SetReplicaPath tells the SyncManager where the on-disk replica lives so
// checkBudgetBeforeWrite can enforce MaxStorageMB. Callers (OpenStoreWithCloud)
// should invoke this after construction.
func (m *SyncManager) SetReplicaPath(path string) {
	if m == nil {
		return
	}
	m.replicaPath = path
}

// Start launches background quota, budget, and periodic-sync tickers. Idempotent.
func (m *SyncManager) Start(ctx context.Context) error {
	if m == nil {
		return nil
	}
	m.startStopMu.Lock()
	defer m.startStopMu.Unlock()
	if m.started {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	m.ctx, m.cancel = context.WithCancel(ctx)
	m.started = true
	m.transitionTo(StateIdle)

	// force-offline: skip all network activity, no tickers.
	if m.cfg.OfflineMode == "force-offline" {
		m.transitionTo(StateOffline)
		return nil
	}

	// Hourly row-budget reset.
	m.hourlyResetTicker = time.NewTicker(time.Hour)

	// Quota refresh every 5 minutes.
	m.quotaRefreshTicker = time.NewTicker(5 * time.Minute)
	go m.backgroundQuotaAndBudget(m.ctx, m.hourlyResetTicker, m.quotaRefreshTicker)

	// Periodic sync ticker when SyncIntervalSec > 0.
	if m.cfg.SyncIntervalSec > 0 {
		go m.backgroundPeriodicSync(m.ctx, time.Duration(m.cfg.SyncIntervalSec)*time.Second)
	}

	if m.provider != nil {
		if st, err := m.provider.Status(m.ctx); err == nil && st != nil {
			m.setQuota(st.Quota)
		}
	}
	return nil
}

// backgroundPeriodicSync runs SyncNow on a ticker. Errors are surfaced via
// state transitions (→ StateError) rather than crashing the goroutine.
func (m *SyncManager) backgroundPeriodicSync(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// Skip if already syncing — prevents overlapping runs.
			if m.CurrentState() == StateSyncing {
				continue
			}
			// Skip from force-offline mode.
			if m.cfg.OfflineMode == "force-offline" {
				continue
			}
			_, _ = m.SyncNow(ctx)
		}
	}
}

func (m *SyncManager) backgroundQuotaAndBudget(ctx context.Context, hourly, quota *time.Ticker) {
	// Capture ticker handles by parameter so this goroutine doesn't race with
	// Stop() clearing the fields on the manager.
	for {
		select {
		case <-ctx.Done():
			return
		case <-hourly.C:
			m.rowsThisHour.Store(0)
		case <-quota.C:
			if m.provider != nil {
				if st, err := m.provider.Status(ctx); err == nil && st != nil {
					m.setQuota(st.Quota)
				}
			}
		}
	}
}

// Stop halts background work and closes the provider.
func (m *SyncManager) Stop() (err error) {
	if m == nil {
		return nil
	}
	defer func() {
		extra := map[string]any{"provider": m.cfg.Provider}
		if err != nil {
			extra["error"] = err.Error()
		}
		_ = audit.LogCloudOp("disable", err == nil, "", extra)
	}()
	m.startStopMu.Lock()
	defer m.startStopMu.Unlock()
	if m.cancel != nil {
		m.cancel()
		m.cancel = nil
	}
	if m.hourlyResetTicker != nil {
		m.hourlyResetTicker.Stop()
		m.hourlyResetTicker = nil
	}
	if m.quotaRefreshTicker != nil {
		m.quotaRefreshTicker.Stop()
		m.quotaRefreshTicker = nil
	}
	m.started = false
	m.ctx = nil
	m.transitionTo(StateDisabled)
	if m.provider != nil {
		_ = m.provider.Close()
	}
	return nil
}

// Recover attempts to clear the error/conflict state by re-checking conditions.
// It is idempotent and safe to call from any state.
//
//   - StateError → StateIdle (error condition presumed resolved by caller)
//   - StateConflict → StateIdle if no unresolved conflicts remain (or store is nil)
//   - Other states are left unchanged
func (m *SyncManager) Recover() {
	if m == nil {
		return
	}
	current := m.CurrentState()
	switch current {
	case StateError:
		m.transitionTo(StateIdle)
	case StateConflict:
		// If there's no store, there's nothing to conflict about.
		if m.store == nil {
			m.transitionTo(StateIdle)
			return
		}
		conflicts, err := m.store.ListUnresolvedConflicts()
		if err == nil && len(conflicts) == 0 {
			m.transitionTo(StateIdle)
		}
	}
}

// Status returns the current sync status, refreshing quota cache on success.
func (m *SyncManager) Status(ctx context.Context) (*SyncStatus, error) {
	if m == nil || m.provider == nil {
		return &SyncStatus{State: StateDisabled}, nil
	}
	st, err := m.provider.Status(ctx)
	if st == nil {
		st = &SyncStatus{}
	}
	if err == nil {
		m.setQuota(st.Quota)
	}
	if m.store != nil {
		if conflicts, e := m.store.ListUnresolvedConflicts(); e == nil {
			st.UnresolvedConflicts = len(conflicts)
		}
	}
	return st, err
}

// SyncNow performs push → pull → conflict-detection → reconcile in one call.
// State transitions: previous → StateSyncing → (StateIdle | StateConflict | StateError).
func (m *SyncManager) SyncNow(ctx context.Context) (report *SyncReport, err error) {
	report = &SyncReport{StartedAt: time.Now()}
	if m != nil {
		m.transitionTo(StateSyncing)
	}
	defer func() {
		report.FinishedAt = time.Now()
		if m == nil {
			return
		}
		switch {
		case err != nil || len(report.Errors) > 0:
			m.transitionTo(StateError)
		case report.Conflicts > 0 && m.cfg.ConflictPolicy == PolicyManual:
			m.transitionTo(StateConflict)
		default:
			m.transitionTo(StateIdle)
		}
		extra := map[string]any{
			"pushed":    report.PushedRows,
			"pulled":    report.PulledFrames,
			"conflicts": report.Conflicts,
		}
		if len(report.Errors) > 0 {
			extra["errors"] = report.Errors
		}
		_ = audit.LogCloudOp("sync", err == nil && len(report.Errors) == 0, m.cfg.Provider, extra)
	}()

	if m == nil || m.provider == nil {
		err = errors.New("cloud: SyncManager not initialized")
		report.Errors = append(report.Errors, err.Error())
		return report, err
	}

	if push, e := m.provider.Push(ctx); e != nil {
		report.Errors = append(report.Errors, e.Error())
	} else if push != nil {
		report.PushedRows += push.PushedRows
		report.Errors = append(report.Errors, push.Errors...)
	}

	if pull, e := m.provider.Pull(ctx); e != nil {
		report.Errors = append(report.Errors, e.Error())
	} else if pull != nil {
		report.PulledFrames += pull.PulledFrames
		report.Errors = append(report.Errors, pull.Errors...)
	}

	conflicts, e := m.detectConflicts(ctx)
	if e != nil {
		report.Errors = append(report.Errors, e.Error())
	} else {
		report.Conflicts = conflicts
	}

	if m.cfg.ConflictPolicy != PolicyManual && report.Conflicts > 0 {
		if _, e := m.Reconcile(ctx); e != nil {
			report.Errors = append(report.Errors, e.Error())
		}
	}

	if len(report.Errors) > 0 {
		err = fmt.Errorf("cloud sync completed with %d error(s)", len(report.Errors))
	}
	return report, err
}

// detectConflicts inspects _cloud_pull_staging vs memories and inserts any
// divergent rows into cloud_conflicts. Returns the number of new conflicts.
func (m *SyncManager) detectConflicts(ctx context.Context) (int, error) {
	if m == nil || m.store == nil || m.store.DB() == nil {
		return 0, nil
	}
	db := m.store.DB()
	var exists string
	if err := db.QueryRowContext(ctx, `SELECT name FROM sqlite_master WHERE type='table' AND name='_cloud_pull_staging'`).Scan(&exists); err != nil {
		return 0, nil
	}
	rows, err := db.QueryContext(ctx, `
		SELECT m.id, m.cloud_id, m.content, m.updated_at, m.version, COALESCE(m.device_id,''),
		       COALESCE(p.content,''), COALESCE(p.updated_at, CURRENT_TIMESTAMP),
		       COALESCE(p.version,1), COALESCE(p.device_id,'')
		FROM memories m
		JOIN _cloud_pull_staging p ON m.cloud_id = p.cloud_id
		WHERE COALESCE(m.content_hash,'') != COALESCE(p.content_hash,'')
		  AND COALESCE(m.device_id,'')    != COALESCE(p.device_id,'')`)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		var c CloudConflict
		if err := rows.Scan(
			&c.MemoryID, &c.CloudID, &c.LocalContent, &c.LocalUpdatedAt, &c.LocalVersion, &c.LocalDeviceID,
			&c.RemoteContent, &c.RemoteUpdatedAt, &c.RemoteVersion, &c.RemoteDeviceID,
		); err != nil {
			return count, err
		}
		if err := m.store.InsertCloudConflict(c); err != nil {
			return count, err
		}
		count++
	}
	return count, rows.Err()
}

// Reconcile applies the configured ConflictPolicy to every unresolved conflict.
func (m *SyncManager) Reconcile(ctx context.Context) (*ReconcileReport, error) {
	report := &ReconcileReport{SyncReport: SyncReport{StartedAt: time.Now()}}
	defer func() { report.FinishedAt = time.Now() }()
	if m == nil || m.store == nil {
		return report, errors.New("cloud: SyncManager store not initialized")
	}
	resolver, err := NewResolver(m.cfg.ConflictPolicy)
	if err != nil {
		return report, err
	}
	conflicts, err := m.store.ListUnresolvedConflicts()
	if err != nil {
		return report, err
	}
	for _, c := range conflicts {
		select {
		case <-ctx.Done():
			return report, ctx.Err()
		default:
		}
		local := MemoryRow{ID: c.MemoryID, CloudID: c.CloudID, Content: c.LocalContent, DeviceID: c.LocalDeviceID, Version: c.LocalVersion, UpdatedAt: c.LocalUpdatedAt}
		remote := MemoryRow{ID: c.MemoryID, CloudID: c.CloudID, Content: c.RemoteContent, DeviceID: c.RemoteDeviceID, Version: c.RemoteVersion, UpdatedAt: c.RemoteUpdatedAt}
		winner, loser, rerr := resolver.Resolve(local, remote)
		if errors.Is(rerr, ErrManualConflict) {
			report.DeferredManual++
			continue
		}
		if rerr != nil {
			report.Errors = append(report.Errors, rerr.Error())
			continue
		}
		if uerr := m.store.UpdateMemoryFromConflict(c.MemoryID, winner, loser); uerr != nil {
			report.Errors = append(report.Errors, uerr.Error())
			continue
		}
		label := fmt.Sprintf("auto:%s", m.cfg.ConflictPolicy)
		if mErr := m.store.MarkConflictResolved(c.ID, label); mErr != nil {
			report.Errors = append(report.Errors, mErr.Error())
			continue
		}
		report.ResolvedAuto++
		if loser != nil {
			report.ArchivedLosers++
		}
	}
	if len(report.Errors) > 0 {
		return report, fmt.Errorf("reconcile completed with %d error(s)", len(report.Errors))
	}
	return report, nil
}

func (m *SyncManager) setQuota(q QuotaInfo) {
	m.quotaMu.Lock()
	m.lastQuota = q
	m.quotaMu.Unlock()
}

// Push delegates to the underlying provider and returns a SyncReport-shaped
// result so callers can print the same summary used by SyncNow.
func (m *SyncManager) Push(ctx context.Context) (*SyncReport, error) {
	report := &SyncReport{StartedAt: time.Now()}
	defer func() { report.FinishedAt = time.Now() }()
	if m == nil || m.provider == nil {
		return report, errors.New("cloud: SyncManager not initialized")
	}
	res, err := m.provider.Push(ctx)
	if res != nil {
		report.PushedRows = res.PushedRows
		report.Errors = append(report.Errors, res.Errors...)
	}
	if err != nil {
		report.Errors = append(report.Errors, err.Error())
	}
	_ = audit.LogCloudOp("push", err == nil, m.cfg.Provider, map[string]any{"pushed": report.PushedRows})
	return report, err
}

// Pull delegates to the underlying provider.
func (m *SyncManager) Pull(ctx context.Context) (*SyncReport, error) {
	report := &SyncReport{StartedAt: time.Now()}
	defer func() { report.FinishedAt = time.Now() }()
	if m == nil || m.provider == nil {
		return report, errors.New("cloud: SyncManager not initialized")
	}
	res, err := m.provider.Pull(ctx)
	if res != nil {
		report.PulledFrames = res.PulledFrames
		report.Errors = append(report.Errors, res.Errors...)
	}
	if err != nil {
		report.Errors = append(report.Errors, err.Error())
	}
	_ = audit.LogCloudOp("pull", err == nil, m.cfg.Provider, map[string]any{"pulled": report.PulledFrames})
	return report, err
}

// checkBudgetBeforeWrite enforces (a) quota cap, (b) per-hour rate limit,
// (c) replica file size before allowing a write to proceed.
func (m *SyncManager) checkBudgetBeforeWrite(rowCount int) (bool, string) {
	if m == nil {
		return true, ""
	}
	m.quotaMu.RLock()
	quota := m.lastQuota
	m.quotaMu.RUnlock()
	if quota.PercentUsed >= 99 {
		return false, fmt.Sprintf("kuota cloud >=99%% (storage %d/%d MB)", quota.StorageBytes/1024/1024, quota.StorageLimitBytes/1024/1024)
	}
	limit := m.cfg.MaxRowsPerHour
	if limit <= 0 {
		limit = 1000
	}
	if m.rowsThisHour.Load()+int64(rowCount) > int64(limit) {
		return false, fmt.Sprintf("rate-limit lokal: %d rows/jam tercapai", limit)
	}
	if m.replicaPath != "" && m.cfg.MaxStorageMB > 0 {
		if st, err := os.Stat(m.replicaPath); err == nil && st.Size() > int64(m.cfg.MaxStorageMB)*1024*1024 {
			return false, fmt.Sprintf("replika lokal > %d MB; jalankan 'smara memory cleanup'", m.cfg.MaxStorageMB)
		}
	}
	m.rowsThisHour.Add(int64(rowCount))
	return true, ""
}
