package repository

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/Wei-Shaw/sub2api/migrations"
	"github.com/stretchr/testify/require"
)

func applyUpstreamHistoryQueueMigration(t *testing.T, db *sql.DB, ctx context.Context) {
	t.Helper()
	applyUpstreamAccountHistoryMigration(t, db, ctx)
	migration, err := migrations.FS.ReadFile("271_upstream_account_history_queue.sql")
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, string(migration))
	require.NoError(t, err)
}

func drainUpstreamHistoryQueue(t *testing.T, db *sql.DB, ctx context.Context) {
	t.Helper()
	repo := &upstreamFinanceRepository{db: db}
	for i := 0; i < 20; i++ {
		worked, err := repo.BackfillAccountHistoryBatch(ctx)
		require.NoError(t, err)
		if !worked {
			return
		}
	}
	t.Fatal("history queue did not drain")
}

func TestUpstreamHistoryQueuePostgresBoundedResumeAndWindowStatus(t *testing.T) {
	db, ctx, _ := upstreamAccountHistoryTestDB(t)
	applyUpstreamHistoryQueueMigration(t, db, ctx)
	now := time.Now().UTC()
	today := now.Truncate(24 * time.Hour)
	_, err := db.ExecContext(ctx, `INSERT INTO usage_logs(id,created_at,account_id,user_id,api_key_id,model,actual_cost,total_cost,billing_type,input_tokens)
 SELECT 10000+n,$1::timestamptz-make_interval(secs=>n),101,11,21,'history',2,1,0,10 FROM generate_series(1,2505) n`, today.Add(-time.Hour))
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `INSERT INTO upstream_account_bindings(target_id,account_id,supplier_id,target_name,supplier_name) VALUES(1,101,1,'Supplier key','First')`)
	require.NoError(t, err)
	var rows int64
	require.NoError(t, db.QueryRowContext(ctx, `SELECT COUNT(*) FROM upstream_finance_ledger`).Scan(&rows))
	require.Zero(t, rows, "the HTTP save only enqueues; it never copies historical rows")
	repo := &upstreamFinanceRepository{db: db}
	pending, err := repo.PendingAccountHistory(ctx, today, today.AddDate(0, 0, 1))
	require.NoError(t, err)
	require.Len(t, pending, 1)
	worked, err := repo.BackfillAccountHistoryBatch(ctx) // initialize write barrier, no history I/O
	require.NoError(t, err)
	require.True(t, worked)
	require.NoError(t, db.QueryRowContext(ctx, `SELECT COUNT(*) FROM upstream_finance_ledger`).Scan(&rows))
	require.Zero(t, rows)
	worked, err = repo.BackfillAccountHistoryBatch(ctx)
	require.NoError(t, err)
	require.True(t, worked)
	require.NoError(t, db.QueryRowContext(ctx, `SELECT COUNT(*) FROM upstream_finance_ledger`).Scan(&rows))
	require.Equal(t, int64(1000), rows)
	// Existing account/time indexes must seek directly to the saved time cursor.
	// No expensive replacement index is needed when upgrading a large usage table.
	_, err = db.ExecContext(ctx, `ANALYZE usage_logs`)
	require.NoError(t, err)
	plan, err := db.QueryContext(ctx, `EXPLAIN (COSTS OFF) SELECT u.*
 FROM upstream_account_history_jobs j JOIN upstream_account_bindings b ON b.id=j.binding_id
 CROSS JOIN LATERAL (SELECT u.* FROM usage_logs u WHERE u.account_id=b.account_id
 AND u.created_at >= j.lower_bound AND u.created_at<LEAST(j.upper_bound,b.valid_until)
 AND u.created_at<=COALESCE(j.cursor_at,j.upper_bound)
 AND (j.cursor_at IS NULL OR (u.created_at,u.id)<(j.cursor_at,j.cursor_id))
 ORDER BY u.created_at DESC,u.id DESC LIMIT 1000 FOR SHARE OF u) u`)
	require.NoError(t, err)
	var explain []string
	for plan.Next() {
		var line string
		require.NoError(t, plan.Scan(&line))
		explain = append(explain, line)
	}
	require.NoError(t, plan.Err())
	require.NoError(t, plan.Close())
	t.Log(strings.Join(explain, "\n"))
	pending, err = repo.PendingAccountHistory(ctx, today, today.AddDate(0, 0, 1))
	require.NoError(t, err)
	require.Empty(t, pending, "once the newest page is below today, older work cannot hold today's profit pending")
	pending, err = repo.PendingAccountHistory(ctx, today.AddDate(0, 0, -29), today.AddDate(0, 0, 1))
	require.NoError(t, err)
	require.Len(t, pending, 1)
	// A fresh repository simulates restart: the durable cursor resumes at row 1001.
	repo = &upstreamFinanceRepository{db: db}
	worked, err = repo.BackfillAccountHistoryBatch(ctx)
	require.NoError(t, err)
	require.True(t, worked)
	require.NoError(t, db.QueryRowContext(ctx, `SELECT COUNT(*) FROM upstream_finance_ledger`).Scan(&rows))
	require.Equal(t, int64(2000), rows)
	// Correct already imported and not-yet-imported records while the job runs.
	_, err = db.ExecContext(ctx, `UPDATE usage_logs SET actual_cost=7 WHERE id IN(10001,12505)`)
	require.NoError(t, err)
	drainUpstreamHistoryQueue(t, db, ctx)
	var revenue float64
	require.NoError(t, db.QueryRowContext(ctx, `SELECT COUNT(*),SUM(revenue) FROM upstream_finance_ledger`).Scan(&rows, &revenue))
	require.Equal(t, int64(2505), rows)
	require.Equal(t, 5020.0, revenue)
	pending, err = repo.PendingAccountHistory(ctx, today.AddDate(0, 0, -29), today.AddDate(0, 0, 1))
	require.NoError(t, err)
	require.Empty(t, pending)
	// Migration replay and retry must not rescan or duplicate completed jobs.
	migration, err := migrations.FS.ReadFile("271_upstream_account_history_queue.sql")
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, string(migration))
	require.NoError(t, err)
	worked, err = repo.BackfillAccountHistoryBatch(ctx)
	require.NoError(t, err)
	require.False(t, worked)
}

