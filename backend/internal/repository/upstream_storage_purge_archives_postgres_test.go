package repository

import (
	"encoding/base64"
	"encoding/json"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/Wei-Shaw/sub2api/migrations"
	"github.com/stretchr/testify/require"
)

func TestUpstreamStoragePurgeArchivesPostgresBeyondListLimit(t *testing.T) {
	db, ctx, _ := upstreamStorageTestDB(t)
	_, err := db.ExecContext(ctx, `ALTER TABLE accounts ADD COLUMN name TEXT NOT NULL DEFAULT 'Account';
 CREATE TABLE api_keys(id BIGINT PRIMARY KEY,user_id BIGINT NOT NULL,key TEXT NOT NULL,status TEXT NOT NULL DEFAULT 'active',updated_at TIMESTAMPTZ,deleted_at TIMESTAMPTZ);
 CREATE TABLE settings(key TEXT PRIMARY KEY,value TEXT NOT NULL,updated_at TIMESTAMPTZ);`)
	require.NoError(t, err)
	for _, name := range []string{"245_intelligence_monitor.sql", "246_intelligence_monitor_oauth.sql", "251_upstream_manual_order.sql", "257_intelligence_candy_monitor.sql", "258_intelligence_candy_schedule.sql", "259_intelligence_local_key_ownership.sql", "262_intelligence_monitor_models.sql", "270_upstream_account_finance_history.sql", "271_upstream_account_history_queue.sql", "274_upstream_purge_finance_barrier.sql"} {
		body, e := migrations.FS.ReadFile(name)
		require.NoError(t, e)
		_, e = db.ExecContext(ctx, string(body))
		require.NoError(t, e, name)
	}
	_, err = db.ExecContext(ctx, `INSERT INTO intelligence_monitor_plans(id,name,source_type,created_by,deleted_at) SELECT n,'Archive '||n,'external',1,NOW() FROM generate_series(1001,1205) n;
 INSERT INTO intelligence_monitor_runs(plan_id,plan_name,status,trigger,model,reasoning_effort,prompt,source_type,source_name,source_endpoint,api_mode,timeout_seconds,html,raw_text)
 SELECT id,name,CASE WHEN id=1001 THEN 'running' ELSE 'succeeded' END,'manual','m','high','draw',source_type,name,'','responses',300,repeat('artwork',1000),repeat('response',1000) FROM intelligence_monitor_plans;
 INSERT INTO api_keys(id,user_id,key) VALUES(10,1,'dedicated'),(11,1,'borrowed');
 UPDATE intelligence_monitor_plans SET local_api_key_id=10,local_key_owner_id=1 WHERE id=1002;
 UPDATE intelligence_monitor_plans SET local_api_key_id=11,local_key_owner_id=1,local_api_key_borrowed=TRUE WHERE id=1003;
 CREATE TABLE billing_usage_entries(api_key_id BIGINT);
 INSERT INTO settings(key,value) VALUES('intelligence_monitor_public_display','{"plan_ids":[1002,1200],"enabled":true}');`)
	require.NoError(t, err)
	repo := &upstreamCenterRepository{db: db}
	page, err := repo.ListStorageArchives(ctx)
	require.NoError(t, err)
	require.Equal(t, int64(206), page.Total)
	require.Len(t, page.Items, 200)
	cursor := ""
	deleted, busy := int64(0), int64(0)
	keys := []string{}
	for batch := 0; ; batch++ {
		require.Less(t, batch, 20, "busy roots must not cause an infinite retry loop")
		result, itemKeys, e := repo.PurgeArchivedStorage(ctx, service.UpstreamStoragePurgeArchivesInput{Cursor: cursor})
		require.NoError(t, e)
		deleted += result.DeletedCount
		busy += result.BusyCount
		keys = append(keys, itemKeys...)
		if batch == 0 {
			_, e = db.ExecContext(ctx, `UPDATE intelligence_monitor_plans SET deleted_at=NULL WHERE id=1200; INSERT INTO intelligence_monitor_plans(id,name,source_type,created_by,deleted_at) VALUES(5000,'Newly archived','external',1,clock_timestamp())`)
			require.NoError(t, e)
		}
		if !result.HasMore {
			require.Greater(t, batch, 7)
			require.Equal(t, int64(2), result.RemainingCount)
			break
		}
		require.NotEmpty(t, result.NextCursor)
		require.NotEqual(t, cursor, result.NextCursor)
		cursor = result.NextCursor
	}
	require.Equal(t, int64(204), deleted)
	require.Equal(t, int64(1), busy)
	require.Equal(t, []string{"dedicated"}, keys)
	var count int
	require.NoError(t, db.QueryRowContext(ctx, `SELECT COUNT(*) FROM intelligence_monitor_runs`).Scan(&count))
	require.Equal(t, 2, count, "only busy and restored works survive")
	require.NoError(t, db.QueryRowContext(ctx, `SELECT COUNT(*) FROM api_keys WHERE id=11 AND status='active' AND deleted_at IS NULL`).Scan(&count))
	require.Equal(t, 1, count)
	var raw string
	require.NoError(t, db.QueryRowContext(ctx, `SELECT value FROM settings WHERE key='intelligence_monitor_public_display'`).Scan(&raw))
	require.JSONEq(t, `{"plan_ids":[1200],"enabled":true}`, raw)
	require.NoError(t, db.QueryRowContext(ctx, `SELECT COUNT(*) FROM api_keys WHERE id=10 AND status='disabled' AND deleted_at IS NOT NULL`).Scan(&count))
	require.Equal(t, 1, count, "keep the revoked key identity for usage writes queued before the plan was purged")
	// A child restored after confirmation must not be swept away by its
	// parent's later batch, even while its upstream remains archived.
	_, err = db.ExecContext(ctx, `INSERT INTO upstream_suppliers(id,name,deleted_at) VALUES(6,'Archived parent',NOW());
INSERT INTO upstream_targets(id,supplier_id,name,provider,endpoint,api_key_encrypted,api_key_fingerprint,deleted_at) VALUES(6,6,'Archived child','openai','https://six.example.com','cipher','six',NOW());
INSERT INTO intelligence_monitor_plans(id,name,source_type,upstream_target_id,created_by,deleted_at) VALUES(6000,'Restored child','upstream',6,1,NOW())`)
	require.NoError(t, err)
	cutoff := upstreamArchiveCursor{Before: time.Now().UTC(), Rank: 1, ID: 1}
	_, err = repo.RestoreStorage(ctx, service.UpstreamStorageRestoreInput{Kind: "intelligence", ID: 6000})
	require.NoError(t, err)
	rawCursor, _ := json.Marshal(cutoff)
	result, _, err := repo.PurgeArchivedStorage(ctx, service.UpstreamStoragePurgeArchivesInput{Cursor: base64.RawURLEncoding.EncodeToString(rawCursor)})
	require.NoError(t, err)
	require.GreaterOrEqual(t, result.BusyCount, int64(1))
	require.NoError(t, db.QueryRowContext(ctx, `SELECT COUNT(*) FROM intelligence_monitor_plans p JOIN upstream_targets t ON t.id=p.upstream_target_id JOIN upstream_suppliers s ON s.id=t.supplier_id WHERE p.id=6000 AND p.deleted_at IS NULL`).Scan(&count))
	require.Equal(t, 1, count)
	_, _, err = repo.PurgeArchivedStorage(ctx, service.UpstreamStoragePurgeArchivesInput{Cursor: "bad cursor"})
	require.ErrorIs(t, err, service.ErrUpstreamStorageInvalid)
}

