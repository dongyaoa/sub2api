package repository

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/Wei-Shaw/sub2api/migrations"
	"github.com/stretchr/testify/require"
)

func upstreamAccountHistoryTestDB(t *testing.T) (*sql.DB, context.Context, func() *sql.DB) {
	t.Helper()
	db, ctx, other := upstreamStorageTestDB(t)
	_, err := db.ExecContext(ctx, `ALTER TABLE accounts ADD COLUMN parent_account_id BIGINT;
ALTER TABLE accounts ADD COLUMN extra JSONB NOT NULL DEFAULT '{}';
CREATE INDEX history_test_usage_account_time ON usage_logs(account_id,created_at);
INSERT INTO accounts(id,credentials) VALUES
 (101,'{"api_key":"import-key","base_url":"https://example.com/v1"}'),
 (102,'{"api_key":"import-key","base_url":"https://example.com/v1"}'),
 (103,'{"api_key":"wrong-key","base_url":"https://example.com/v1"}');`)
	require.NoError(t, err)
	fingerprint := sha256.Sum256([]byte("https://example.com/v1\x00import-key"))
	_, err = db.ExecContext(ctx, `UPDATE upstream_targets SET api_key_fingerprint=$1 WHERE id IN(1,2)`, hex.EncodeToString(fingerprint[:]))
	require.NoError(t, err)
	return db, ctx, other
}

func applyUpstreamAccountHistoryMigration(t *testing.T, db *sql.DB, ctx context.Context) {
	t.Helper()
	migration, err := migrations.FS.ReadFile("270_upstream_account_finance_history.sql")
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, string(migration))
	require.NoError(t, err)
}

func insertUpstreamAccountHistoryUsage(t *testing.T, db *sql.DB, ctx context.Context, id, accountID int64, at time.Time, revenue float64) {
	t.Helper()
	_, err := db.ExecContext(ctx, `INSERT INTO usage_logs(id,created_at,account_id,user_id,api_key_id,model,requested_model,request_id,
 actual_cost,total_cost,account_stats_cost,account_rate_multiplier,billing_type,input_tokens,output_tokens,cache_creation_tokens,cache_read_tokens)
 VALUES($1,$2,$3,11,21,'resolved-model','requested-model','request',$4,99,2,0.5,0,10,20,3,4)`, id, at, accountID, revenue)
	require.NoError(t, err)
}

func TestUpstreamAccountHistoryPostgresImportAndLiveCorrections(t *testing.T) {
	db, ctx, _ := upstreamAccountHistoryTestDB(t)
	applyUpstreamAccountHistoryMigration(t, db, ctx)
	now := time.Now().UTC()
	local := now.In(time.FixedZone("Asia/Shanghai", 8*60*60))
	dayStart := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, local.Location())
	todayAt := dayStart.Add(now.Sub(dayStart) / 2).Truncate(time.Microsecond)
	insertUpstreamAccountHistoryUsage(t, db, ctx, 1001, 101, todayAt, 2)
	insertUpstreamAccountHistoryUsage(t, db, ctx, 1002, 101, dayStart.AddDate(0, 0, -10), 4)
	insertUpstreamAccountHistoryUsage(t, db, ctx, 1003, 101, dayStart.AddDate(0, 0, -40), 500)
	insertUpstreamAccountHistoryUsage(t, db, ctx, 1004, 102, todayAt, 900)
	var bindingID int64
	err := db.QueryRowContext(ctx, `INSERT INTO upstream_account_bindings(target_id,account_id,supplier_id,target_name,supplier_name)
 VALUES(1,101,1,'Supplier key','First') RETURNING id`).Scan(&bindingID)
	require.NoError(t, err)

	repo := &upstreamFinanceRepository{db: db}
	targetID, supplierID := int64(1), int64(1)
	q := service.UpstreamFinanceQuery{From: dayStart, To: dayStart.AddDate(0, 0, 1), TargetID: &targetID, SupplierID: &supplierID}
	today, err := repo.Summary(ctx, q)
	require.NoError(t, err)
	require.Equal(t, 2.0, today.Revenue)
	require.Equal(t, 1.0, today.AccountBilled, "local account billing remains separate from upstream actual costs")
	require.Equal(t, int64(1), today.RequestCount)
	require.Equal(t, int64(37), *today.TotalTokens)
	q.From = dayStart.AddDate(0, 0, -29)
	month, err := repo.Summary(ctx, q)
	require.NoError(t, err)
	require.Equal(t, 6.0, month.Revenue)
	require.Equal(t, int64(2), month.RequestCount)
	require.Equal(t, int64(74), *month.TotalTokens)
	sources, err := repo.LoadProfitSources(ctx, dayStart, q.To)
	require.NoError(t, err)
	require.True(t, sources.LastBusinessAt[1].Equal(todayAt), "imported activity must participate in profit immediately")

	// An async usage write created before the import still belongs to this exact
	// imported account; retries and subsequent monetary corrections count once.
	insertUpstreamAccountHistoryUsage(t, db, ctx, 1005, 101, todayAt.Add(time.Millisecond), 3)
	var inserted int64
	require.NoError(t, db.QueryRowContext(ctx, `SELECT upstream_import_account_finance_history($1)`, bindingID).Scan(&inserted))
	require.Zero(t, inserted)
	_, err = db.ExecContext(ctx, `UPDATE usage_logs SET actual_cost=7,input_tokens=100 WHERE id=1001`)
	require.NoError(t, err)
	month, err = repo.Summary(ctx, q)
	require.NoError(t, err)
	require.Equal(t, 14.0, month.Revenue)
	require.Equal(t, int64(3), month.RequestCount)
	require.Equal(t, int64(201), *month.TotalTokens)
	_, err = db.ExecContext(ctx, `DELETE FROM usage_logs WHERE account_id=101`)
	require.NoError(t, err)
	preserved, err := repo.Summary(ctx, q)
	require.NoError(t, err)
	require.Equal(t, month, preserved, "usage retention cannot erase imported ledger history")
}

