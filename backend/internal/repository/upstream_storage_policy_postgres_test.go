package repository

import (
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/Wei-Shaw/sub2api/migrations"
	"github.com/lib/pq"
	"github.com/stretchr/testify/require"
)

// All DDL and mutations are scoped to the random schema owned by this test.
func TestUpstreamStoragePolicyPostgres(t *testing.T) {
	db, ctx, _ := upstreamStorageTestDB(t)
	initial, err := migrations.FS.ReadFile("255_upstream_storage_policy.sql")
	require.NoError(t, err)
	migration, err := migrations.FS.ReadFile("278_upstream_monitor_retention.sql")
	require.NoError(t, err)
	// Mirror the migration runner: the initial singleton and its migration
	// marker receive the same transaction timestamp.
	tx, err := db.BeginTx(ctx, nil)
	require.NoError(t, err)
	_, err = tx.ExecContext(ctx, string(initial))
	require.NoError(t, err)
	_, err = tx.ExecContext(ctx, `CREATE TABLE schema_migrations(filename TEXT PRIMARY KEY, applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW());
 INSERT INTO schema_migrations(filename) VALUES('255_upstream_storage_policy.sql')`)
	require.NoError(t, err)
	require.NoError(t, tx.Commit())
	_, err = db.ExecContext(ctx, string(migration))
	require.NoError(t, err)
	repo := &upstreamCenterRepository{db: db}

	policy, err := repo.GetStoragePolicy(ctx)
	require.NoError(t, err)
	require.Equal(t, &service.UpstreamStoragePolicy{Enabled: true, HistoryRetentionDays: 3, SnapshotRetentionDays: 7}, policy)
	var defaultHistory int
	require.NoError(t, db.QueryRowContext(ctx, `UPDATE upstream_storage_policy SET history_retention_days=DEFAULT WHERE id=1 RETURNING history_retention_days`).Scan(&defaultHistory))
	require.Equal(t, 3, defaultHistory)

	for _, values := range []struct {
		enabled           bool
		history, snapshot int
	}{{true, 3, 1}, {false, 365, 90}, {false, 30, 14}} {
		input := service.UpstreamStoragePolicyInput{Enabled: &values.enabled, HistoryRetentionDays: &values.history, SnapshotRetentionDays: &values.snapshot}
		saved, err := repo.SaveStoragePolicy(ctx, input)
		require.NoError(t, err)
		require.Equal(t, &service.UpstreamStoragePolicy{Enabled: values.enabled, HistoryRetentionDays: values.history, SnapshotRetentionDays: values.snapshot}, saved)
		policy, err = repo.GetStoragePolicy(ctx)
		require.NoError(t, err)
		require.Equal(t, saved, policy)
	}

	at := time.Date(2026, 9, 26, 12, 0, 0, 123456000, time.UTC)
	result := &service.UpstreamStorageCleanupResult{HistoryDeleted: 5000, BalanceDeleted: 2, BillingDeleted: 3, HasMore: true}
	require.NoError(t, repo.RecordStorageCleanup(ctx, at, result))
	policy, err = repo.GetStoragePolicy(ctx)
	require.NoError(t, err)
	require.NotNil(t, policy.LastCleanupAt)
	require.True(t, at.Equal(*policy.LastCleanupAt))
	require.Equal(t, result, policy.LastResult)
	current := policy

	// A slow earlier batch cannot replace the newer cleanup time or backlog flag.
	require.NoError(t, repo.RecordStorageCleanup(ctx, at.Add(-time.Minute), &service.UpstreamStorageCleanupResult{}))
	policy, err = repo.GetStoragePolicy(ctx)
	require.NoError(t, err)
	require.Equal(t, current, policy)

	at = at.Add(time.Minute)
	result = &service.UpstreamStorageCleanupResult{HistoryDeleted: 13, HasMore: false}
	require.NoError(t, repo.RecordStorageCleanup(ctx, at, result))
	policy, err = repo.GetStoragePolicy(ctx)
	require.NoError(t, err)
	require.True(t, at.Equal(*policy.LastCleanupAt))
	require.Equal(t, result, policy.LastResult)
	current = policy

	for _, statement := range []string{
		`INSERT INTO upstream_storage_policy(id) VALUES(2)`,
		`UPDATE upstream_storage_policy SET history_retention_days=2 WHERE id=1`,
		`UPDATE upstream_storage_policy SET history_retention_days=366 WHERE id=1`,
		`UPDATE upstream_storage_policy SET snapshot_retention_days=0 WHERE id=1`,
		`UPDATE upstream_storage_policy SET snapshot_retention_days=91 WHERE id=1`,
	} {
		_, err := db.ExecContext(ctx, statement)
		var constraint *pq.Error
		require.ErrorAs(t, err, &constraint, statement)
		require.Equal(t, pq.ErrorCode("23514"), constraint.Code, statement)
	}
	_, err = db.ExecContext(ctx, `UPDATE upstream_storage_policy SET enabled=NULL WHERE id=1`)
	var notNull *pq.Error
	require.ErrorAs(t, err, &notNull)
	require.Equal(t, pq.ErrorCode("23502"), notNull.Code)
	policy, err = repo.GetStoragePolicy(ctx)
	require.NoError(t, err)
	require.Equal(t, current, policy, "rejected writes must preserve both settings and cleanup history")

	// Re-running the migration must not reset the singleton or its recorded result.
	_, err = db.ExecContext(ctx, string(migration))
	require.NoError(t, err)
	policy, err = repo.GetStoragePolicy(ctx)
	require.NoError(t, err)
	require.Equal(t, current, policy)
	var count int
	require.NoError(t, db.QueryRowContext(ctx, `SELECT COUNT(*) FROM upstream_storage_policy`).Scan(&count))
	require.Equal(t, 1, count)
}