func TestUpstreamStoragePurgeFinanceBarrierPostgres(t *testing.T) {
	db, ctx, otherDB := upstreamStorageTestDB(t)
	_, schemaErr := db.ExecContext(ctx, `ALTER TABLE accounts ADD COLUMN parent_account_id BIGINT, ADD COLUMN extra JSONB NOT NULL DEFAULT '{}'`)
	require.NoError(t, schemaErr)
	for _, name := range []string{"245_intelligence_monitor.sql", "270_upstream_account_finance_history.sql", "271_upstream_account_history_queue.sql", "274_upstream_purge_finance_barrier.sql"} {
		body, e := migrations.FS.ReadFile(name)
		require.NoError(t, e)
		_, e = db.ExecContext(ctx, string(body))
		require.NoError(t, e, name)
	}
	_, err := db.ExecContext(ctx, `INSERT INTO accounts(id) VALUES(101); INSERT INTO upstream_account_bindings(target_id,account_id,supplier_id,target_name,valid_from) VALUES(1,101,1,'Bound',NOW()-INTERVAL '1 day')`)
	require.NoError(t, err)
	other := otherDB()
	purge, err := db.BeginTx(ctx, nil)
	require.NoError(t, err)
	defer func() { _ = purge.Rollback() }()
	_, err = purge.ExecContext(ctx, `DELETE FROM upstream_account_bindings WHERE target_id=1`)
	require.NoError(t, err)
	done := make(chan error, 1)
	go func() {
		_, e := other.ExecContext(ctx, `INSERT INTO usage_logs(created_at,account_id,user_id,api_key_id,model,actual_cost,total_cost,billing_type) VALUES(NOW(),101,1,1,'m',2,1,0)`)
		done <- e
	}()
	select {
	case err = <-done:
		t.Fatalf("usage bypassed binding purge barrier: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	_, err = purge.ExecContext(ctx, `DELETE FROM upstream_finance_ledger WHERE target_id=1`)
	require.NoError(t, err)
	require.NoError(t, purge.Commit())
	require.NoError(t, <-done)
	var count int
	require.NoError(t, db.QueryRowContext(ctx, `SELECT COUNT(*) FROM upstream_finance_ledger WHERE target_id=1`).Scan(&count))
	require.Zero(t, count, "in-flight usage must not recreate a removed accounting copy")
	require.NoError(t, db.QueryRowContext(ctx, `SELECT COUNT(*) FROM usage_logs WHERE account_id=101`).Scan(&count))
	require.Equal(t, 1, count, "the actual site's usage remains intact")
}