func TestUpstreamHistoryQueuePostgresArchiveBoundariesWhilePending(t *testing.T) {
	db, ctx, _ := upstreamAccountHistoryTestDB(t)
	applyUpstreamHistoryQueueMigration(t, db, ctx)
	now := time.Now().UTC()
	closed := now.Add(-2 * time.Hour)
	_, err := db.ExecContext(ctx, `INSERT INTO upstream_account_bindings(target_id,account_id,supplier_id,target_name,supplier_name,valid_from,valid_until)
 VALUES(3,101,2,'Old key','Second',$1,$2)`, now.Add(-3*time.Hour), closed)
	require.NoError(t, err)
	insertUpstreamAccountHistoryUsage(t, db, ctx, 5001, 101, closed.Add(-time.Minute), 20)
	insertUpstreamAccountHistoryUsage(t, db, ctx, 5002, 101, closed.Add(time.Minute), 3)
	_, err = db.ExecContext(ctx, `INSERT INTO upstream_account_bindings(target_id,account_id,supplier_id,target_name,supplier_name,valid_from)
 VALUES(1,101,1,'Supplier key','First',$1)`, now.Add(-time.Hour))
	require.NoError(t, err)
	// Archive and restore before the first history page has even run.
	_, err = db.ExecContext(ctx, `UPDATE upstream_account_bindings SET valid_until=$1 WHERE valid_until IS NULL`, now.Add(-30*time.Minute))
	require.NoError(t, err)
	gap := now.Add(-20 * time.Minute)
	insertUpstreamAccountHistoryUsage(t, db, ctx, 5003, 101, gap, 999)
	_, err = db.ExecContext(ctx, `INSERT INTO upstream_account_bindings(target_id,account_id,supplier_id,target_name,supplier_name,valid_from)
 VALUES(1,101,1,'Supplier key','First',$1)`, now.Add(-10*time.Minute))
	require.NoError(t, err)
	insertUpstreamAccountHistoryUsage(t, db, ctx, 5004, 101, gap, 999)
	drainUpstreamHistoryQueue(t, db, ctx)
	var owner int64
	require.NoError(t, db.QueryRowContext(ctx, `SELECT target_id FROM upstream_finance_ledger WHERE usage_id=5001`).Scan(&owner))
	require.Equal(t, int64(3), owner)
	require.NoError(t, db.QueryRowContext(ctx, `SELECT target_id FROM upstream_finance_ledger WHERE usage_id=5002`).Scan(&owner))
	require.Equal(t, int64(1), owner)
	var rows int64
	require.NoError(t, db.QueryRowContext(ctx, `SELECT COUNT(*) FROM upstream_finance_ledger WHERE usage_id IN(5003,5004)`).Scan(&rows))
	require.Zero(t, rows, "retained and late writes during the archived gap remain excluded")
}

