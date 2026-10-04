package repository

import (
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	"github.com/Wei-Shaw/sub2api/migrations"
	"github.com/stretchr/testify/require"
)

func TestUpstreamProfitIdentityLegacyMigrationPostgres(t *testing.T) {
	db, ctx, _ := upstreamStorageTestDB(t)
	dayStart := timezone.StartOfDay(time.Now().UTC())
	oldUpdate := dayStart.Add(-time.Hour)
	recentUpdate := time.Now().UTC().Truncate(time.Microsecond)
	_, err := db.ExecContext(ctx, `DROP TRIGGER upstream_profit_identity_changed ON upstream_targets`)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `ALTER TABLE upstream_targets DROP COLUMN profit_identity_since`)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `UPDATE upstream_targets SET updated_at=$1 WHERE id=1`, oldUpdate)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `UPDATE upstream_targets SET updated_at=$1 WHERE id=2`, recentUpdate)
	require.NoError(t, err)
	migration, err := migrations.FS.ReadFile("265_upstream_finance_reported_day.sql")
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, string(migration))
	require.NoError(t, err)
	repo := &upstreamFinanceRepository{db: db}
	oldTarget, err := repo.GetTarget(ctx, 1)
	require.NoError(t, err)
	require.True(t, oldTarget.ProfitIdentitySince.Equal(oldUpdate))
	recentTarget, err := repo.GetTarget(ctx, 2)
	require.NoError(t, err)
	require.True(t, recentTarget.ProfitIdentitySince.Equal(recentUpdate))

	_, err = db.ExecContext(ctx, `UPDATE upstream_targets SET api_key_encrypted='rotated' WHERE id=1`)
	require.NoError(t, err)
	rotated, err := repo.GetTarget(ctx, 1)
	require.NoError(t, err)
	require.True(t, rotated.ProfitIdentitySince.After(dayStart))
	_, err = db.ExecContext(ctx, string(migration))
	require.NoError(t, err)
	stillRotated, err := repo.GetTarget(ctx, 1)
	require.NoError(t, err)
	require.Equal(t, rotated.ProfitIdentitySince, stillRotated.ProfitIdentitySince)
}

func TestUpstreamProfitIdentityAndArchivedSourcesPostgres(t *testing.T) {
	db, ctx, _ := upstreamStorageTestDB(t)
	now := time.Now().UTC()
	dayStart := timezone.StartOfDay(now)
	repo := &upstreamFinanceRepository{db: db}

	// The migration is applied twice by the fixture. Existing targets and newly
	// inserted targets both start with today's identity, never a false full day.
	first, err := repo.GetTarget(ctx, 1)
	require.NoError(t, err)
	require.True(t, first.ProfitIdentitySince.After(dayStart))
	// The shared fixture assigns target IDs explicitly, so advance its sequence.
	_, err = db.ExecContext(ctx, `SELECT setval(pg_get_serial_sequence('upstream_targets', 'id'), (SELECT MAX(id) FROM upstream_targets))`)
	require.NoError(t, err)
	var createdID int64
	err = db.QueryRowContext(ctx, `INSERT INTO upstream_targets(name,provider,endpoint,api_key_encrypted,api_key_fingerprint)
 VALUES('New today','openai','https://new.example','cipher-new','new-key') RETURNING id`).Scan(&createdID)
	require.NoError(t, err)
	created, err := repo.GetTarget(ctx, createdID)
	require.NoError(t, err)
	require.True(t, created.ProfitIdentitySince.After(dayStart))

	_, err = db.ExecContext(ctx, `UPDATE upstream_targets SET name='Renamed' WHERE id=$1`, createdID)
	require.NoError(t, err)
	renamed, err := repo.GetTarget(ctx, createdID)
	require.NoError(t, err)
	require.Equal(t, created.ProfitIdentitySince, renamed.ProfitIdentitySince)
	time.Sleep(time.Millisecond)
	_, err = db.ExecContext(ctx, `UPDATE upstream_targets SET api_key_encrypted='rotated' WHERE id=$1`, createdID)
	require.NoError(t, err)
	rotated, err := repo.GetTarget(ctx, createdID)
	require.NoError(t, err)
	require.True(t, rotated.ProfitIdentitySince.After(renamed.ProfitIdentitySince))

	_, err = db.ExecContext(ctx, `UPDATE upstream_targets SET deleted_at=NOW() WHERE id=$1`, createdID)
	require.NoError(t, err)
	sources, err := repo.LoadProfitSources(ctx, dayStart)
	require.NoError(t, err)
	require.NotNil(t, sources.Targets[createdID])
	require.NotNil(t, sources.Targets[createdID].ArchivedAt)
	require.Nil(t, sources.Balances[createdID])
}
