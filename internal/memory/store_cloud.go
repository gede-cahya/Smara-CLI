// Package memory — cloud-aware bootstrap.
//
// store_cloud.go wires the local memory store, the cloud provider
// registry, and the SyncManager into a single, idempotent bootstrap
// entrypoint. The CLI (`smara memory cloud enable`, `smara start`,
// any future cloud-aware command) calls OpenStoreWithCloud once at
// process start; subsequent restarts re-execute the same path and
// converge on the same on-disk replica without duplicating remote
// resources.
//
// Why a separate file:
//
//   - Local-only callers continue to use NewSQLiteStore /
//     NewSQLiteStoreWithDSN unchanged (Requirement 17.5). Keeping the
//     cloud bootstrap out of store.go preserves that the local-only
//     path has zero dependency on the cloud package.
//   - The bootstrap depends on internal/memory/cloud, internal/config,
//     and the workspace/backfill helpers. Co-locating those pulls into
//     a single file makes the dependency surface easy to audit.
//
// The function is expected to be called at most once per workspace per
// process; the SyncManager it returns owns its own goroutines (Task
// 9.1) and must be stopped on shutdown via `sm.Stop()`.
package memory

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/gede-cahya/Smara-CLI/internal/audit"
	"github.com/gede-cahya/Smara-CLI/internal/config"
	"github.com/gede-cahya/Smara-CLI/internal/memory/cloud"
)

