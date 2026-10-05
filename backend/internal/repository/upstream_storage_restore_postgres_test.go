package repository

import (
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/Wei-Shaw/sub2api/migrations"
	"github.com/stretchr/testify/require"
)

func TestUpstreamStorageRestorePostgresPreservesArchiveBoundariesAndWorks(t *testing.T) {
	db, ctx, _ := upstreamStorageTestDB(t)
	for _, file := range []string{"245_intelligence_monitor.sql", "246_intelligence_monitor_oauth.sql", "251_upstream_manual_order.sql", "257_intelligence_candy_monitor.sql", "258_intelligence_candy_schedule.sql", "259_intelligence_local_key_ownership.sql", "262_intelligence_monitor_models.sql"} {
		body, err := migrations.FS.ReadFile(file)
		require.NoError(t, err)
		_, err = db.ExecContext(ctx, string(body))
		require.NoError(t, err, file)
	}
	fingerprint := upstreamRestoreAccountFingerprint("openai", "https://example.com/v1", "bound-key")
	_, err := db.ExecContext(ctx, `UPDATE upstream_targets SET api_key_fingerprint=$1 WHERE id=1`, fingerprint)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `INSERT INTO upstream_targets(id,supplier_id,name,provider,endpoint,api_key_encrypted,api_key_fingerprint,enabled,deleted_at) VALUES
(4,1,'Earlier standalone archive','openai','https://older.example.com','older-cipher','older-fingerprint',FALSE,NOW()-INTERVAL '1 day');
INSERT INTO accounts(id,credentials) VALUES(10,'{"api_key":"bound-key","base_url":"https://example.com/v1/"}');
INSERT INTO upstream_account_bindings(target_id,account_id,supplier_id,target_name,supplier_name,valid_from) VALUES(1,10,1,'Supplier key','First',NOW()-INTERVAL '1 hour');
INSERT INTO intelligence_monitor_plans(id,name,source_type,upstream_target_id,api_key_encrypted,enabled,next_run_at,created_by) VALUES(21,'Retained upstream artwork','upstream',1,'',TRUE,NOW(),1);
INSERT INTO intelligence_monitor_plans(id,name,source_type,api_key_encrypted,enabled,deleted_at,created_by) VALUES(23,'Archived external artwork','external','',FALSE,NOW(),1);
INSERT INTO intelligence_monitor_runs(plan_id,plan_name,status,trigger,model,reasoning_effort,prompt,source_type,source_name,source_endpoint,api_mode,timeout_seconds,html,raw_text) VALUES
(21,'Retained upstream artwork','succeeded','manual','gpt-6-astra','high','draw','upstream','Supplier key','https://example.com/v1','responses',300,'<html>upstream artwork</html>','upstream text'),
(23,'Archived external artwork','succeeded','manual','gpt-6-astra','high','draw','external','external','https://example.com/v1','responses',300,'<html>external artwork</html>','external text');`)
	require.NoError(t, err)
	repo := &upstreamCenterRepository{db: db}
	require.NoError(t, repo.ArchiveSupplier(ctx, 1))
	var archiveAt time.Time
	require.NoError(t, db.QueryRowContext(ctx, `SELECT deleted_at FROM upstream_suppliers WHERE id=1`).Scan(&archiveAt))
	_, err = repo.RestoreStorage(ctx, service.UpstreamStorageRestoreInput{Kind: "target", ID: 1})
	require.ErrorIs(t, err, service.ErrUpstreamStorageParentArchived)
	_, err = db.ExecContext(ctx, `INSERT INTO upstream_account_bindings(target_id,account_id,target_name) VALUES(2,10,'Active replacement')`)
	require.NoError(t, err)
	_, err = repo.RestoreStorage(ctx, service.UpstreamStorageRestoreInput{Kind: "supplier", ID: 1})
	require.ErrorIs(t, err, service.ErrUpstreamBindingConflict)
	count := func(query string, args ...any) int64 {
		t.Helper()
		var value int64
		require.NoError(t, db.QueryRowContext(ctx, query, args...).Scan(&value))
		return value
	}
	require.Equal(t, int64(1), count(`SELECT COUNT(*) FROM upstream_suppliers WHERE id=1 AND deleted_at=$1`, archiveAt))
	require.Equal(t, int64(1), count(`SELECT COUNT(*) FROM upstream_account_bindings WHERE target_id=2 AND account_id=10 AND valid_until IS NULL`), "failed restore cannot steal the replacement binding")
	_, err = db.ExecContext(ctx, `UPDATE upstream_account_bindings SET valid_until=clock_timestamp() WHERE target_id=2 AND account_id=10 AND valid_until IS NULL`)
	require.NoError(t, err)
	result, err := repo.RestoreStorage(ctx, service.UpstreamStorageRestoreInput{Kind: "supplier", ID: 1})
	require.NoError(t, err)
	require.Equal(t, &service.UpstreamStorageRestoreResult{SuppliersRestored: 1, TargetsRestored: 1, BindingsRestored: 1}, result)
	require.Equal(t, int64(1), count(`SELECT COUNT(*) FROM upstream_targets WHERE id=1 AND deleted_at IS NULL AND NOT enabled AND next_check_at IS NULL AND lease_until IS NULL AND balance_lease_until IS NULL`))
	require.Equal(t, int64(1), count(`SELECT COUNT(*) FROM upstream_targets WHERE id=4 AND deleted_at IS NOT NULL`), "an earlier independently archived group must remain archived")
	require.Equal(t, int64(1), count(`SELECT COUNT(*) FROM upstream_account_bindings WHERE target_id=1 AND account_id=10 AND valid_until=$1`, archiveAt), "the old financial interval remains closed")
	require.Equal(t, int64(1), count(`SELECT COUNT(*) FROM upstream_account_bindings WHERE target_id=1 AND account_id=10 AND valid_until IS NULL AND valid_from>$1`, archiveAt), "restored binding starts a new financial interval")
	require.Equal(t, int64(1), count(`SELECT COUNT(*) FROM intelligence_monitor_plans WHERE id=21 AND NOT enabled AND next_run_at IS NULL AND candy_next_run_at IS NULL`))
	result, err = repo.RestoreStorage(ctx, service.UpstreamStorageRestoreInput{Kind: "intelligence", ID: 23})
	require.NoError(t, err)
	require.Equal(t, &service.UpstreamStorageRestoreResult{PlansRestored: 1, RequiresConfiguration: true}, result)
	require.Equal(t, int64(1), count(`SELECT COUNT(*) FROM intelligence_monitor_plans WHERE id=23 AND deleted_at IS NULL AND NOT enabled AND api_key_encrypted=''`))
	require.Equal(t, int64(2), count(`SELECT COUNT(*) FROM intelligence_monitor_runs WHERE html LIKE '<html>%artwork</html>'`), "restoring source and plan preserves completed works")
}