func TestUpstreamAccountHistoryPostgresUpgradeScopeAndOwnership(t *testing.T) {
	db, ctx, _ := upstreamAccountHistoryTestDB(t)
	at := time.Now().Add(-time.Hour)
	insertUpstreamAccountHistoryUsage(t, db, ctx, 1101, 101, at, 2)
	insertUpstreamAccountHistoryUsage(t, db, ctx, 1102, 102, at, 4)
	insertUpstreamAccountHistoryUsage(t, db, ctx, 1103, 103, at, 8)
	_, err := db.ExecContext(ctx, `INSERT INTO upstream_account_bindings(target_id,account_id,supplier_id,target_name,supplier_name) VALUES
 (1,101,1,'Supplier key','First'),(2,102,NULL,'Independent',''),(1,103,1,'Wrong credentials','First')`)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `INSERT INTO upstream_finance_ledger(usage_id,created_at,target_id,target_name,supplier_id,supplier_name,account_id,user_id,api_key_id,model,revenue,business_cost,billing_type,total_tokens)
 VALUES(1101,$1,3,'Original archived owner',2,'Second',101,11,21,'original',2,1,0,37)`, at)
	require.NoError(t, err)
	insertUpstreamAccountHistoryUsage(t, db, ctx, 1104, 101, at.Add(time.Minute), 16)
	applyUpstreamAccountHistoryMigration(t, db, ctx)
	applyUpstreamAccountHistoryMigration(t, db, ctx)
	var count, targetID int64
	require.NoError(t, db.QueryRowContext(ctx, `SELECT COUNT(*) FROM upstream_finance_ledger`).Scan(&count))
	require.Equal(t, int64(2), count, "only verified supplier-linked accounts import history")
	require.NoError(t, db.QueryRowContext(ctx, `SELECT target_id FROM upstream_finance_ledger WHERE usage_id=1101`).Scan(&targetID))
	require.Equal(t, int64(3), targetID, "upgrade never steals previously attributed history")
	require.NoError(t, db.QueryRowContext(ctx, `SELECT target_id FROM upstream_finance_ledger WHERE usage_id=1104`).Scan(&targetID))
	require.Equal(t, int64(1), targetID)
}