func TestUpstreamHistoryQueuePostgresOldBindingKeepsRecentScanBound(t *testing.T) {
	db, ctx, _ := upstreamAccountHistoryTestDB(t)
	applyUpstreamAccountHistoryMigration(t, db, ctx)
	_, err := db.ExecContext(ctx, `ALTER TABLE upstream_account_bindings DISABLE TRIGGER upstream_account_finance_history_import;
 ALTER TABLE usage_logs DISABLE TRIGGER upstream_usage_finance_snapshot;
 INSERT INTO upstream_account_bindings(target_id,account_id,supplier_id,target_name,supplier_name,valid_from)
 VALUES(1,101,1,'Supplier key','First',clock_timestamp()-INTERVAL '1 year');
 INSERT INTO usage_logs(id,created_at,account_id,user_id,api_key_id,model,actual_cost,total_cost,billing_type)
 VALUES(6001,clock_timestamp()-INTERVAL '40 days',101,11,21,'expired',999,1,0),
 (6002,clock_timestamp()-INTERVAL '1 day',101,11,21,'recent',2,1,0);
 ALTER TABLE upstream_account_bindings ENABLE TRIGGER upstream_account_finance_history_import;
 ALTER TABLE usage_logs ENABLE TRIGGER upstream_usage_finance_snapshot;`)
	require.NoError(t, err)
	migration, err := migrations.FS.ReadFile("271_upstream_account_history_queue.sql")
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, string(migration))
	require.NoError(t, err)
	drainUpstreamHistoryQueue(t, db, ctx)
	var rows int64
	var revenue float64
	require.NoError(t, db.QueryRowContext(ctx, `SELECT COUNT(*),SUM(revenue) FROM upstream_finance_ledger`).Scan(&rows, &revenue))
	require.Equal(t, int64(1), rows)
	require.Equal(t, 2.0, revenue, "upgrading a long-lived binding imports retained recent history only")
	var preservesLiveInterval bool
	require.NoError(t, db.QueryRowContext(ctx, `SELECT history_from=valid_from FROM upstream_account_bindings WHERE account_id=101`).Scan(&preservesLiveInterval))
	require.True(t, preservesLiveInterval, "the bounded backfill does not truncate the original live accounting interval")
}

func TestUpstreamHistoryQueuePostgresFinancialReadSnapshot(t *testing.T) {
	db, ctx, otherDB := upstreamAccountHistoryTestDB(t)
	applyUpstreamHistoryQueueMigration(t, db, ctx)
	other := otherDB()
	now := time.Now()
	insertUpstreamAccountHistoryUsage(t, db, ctx, 7001, 101, now.Add(-time.Hour), 2)
	repo := &upstreamFinanceRepository{db: db}
	query := service.UpstreamFinanceQuery{From: now.Add(-24 * time.Hour), To: now.Add(time.Hour)}
	err := repo.WithFinanceReadSnapshot(ctx, func(readCtx context.Context) error {
		pending, err := repo.PendingAccountHistory(readCtx, query.From, query.To)
		require.NoError(t, err)
		require.Empty(t, pending)
		_, err = other.ExecContext(ctx, `INSERT INTO upstream_account_bindings(target_id,account_id,supplier_id,target_name,supplier_name) VALUES(1,101,1,'Supplier key','First')`)
		require.NoError(t, err)
		drainUpstreamHistoryQueue(t, other, ctx)
		// The entire job starts and finishes between reads. Income and pending
		// still both reflect the snapshot taken before it, never a partial mix.
		summary, err := repo.Summary(readCtx, query)
		require.NoError(t, err)
		require.Zero(t, summary.Revenue)
		return nil
	})
	require.NoError(t, err)
	summary, err := repo.Summary(ctx, query)
	require.NoError(t, err)
	require.Equal(t, 2.0, summary.Revenue)
	insertUpstreamAccountHistoryUsage(t, db, ctx, 7002, 102, now.Add(-time.Hour), 3)
	_, err = db.ExecContext(ctx, `INSERT INTO upstream_account_bindings(target_id,account_id,supplier_id,target_name,supplier_name) VALUES(1,102,1,'Supplier key','First')`)
	require.NoError(t, err)
	err = repo.WithFinanceReadSnapshot(ctx, func(readCtx context.Context) error {
		pending, err := repo.PendingAccountHistory(readCtx, query.From, query.To)
		require.NoError(t, err)
		require.Len(t, pending, 1)
		drainUpstreamHistoryQueue(t, other, ctx)
		summary, err := repo.Summary(readCtx, query)
		require.NoError(t, err)
		require.Equal(t, 2.0, summary.Revenue)
		pending, err = repo.PendingAccountHistory(readCtx, query.From, query.To)
		require.NoError(t, err)
		require.Len(t, pending, 1, "a completed job stays pending in the same snapshot as its partial income")
		return nil
	})
	require.NoError(t, err)
	summary, err = repo.Summary(ctx, query)
	require.NoError(t, err)
	require.Equal(t, 5.0, summary.Revenue)
}

