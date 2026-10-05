package repository

import (
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/Wei-Shaw/sub2api/migrations"
	"github.com/stretchr/testify/require"
)

func TestUpstreamProfitPostgresArchivePreservesRemainingTotals(t *testing.T) {
	db, ctx, _ := upstreamStorageTestDB(t)
	repo := &upstreamFinanceRepository{db: db}
	now := time.Now().UTC().Truncate(time.Microsecond)
	start := timezone.StartOfDay(now)
	end := start.AddDate(0, 0, 1)
	periodStart := start.AddDate(0, 0, -29)
	_, err := db.ExecContext(ctx, `UPDATE upstream_targets SET supplier_id=1 WHERE id=2;
 UPDATE upstream_suppliers SET recharge_ratio=10 WHERE id=1`)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `INSERT INTO upstream_finance_ledger(usage_id,created_at,target_id,target_name,supplier_id,account_id,user_id,api_key_id,model,revenue,business_cost,billing_type,total_tokens) VALUES
 (1,$1,1,'First',1,1,1,1,'m',2,999,0,100),
 (2,$1,2,'Second',1,1,1,1,'m',4,999,0,100),
 (3,$1,3,'Archived',2,1,1,1,'m',100,999,0,100)`, now)
	require.NoError(t, err)
	for _, id := range []int64{1, 2} {
		target, err := repo.GetTarget(ctx, id)
		require.NoError(t, err)
		_, err = db.ExecContext(ctx, `INSERT INTO upstream_balance_snapshots(target_id,identity_hash,wallet_ref,kind,currency,status,synced_at,day_used,day_start,day_end,last_30_days_used,period_start,period_end)
 VALUES($1,$2,'default','wallet','USD','ok',$3,10,$4,$5,10,$6,$5)`, id, service.UpstreamBalanceIdentity(target), now, start, end, periodStart)
		require.NoError(t, err)
	}
	finance := service.NewUpstreamFinanceService(repo, nil, nil, nil, nil)
	before, err := finance.PeriodSummaries(ctx, nil, nil)
	require.NoError(t, err)
	for _, summary := range []*service.UpstreamFinanceSummary{before.Today, before.Last30Days} {
		require.Equal(t, 6.0, summary.Revenue)
		require.Equal(t, 2.0, *summary.RemoteUsed)
		require.Equal(t, 4.0, *summary.Profit)
	}
	_, err = db.ExecContext(ctx, `UPDATE upstream_targets SET deleted_at=NOW() WHERE id=2`)
	require.NoError(t, err)
	after, err := finance.PeriodSummaries(ctx, nil, nil)
	require.NoError(t, err)
	for _, summary := range []*service.UpstreamFinanceSummary{after.Today, after.Last30Days} {
		require.Equal(t, 2.0, summary.Revenue)
		require.Equal(t, 1.0, *summary.RemoteUsed)
		require.Equal(t, 1.0, *summary.Profit)
		require.False(t, summary.CostPartial)
	}
	supplierID := int64(1)
	data, err := finance.OverviewFinance(ctx, []*service.UpstreamSupplier{{ID: 1}}, []*service.UpstreamTarget{{ID: 1, SupplierID: &supplierID}}, start, end)
	require.NoError(t, err)
	require.Equal(t, after.Today.Revenue, data.Summary.Revenue)
	require.Equal(t, after.Today.Profit, data.Summary.Profit)
	var rows int
	require.NoError(t, db.QueryRowContext(ctx, `SELECT COUNT(*) FROM upstream_finance_ledger`).Scan(&rows))
	require.Equal(t, 3, rows, "archiving changes summary scope without deleting any ledger history")
}

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