func TestUpstreamAccountHistoryPostgresPriorBindingAndRestoreBoundaries(t *testing.T) {
	db, ctx, _ := upstreamAccountHistoryTestDB(t)
	applyUpstreamAccountHistoryMigration(t, db, ctx)
	now := time.Now().UTC()
	closed := now.Add(-time.Hour)
	_, err := db.ExecContext(ctx, `INSERT INTO upstream_account_bindings(target_id,account_id,supplier_id,target_name,supplier_name,valid_from,valid_until)
 VALUES(3,101,2,'Old key','Second',$1,$2)`, now.Add(-3*time.Hour), closed)
	require.NoError(t, err)
	insertUpstreamAccountHistoryUsage(t, db, ctx, 1201, 101, closed.Add(-time.Minute), 20)
	insertUpstreamAccountHistoryUsage(t, db, ctx, 1202, 101, closed.Add(time.Minute), 3)
	_, err = db.ExecContext(ctx, `INSERT INTO upstream_account_bindings(target_id,account_id,supplier_id,target_name,supplier_name,valid_from)
 VALUES(1,101,1,'Supplier key','First',$1)`, closed.Add(2*time.Minute))
	require.NoError(t, err)
	var targetID int64
	require.NoError(t, db.QueryRowContext(ctx, `SELECT target_id FROM upstream_finance_ledger WHERE usage_id=1201`).Scan(&targetID))
	require.Equal(t, int64(3), targetID)
	require.NoError(t, db.QueryRowContext(ctx, `SELECT target_id FROM upstream_finance_ledger WHERE usage_id=1202`).Scan(&targetID))
	require.Equal(t, int64(1), targetID)

	// Restore inserts another binding for the same target. Its archived gap is
	// deliberately excluded, even though the key and retained logs still match.
	_, err = db.ExecContext(ctx, `UPDATE upstream_account_bindings SET valid_until=$1 WHERE account_id=101 AND valid_until IS NULL`, now.Add(-30*time.Minute))
	require.NoError(t, err)
	gapAt := now.Add(-20 * time.Minute)
	insertUpstreamAccountHistoryUsage(t, db, ctx, 1203, 101, gapAt, 999)
	_, err = db.ExecContext(ctx, `INSERT INTO upstream_account_bindings(target_id,account_id,supplier_id,target_name,supplier_name,valid_from)
	 VALUES(1,101,1,'Supplier key','First',$1)`, now.Add(-10*time.Minute))
	require.NoError(t, err)
	insertUpstreamAccountHistoryUsage(t, db, ctx, 1204, 101, gapAt, 999)
	insertUpstreamAccountHistoryUsage(t, db, ctx, 1205, 101, now.Add(-5*time.Minute), 1)
	var count int64
	require.NoError(t, db.QueryRowContext(ctx, `SELECT COUNT(*) FROM upstream_finance_ledger WHERE usage_id IN(1203,1204)`).Scan(&count))
	require.Zero(t, count, "both retained and late-arriving archived-gap usage stays excluded")
	require.NoError(t, db.QueryRowContext(ctx, `SELECT COUNT(*) FROM upstream_finance_ledger WHERE usage_id=1205`).Scan(&count))
	require.Equal(t, int64(1), count)
}

func TestUpstreamAccountHistoryPostgresUsageRacingImport(t *testing.T) {
	db, ctx, otherDB := upstreamAccountHistoryTestDB(t)
	applyUpstreamAccountHistoryMigration(t, db, ctx)
	other := otherDB()
	usageTx, err := db.BeginTx(ctx, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = usageTx.Rollback() })
	_, err = usageTx.ExecContext(ctx, `INSERT INTO usage_logs(id,created_at,account_id,user_id,api_key_id,model,actual_cost,total_cost,billing_type)
 VALUES(1301,clock_timestamp(),101,11,21,'race',2,1,0)`)
	require.NoError(t, err)
	done := make(chan error, 1)
	go func() {
		_, insertErr := other.ExecContext(ctx, `INSERT INTO upstream_account_bindings(target_id,account_id,supplier_id,target_name,supplier_name)
 VALUES(1,101,1,'Supplier key','First')`)
		done <- insertErr
	}()
	select {
	case err := <-done:
		t.Fatalf("import passed an uncommitted usage write: %v", err)
	case <-time.After(30 * time.Millisecond):
	}
	require.NoError(t, usageTx.Commit())
	require.NoError(t, <-done)
	var count int64
	require.NoError(t, db.QueryRowContext(ctx, `SELECT COUNT(*) FROM upstream_finance_ledger WHERE usage_id=1301`).Scan(&count))
	require.Equal(t, int64(1), count, "the import includes an in-flight usage transaction after it commits")
}

func TestUpstreamAccountHistoryPostgresImportRacingUsage(t *testing.T) {
	db, ctx, otherDB := upstreamAccountHistoryTestDB(t)
	applyUpstreamAccountHistoryMigration(t, db, ctx)
	other := otherDB()
	importTx, err := db.BeginTx(ctx, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = importTx.Rollback() })
	_, err = importTx.ExecContext(ctx, `INSERT INTO upstream_account_bindings(target_id,account_id,supplier_id,target_name,supplier_name)
 VALUES(1,101,1,'Supplier key','First')`)
	require.NoError(t, err)
	done := make(chan error, 1)
	go func() {
		_, insertErr := other.ExecContext(ctx, `INSERT INTO usage_logs(id,created_at,account_id,user_id,api_key_id,model,actual_cost,total_cost,billing_type)
 VALUES(1401,clock_timestamp()-INTERVAL '1 minute',101,11,21,'race',2,1,0)`)
		done <- insertErr
	}()
	select {
	case err := <-done:
		t.Fatalf("usage passed an uncommitted import: %v", err)
	case <-time.After(30 * time.Millisecond):
	}
	require.NoError(t, importTx.Commit())
	require.NoError(t, <-done)
	var count int64
	require.NoError(t, db.QueryRowContext(ctx, `SELECT COUNT(*) FROM upstream_finance_ledger WHERE usage_id=1401`).Scan(&count))
	require.Equal(t, int64(1), count, "usage sees the imported historical interval after the import commits")
}