func TestUpstreamHistoryQueuePostgresSaveDoesNotWaitForUsage(t *testing.T) {
	db, ctx, otherDB := upstreamAccountHistoryTestDB(t)
	applyUpstreamHistoryQueueMigration(t, db, ctx)
	other := otherDB()
	usageTx, err := db.BeginTx(ctx, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = usageTx.Rollback() })
	_, err = usageTx.ExecContext(ctx, `INSERT INTO usage_logs(id,created_at,account_id,user_id,api_key_id,model,actual_cost,total_cost,billing_type)
 VALUES(3001,clock_timestamp()-INTERVAL '1 hour',101,11,21,'in-flight',2,1,0)`)
	require.NoError(t, err)
	saveCtx, cancel := context.WithTimeout(ctx, time.Second)
	_, err = other.ExecContext(saveCtx, `INSERT INTO upstream_account_bindings(target_id,account_id,supplier_id,target_name,supplier_name) VALUES(1,101,1,'Supplier key','First')`)
	cancel()
	require.NoError(t, err, "saving must return while the pre-import usage transaction is still uncommitted")
	repo := &upstreamFinanceRepository{db: other}
	_, err = repo.BackfillAccountHistoryBatch(ctx)
	require.Error(t, err, "only the background barrier waits for the old usage write; lock timeout is bounded")
	var pending int
	require.NoError(t, other.QueryRowContext(ctx, `SELECT COUNT(*) FROM upstream_account_history_jobs WHERE completed_at IS NULL AND attempts=1`).Scan(&pending))
	require.Equal(t, 1, pending)
	require.NoError(t, usageTx.Commit())
	_, err = other.ExecContext(ctx, `UPDATE upstream_account_history_jobs SET next_attempt_at=clock_timestamp()`)
	require.NoError(t, err)
	drainUpstreamHistoryQueue(t, other, ctx)
	var rows int
	require.NoError(t, other.QueryRowContext(ctx, `SELECT COUNT(*) FROM upstream_finance_ledger WHERE usage_id=3001`).Scan(&rows))
	require.Equal(t, 1, rows, "the barrier includes the usage committed after the HTTP save returned")
	insertUpstreamAccountHistoryUsage(t, other, ctx, 3002, 101, time.Now().Add(-2*time.Hour), 3)
	require.NoError(t, other.QueryRowContext(ctx, `SELECT COUNT(*) FROM upstream_finance_ledger WHERE usage_id=3002`).Scan(&rows))
	require.Equal(t, 1, rows, "a late old timestamp is captured even after the history cursor is completed")
}

func TestUpstreamHistoryQueuePostgresConcurrentWorkersAndAttribution(t *testing.T) {
	db, ctx, otherDB := upstreamAccountHistoryTestDB(t)
	applyUpstreamHistoryQueueMigration(t, db, ctx)
	other := otherDB()
	before := time.Now().Add(-time.Hour)
	insertUpstreamAccountHistoryUsage(t, db, ctx, 4001, 101, before, 2)
	insertUpstreamAccountHistoryUsage(t, db, ctx, 4002, 102, before, 3)
	_, err := db.ExecContext(ctx, `INSERT INTO upstream_account_bindings(target_id,account_id,supplier_id,target_name,supplier_name) VALUES(1,101,1,'Supplier key','First'),(1,102,1,'Supplier key','First')`)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `INSERT INTO upstream_finance_ledger(usage_id,created_at,target_id,target_name,supplier_id,supplier_name,account_id,user_id,api_key_id,model,revenue,business_cost,billing_type,total_tokens)
 VALUES(4001,$1,3,'Original archived owner',2,'Second',101,11,21,'original',2,1,0,37)`, before)
	require.NoError(t, err)
	tx, err := db.BeginTx(ctx, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = tx.Rollback() })
	_, err = tx.ExecContext(ctx, `SELECT j.binding_id FROM upstream_account_history_jobs j JOIN upstream_account_bindings b ON b.id=j.binding_id WHERE b.account_id=101 FOR UPDATE OF j`)
	require.NoError(t, err)
	worked, err := (&upstreamFinanceRepository{db: other}).BackfillAccountHistoryBatch(ctx)
	require.NoError(t, err)
	require.True(t, worked, "another instance skips the leased row and makes progress on another account")
	require.NoError(t, tx.Rollback())
	drainUpstreamHistoryQueue(t, other, ctx)
	var targetID int64
	require.NoError(t, db.QueryRowContext(ctx, `SELECT target_id FROM upstream_finance_ledger WHERE usage_id=4001`).Scan(&targetID))
	require.Equal(t, int64(3), targetID, "the historical ledger's original owner is never reassigned")
	require.NoError(t, db.QueryRowContext(ctx, `SELECT target_id FROM upstream_finance_ledger WHERE usage_id=4002`).Scan(&targetID))
	require.Equal(t, int64(1), targetID)
}
