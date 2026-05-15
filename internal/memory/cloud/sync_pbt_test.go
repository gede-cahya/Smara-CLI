package cloud

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"pgregory.net/rapid"
)

type pbtProvider struct {
	status SyncStatus
	pushes int
}

func (p *pbtProvider) Name() string                                              { return "pbt" }
func (p *pbtProvider) Login(context.Context, LoginOptions) (*Credentials, error) { return nil, nil }
func (p *pbtProvider) ValidateCredentials(context.Context, *Credentials) error   { return nil }
func (p *pbtProvider) EnsureDatabase(context.Context, *Credentials, string) (*DatabaseInfo, error) {
	return nil, nil
}
func (p *pbtProvider) OpenStore(context.Context, *DatabaseInfo, string) (string, error) {
	return "", nil
}
func (p *pbtProvider) Push(context.Context) (*SyncReport, error) {
	p.pushes++
	return &SyncReport{PushedRows: 1}, nil
}
func (p *pbtProvider) Pull(context.Context) (*SyncReport, error) {
	return &SyncReport{PulledFrames: 1}, nil
}
func (p *pbtProvider) Status(context.Context) (*SyncStatus, error) { return &p.status, nil }
func (p *pbtProvider) ListWorkspaceDatabases(context.Context, *Credentials) ([]DatabaseInfo, error) {
	return nil, nil
}
func (p *pbtProvider) DeleteWorkspaceDatabase(context.Context, *Credentials, string) error {
	return nil
}
func (p *pbtProvider) Close() error { return nil }

type pbtStore struct{ conflicts []CloudConflict }

func (s *pbtStore) DB() *sql.DB                                                        { return nil }
func (s *pbtStore) GetMemoryByCloudID(string) (*MemoryRow, error)                      { return nil, nil }
func (s *pbtStore) UpdateMemoryFromConflict(int64, MemoryRow, *MemoryVersionRow) error { return nil }
func (s *pbtStore) InsertCloudConflict(CloudConflict) error                            { return nil }
func (s *pbtStore) ListUnresolvedConflicts() ([]CloudConflict, error)                  { return s.conflicts, nil }
func (s *pbtStore) MarkConflictResolved(int64, string) error                           { return nil }

func TestQuotaSafetyPBT(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		maxRows := rapid.IntRange(100, 500).Draw(t, "maxRows")
		percent := rapid.Float64Range(99, 120).Draw(t, "percent")
		m := NewSyncManager(&pbtProvider{}, nil, Config{Provider: "pbt", MaxRowsPerHour: maxRows, MaxStorageMB: 8000})
		m.setQuota(QuotaInfo{PercentUsed: percent, StorageBytes: 99, StorageLimitBytes: 100})
		allowed, reason := m.checkBudgetBeforeWrite(1)
		require.False(t, allowed)
		require.Contains(t, reason, "kuota")
		require.Equal(t, int64(0), m.rowsThisHour.Load())

		m.setQuota(QuotaInfo{PercentUsed: 10, StorageBytes: 10, StorageLimitBytes: 100})
		m.rowsThisHour.Store(int64(maxRows))
		allowed, reason = m.checkBudgetBeforeWrite(1)
		require.False(t, allowed)
		require.Contains(t, reason, "rate-limit")
		require.Equal(t, int64(maxRows), m.rowsThisHour.Load())

		m.rowsThisHour.Store(0)
		rowCount := rapid.IntRange(1, maxRows).Draw(t, "rowCount")
		allowed, reason = m.checkBudgetBeforeWrite(rowCount)
		require.True(t, allowed, reason)
		require.Equal(t, int64(rowCount), m.rowsThisHour.Load())
	})
}

func TestEventualConsistencyPBT(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		interval := rapid.IntRange(1, 5).Draw(t, "interval")
		ops := rapid.SliceOfN(rapid.SampledFrom([]string{"A", "B"}), 1, 25).Draw(t, "ops")
		seenA := map[int]bool{}
		seenB := map[int]bool{}
		primary := map[int]bool{}
		for i, dev := range ops {
			primary[i] = true
			if dev == "A" {
				seenA[i] = true
			} else {
				seenB[i] = true
			}
			deadline := time.Duration(2*interval) * time.Second
			require.LessOrEqual(t, deadline, 10*time.Second)
			for id := range primary {
				seenA[id] = true
				seenB[id] = true
			}
		}
		for id := range primary {
			require.True(t, seenA[id])
			require.True(t, seenB[id])
		}
	})
}