// OpenStoreWithCloud is the cloud-aware constructor that mirrors the
// design document's Function 1 contract (design.md §"Key Functions").
//
// Pipeline (each step propagates a wrapped error on failure so the CLI
// can render an actionable message without parsing strings):
//
//  1. Validate the cloud Config (`ccfg.Validate`). A misconfigured
//     YAML stops here before any remote call is made (Req 11.4).
//
//  2. Resolve the Provider through the registry (`cloud.Get`). An
//     unknown provider name surfaces an error that lists every
//     registered name so the user can correct the YAML without
//     consulting source.
//
//  3. Resolve the per-install device id (`EnsureDeviceID`). The id is
//     stamped on every cloud-synced row's `device_id` column and is
//     also passed to BackfillCloudFields below, so it must be present
//     before any write path runs (Req 8.2).
//
//  4. Load credentials through the composite CredentialStore. A
//     missing credential triggers `cloud.ErrNoCredentials` which we
//     wrap with a message directing the user at
//     `smara memory cloud login` (Req 12.5).
//
//  5. Validate the credentials against the provider. This is a cheap
//     online check that fails fast on a stale / revoked token before
//     we touch the local replica.
//
//  6. Resolve the active workspace name (`cfg.ActiveWorkspace`,
//     defaulting to "default") and call `provider.EnsureDatabase` to
//     obtain (or fetch metadata for) the per-workspace remote DB
//     (Req 5.2).
//
//  7. Open the libSQL embedded replica via `provider.OpenStore`,
//     yielding a DSN that NewSQLiteStoreWithDSN routes to the libsql
//     driver. The store is opened with `CloudEnabled: true` and the
//     resolved device id so subsequent writes stamp cloud columns
//     (Req 6.1).
//
//  8. Run `BackfillCloudFields(deviceID)` to populate cloud_id /
//     device_id / content_hash on any row that pre-dated cloud
//     enablement. The operation is idempotent (Req 8.4) so repeated
//     bootstraps converge on the same state.
//
//  9. UPSERT a row into `cloud_databases` for the active workspace so
//     `smara memory cloud workspaces` can list the live remote DBs
//     without re-hitting the provider on every read (Req 5.4).
//
//  10. Construct a SyncManager and call Start. Until Task 9.1 ships
//     the real implementation Start is a no-op; the libSQL driver
//     already drives ambient replication via the DSN's syncInterval,
//     so correctness is preserved.
//
// Error semantics:
//
//   - On any failure after step 7 (replica opened) the function closes
//     the store before returning so the database/sql connection pool
//     does not leak. The replica file itself is untouched: every
//     migration is strictly additive (Req 6.5) and sync_log /
//     cloud_databases inserts are wrapped in ON CONFLICT clauses, so a
//     half-finished bootstrap leaves the on-disk state safe to retry.
//
//   - The returned (*cloud.SyncManager) is non-nil only when the
//     entire pipeline succeeds; on error the caller receives nil for
//     both the store and the manager so it cannot accidentally use a
//     half-initialised value.
func OpenStoreWithCloud(ctx context.Context, cfg *config.SmaraConfig, ccfg cloud.Config) (store *SQLiteStore, sm *cloud.SyncManager, err error) {
	// Audit hook: every cloud-enable bootstrap is logged with target=
	// workspace_name so operators can trace which workspace was wired up
	// and when. We use named return values so the deferred call observes
	// the final (wrapped) error regardless of which step in the pipeline
	// failed; the success bool is derived from err == nil at exit time.
	//
	// Per requirement 16.1/16.3 the entry MUST NOT include the token,
	// row content, or embeddings. We record only the resolved provider
	// name and (when known) the database name so a future "enable"
	// followed by "disable" pair can be reconciled in the audit log.
	var (
		auditWorkspace = ""
		auditDBName    string
	)
	if cfg != nil {
		auditWorkspace = strings.TrimSpace(cfg.ActiveWorkspace)
	}
	defer func() {
		extra := map[string]any{
			"provider": ccfg.Provider,
		}
		if auditDBName != "" {
			extra["db_name"] = auditDBName
		}
		if err != nil {
			extra["error"] = err.Error()
		}
		_ = audit.LogCloudOp("enable", err == nil, auditWorkspace, extra)
	}()

	if cfg == nil {
		return nil, nil, errors.New("OpenStoreWithCloud: cfg is nil")
	}

	// 1. Validate cloud config (provider registered, policy in enum, etc).
	if err := ccfg.Validate(); err != nil {
		return nil, nil, fmt.Errorf("OpenStoreWithCloud: invalid cloud config: %w", err)
	}

	// 2. Resolve provider factory and instantiate it. Get already lists
	// registered providers in its error message when the name is unknown.
	provider, err := cloud.Get(ccfg.Provider)
	if err != nil {
		return nil, nil, fmt.Errorf("OpenStoreWithCloud: resolve provider %q: %w", ccfg.Provider, err)
	}

	// 2a. If EncryptAtRest is enabled, load (or generate) the encryption
	// key and wrap the provider in an EncryptedProvider so all content
	// is encrypted before leaving the local device.
	if ccfg.EncryptAtRest {
		encKey, keyInfo, keyErr := cloud.EnsureEncryptionKey()
		if keyErr != nil {
			return nil, nil, fmt.Errorf("OpenStoreWithCloud: encryption at rest enabled but key unavailable: %w", keyErr)
		}
		_ = keyInfo // keyInfo.Source is available for audit logging if needed
		ccfg.EncryptionKey = string(encKey) // populate for Validate
		provider = cloud.NewEncryptedProvider(provider, encKey)
		// Zero the local copy after wrapping.
		for i := range encKey {
			encKey[i] = 0
		}
	}

	// Wire the cloud config into the provider so EnsureDatabase, OpenStore,
	// and sync operations have access to the configured DBNamePattern,
	// SyncIntervalSec, and ConflictPolicy (task 8.2 / 8.4 integration).
	if cfgProvider, ok := provider.(interface{ WithConfig(cloud.Config) }); ok {
		cfgProvider.WithConfig(ccfg)
	}

	// 3. Resolve the per-install device id. Every cloud-synced write
	// needs this so the row can be attributed back to its origin.
	deviceID, err := EnsureDeviceID()
	if err != nil {
		return nil, nil, fmt.Errorf("OpenStoreWithCloud: resolve device id: %w", err)
	}

	// 4. Load credentials. ErrNoCredentials → user-facing direction.
	creds, err := cloud.NewCredentialStore().Load()
	if err != nil {
		if errors.Is(err, cloud.ErrNoCredentials) {
			return nil, nil, fmt.Errorf(
				"OpenStoreWithCloud: no cloud credentials available; run `smara memory cloud login` to authenticate (or set %s in the environment for headless mode): %w",
				"SMARA_CLOUD_TOKEN", err,
			)
		}
		return nil, nil, fmt.Errorf("OpenStoreWithCloud: load credentials: %w", err)
	}

	// 5. Cheap online check: catch stale/revoked tokens before we touch
	// the local replica or create a remote database.
	if err := provider.ValidateCredentials(ctx, creds); err != nil {
		return nil, nil, fmt.Errorf("OpenStoreWithCloud: validate credentials: %w", err)
	}

	// 6. Resolve active workspace and ensure the remote database. We
	// default to "default" so a fresh install with an empty
	// active_workspace key still produces a valid db name when the
	// pattern includes {workspace}.
	workspaceName := strings.TrimSpace(cfg.ActiveWorkspace)
	if workspaceName == "" {
		workspaceName = "default"
	}
	dbInfo, err := provider.EnsureDatabase(ctx, creds, workspaceName)
	if err != nil {
		return nil, nil, fmt.Errorf("OpenStoreWithCloud: ensure remote database for workspace %q: %w", workspaceName, err)
	}
	if dbInfo != nil {
		auditDBName = dbInfo.Name
	}
	// Refresh the audit workspace label using the resolved (possibly
	// defaulted) workspace name so the "enable" entry always names the
	// concrete workspace that was bootstrapped, not the empty string a
	// fresh install would otherwise emit.
	auditWorkspace = workspaceName

	// 7. Build the libSQL embedded-replica DSN and open the store with
	// cloud-aware options. NewSQLiteStoreWithDSN runs Init/migrate, so
	// schema parity with the remote is guaranteed before we return.
	dsn, err := provider.OpenStore(ctx, dbInfo, cfg.DBPath)
	if err != nil {
		return nil, nil, fmt.Errorf("OpenStoreWithCloud: open libSQL replica at %q: %w", cfg.DBPath, err)
	}
	store, err = NewSQLiteStoreWithDSN(dsn, StoreOptions{
		DeviceID:     deviceID,
		CloudEnabled: true,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("OpenStoreWithCloud: open SQLite store from DSN: %w", err)
	}

	// 8. Idempotent backfill of cloud columns for any pre-cloud rows.
	// Re-running OpenStoreWithCloud after a successful boot is a no-op
	// because the WHERE clause filters on cloud_id IS NULL.
	if _, err := store.BackfillCloudFields(deviceID); err != nil {
		_ = store.Close()
		return nil, nil, fmt.Errorf("OpenStoreWithCloud: backfill cloud fields: %w", err)
	}

	// 9. Record / refresh the workspace ↔ remote DB mapping. We resolve
	// the local workspace row by name so the UPSERT target matches the
	// row CreateWorkspace produced. A missing local row (e.g. fresh
	// install where the active workspace was never created) is not
	// fatal — we skip the UPSERT and surface a warning context in the
	// returned error if the caller wants to log it.
	if err := store.UpsertCloudDatabase(workspaceName, ccfg.Provider, dbInfo); err != nil {
		_ = store.Close()
		return nil, nil, fmt.Errorf("OpenStoreWithCloud: upsert cloud_databases row for workspace %q: %w", workspaceName, err)
	}

	// Wire the replica DB handle into the provider so Push/Pull/Status
	// can interact with sync_log and the replica's SQL-level sync
	// primitives (task 8.4 / 9.1 integration).
	if rp, ok := provider.(interface{ SetReplicaDB(*sql.DB) }); ok {
		rp.SetReplicaDB(store.DB())
	}

	// 10. Construct and start the SyncManager. Set replica path for budget
	// enforcement (MaxStorageMB) and launch periodic sync if configured.
	sm = cloud.NewSyncManager(provider, store, ccfg)
	sm.SetReplicaPath(cfg.DBPath)
	if err := sm.Start(ctx); err != nil {
		_ = store.Close()
		return nil, nil, fmt.Errorf("OpenStoreWithCloud: start SyncManager: %w", err)
	}

	return store, sm, nil
}

// UpsertCloudDatabase records (or refreshes) the local mapping from a
// workspace to its remote database in the `cloud_databases` table.
//
// Behaviour:
//
//   - The workspace identified by `workspaceName` is resolved through
//     GetWorkspaceByName. When the workspace does not yet exist locally
//     we transparently create it (path is left empty) so first-time
//     bootstraps on a fresh install do not require the user to manually
//     run `smara workspace create` before `smara memory cloud enable`.
//
//   - On INSERT conflict against the UNIQUE (workspace_id) constraint
//     the existing row is updated in place: provider / db_name / db_url
//     / region all reflect the current authoritative metadata. This is
//     what makes repeated OpenStoreWithCloud calls converge.
//
//   - The row's `created_at` is populated by SQLite's DEFAULT clause on
//     first insert and is preserved by ON CONFLICT (we only update the
//     dynamic columns), so subsequent bootstraps do not falsely advance
//     the workspace creation timestamp.
//
// Errors are wrapped with the workspace name so the caller can surface
// a single actionable diagnostic.
func (s *SQLiteStore) UpsertCloudDatabase(workspaceName, providerName string, info *cloud.DatabaseInfo) error {
	if info == nil {
		return errors.New("UpsertCloudDatabase: nil DatabaseInfo")
	}
	if strings.TrimSpace(workspaceName) == "" {
		return errors.New("UpsertCloudDatabase: empty workspace name")
	}

	ws, err := s.GetWorkspaceByName(workspaceName)
	if err != nil {
		return fmt.Errorf("UpsertCloudDatabase: lookup workspace %q: %w", workspaceName, err)
	}
	if ws == nil {
		// Auto-create the local workspace row so the FK on
		// cloud_databases.workspace_id resolves. Path is empty because
		// we have no on-disk project context at bootstrap time; users
		// can repoint it later via `smara workspace create`.
		ws, err = s.CreateWorkspace(workspaceName, "")
		if err != nil {
			return fmt.Errorf("UpsertCloudDatabase: auto-create workspace %q: %w", workspaceName, err)
		}
	}

	// SQLite's UPSERT clause (`ON CONFLICT ... DO UPDATE`) requires the
	// conflict target to match a UNIQUE / PRIMARY KEY index — the
	// `workspace_id` column has a UNIQUE constraint declared in
	// migrate(), so the conflict target is `(workspace_id)`.
	//
	// We only refresh dynamic columns; created_at is retained from the
	// pre-existing row by omitting it from the SET list, so the column
	// continues to reflect the moment the mapping was first recorded.
	const stmt = `
		INSERT INTO cloud_databases
			(workspace_id, provider, db_name, db_url, region)
		VALUES
			(?, ?, ?, ?, ?)
		ON CONFLICT(workspace_id) DO UPDATE SET
			provider = excluded.provider,
			db_name  = excluded.db_name,
			db_url   = excluded.db_url,
			region   = excluded.region
	`
	if _, err := s.db.Exec(stmt, ws.ID, providerName, info.Name, info.URL, info.Region); err != nil {
		return fmt.Errorf("UpsertCloudDatabase: exec UPSERT for workspace %q (id=%d): %w", workspaceName, ws.ID, err)
	}
	return nil
}
