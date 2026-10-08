package repository

import (
	"context"
	"database/sql"
	"io/fs"
	"net/url"
	"os"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/Wei-Shaw/sub2api/migrations"
	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/stretchr/testify/require"
)

// Historical custom migrations explicitly reference public, so schema isolation
// is insufficient here. The supplied database is only a control connection: all
// migrations and fixtures run in a new UUID-named disposable database. This test
// requires CREATEDB and never starts a monitor or sends a model request.
func TestUpstreamCustomUpgradePostgresPreservesCustomData(t *testing.T) {
	dsn := os.Getenv("UPSTREAM_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("UPSTREAM_TEST_DATABASE_URL is not configured")
	}
	parsed, err := url.Parse(dsn)
	require.NoError(t, err)
	require.Contains(t, []string{"postgres", "postgresql"}, parsed.Scheme, "provide a PostgreSQL URL for the isolated upgrade test")
	control, err := sql.Open("postgres", dsn)
	require.NoError(t, err)
	t.Cleanup(func() { _ = control.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	t.Cleanup(cancel)
	databaseName := "upstream_custom_upgrade_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	_, err = control.ExecContext(ctx, `CREATE DATABASE `+pq.QuoteIdentifier(databaseName))
	require.NoError(t, err, "the test connection needs CREATEDB permission")
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cleanupCancel()
		_, dropErr := control.ExecContext(cleanupCtx, `DROP DATABASE `+pq.QuoteIdentifier(databaseName))
		require.NoError(t, dropErr, "drop only the database created by this test, without terminating other sessions")
	})
	parsed.Path = "/" + databaseName
	parsed.RawPath = ""
	query := parsed.Query()
	query.Del("dbname")
	query.Del("database")
	parsed.RawQuery = query.Encode()
	db, err := sql.Open("postgres", parsed.String())
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	var actualDatabase string
	require.NoError(t, db.QueryRowContext(ctx, `SELECT current_database()`).Scan(&actualDatabase))
	require.Equal(t, databaseName, actualDatabase, "migration connection must use the disposable database")

	// Reconstruct the custom schema before any upstream-center migration. Keep
	// every same-number custom migration; filenames, not numeric IDs, identify them.
	files, err := fs.Glob(migrations.FS, "*.sql")
	require.NoError(t, err)
	baseline := fstest.MapFS{}
	for _, name := range files {
		if name[:3] >= "242" {
			continue
		}
		content, readErr := migrations.FS.ReadFile(name)
		require.NoError(t, readErr)
		baseline[name] = &fstest.MapFile{Data: content}
	}
	require.Contains(t, baseline, "241_payment_order_promotion_snapshot.sql")
	require.NoError(t, applyMigrationsFS(ctx, db, baseline), "initialize the existing custom schema")

	type migrationStamp struct {
		Checksum  string
		AppliedAt time.Time
	}
	readStamps := func() map[string]migrationStamp {
		t.Helper()
		rows, queryErr := db.QueryContext(ctx, `SELECT filename,checksum,applied_at FROM schema_migrations`)
		require.NoError(t, queryErr)
		defer func() { _ = rows.Close() }()
		stamps := map[string]migrationStamp{}
		for rows.Next() {
			var name string
			var stamp migrationStamp
			require.NoError(t, rows.Scan(&name, &stamp.Checksum, &stamp.AppliedAt))
			stamps[name] = stamp
		}
		require.NoError(t, rows.Err())
		return stamps
	}
	before := readStamps()
	require.Len(t, before, len(baseline))
	const promotionSnapshot = `{"promotion_id":"custom-launch","paid_amount":25,"bonus_amount":5,"credited_amount":30}`
	_, err = db.ExecContext(ctx, `
INSERT INTO users(id,email,password_hash,balance,frozen_balance) VALUES(93001,'custom-upgrade@example.test','fixture',30,2);
INSERT INTO proxies(id,name,protocol,host,port) VALUES(93001,'Preserved custom proxy','http','127.0.0.1',8080);
INSERT INTO accounts(id,name,platform,type,credentials,extra,proxy_id) VALUES
 (93001,'Preserved account','openai','apikey','{"base_url":"https://example.test","api_key":"fixture-key"}','{"custom_marker":"preserved"}',93001);
INSERT INTO groups(id,name,platform,rate_multiplier,model_pricing) VALUES
 (93001,'Preserved model square group','openai',1.5,'[{"models":["fixture-model"],"input_price_per_million":2.5}]');
INSERT INTO api_keys(id,user_id,key,name,group_id) VALUES(93001,93001,'sk-custom-upgrade-fixture','Preserved key',93001);
INSERT INTO account_groups(account_id,group_id) VALUES(93001,93001);
INSERT INTO batch_image_jobs(batch_id,user_id,provider,model,item_count,status,hold_amount) VALUES
 ('custom-upgrade-batch',93001,'gemini','fixture-image-model',1,'created',2);
INSERT INTO settings(key,value) VALUES('recharge_promotion_enabled','true')
 ON CONFLICT(key) DO UPDATE SET value=EXCLUDED.value;`)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `INSERT INTO payment_orders(id,user_id,amount,pay_amount,status,expires_at,recharge_code,promotion_snapshot)
VALUES(93001,93001,30,25,'COMPLETED',NOW(),'CUSTOM-UPGRADE',$1::jsonb)`, promotionSnapshot)
	require.NoError(t, err)

	require.NoError(t, ApplyMigrations(ctx, db), "upgrade custom with the upstream-center migration set")
	after := readStamps()
	require.Len(t, after, len(files))
	for name, stamp := range before {
		require.Equal(t, stamp, after[name], "existing migration checksum and application time must remain unchanged: %s", name)
	}
	for _, name := range []string{
		"242_upstream_center.sql", "243_upstream_finance.sql", "244_upstream_remote_billing.sql",
		"245_intelligence_monitor.sql", "246_intelligence_monitor_oauth.sql", "247_upstream_finance_usage_totals.sql",
		"248_upstream_monitor_defaults.sql", "249_intelligence_monitor_interval_seconds.sql", "250_intelligence_monitor_generation_timeout.sql",
		"251_upstream_manual_order.sql", "252_intelligence_monitor_creation_defaults.sql", "253_upstream_newapi_credentials.sql",
		"254_upstream_storage_retention.sql", "255_upstream_storage_policy.sql", "256_intelligence_upstream_plan_lookup.sql",
		"257_intelligence_candy_monitor.sql", "258_intelligence_candy_schedule.sql", "259_intelligence_local_key_ownership.sql",
		"260_intelligence_candy_grading_version.sql", "261_intelligence_candy_fingerprint.sql", "262_intelligence_monitor_models.sql",
		"263_intelligence_deleted_oauth_cleanup.sql", "264_upstream_account_monitor_lookup.sql",
		"268_intelligence_monitor_prompts.sql",
	} {
		require.Contains(t, after, name, "all upstream-center migrations must be recorded")
	}

	assertCustomData := func() {
		t.Helper()
		for table, condition := range map[string]string{
			"users":            "id=93001 AND balance=30 AND frozen_balance=2",
			"accounts":         "id=93001 AND proxy_id=93001 AND extra->>'custom_marker'='preserved'",
			"groups":           "id=93001 AND rate_multiplier=1.5 AND model_pricing->0->'models'->>0='fixture-model'",
			"api_keys":         "id=93001 AND group_id=93001 AND name='Preserved key'",
			"account_groups":   "account_id=93001 AND group_id=93001",
			"batch_image_jobs": "batch_id='custom-upgrade-batch' AND status='created' AND hold_amount=2",
			"settings":         "key='recharge_promotion_enabled' AND value='true'",
			"payment_orders":   "id=93001 AND amount=30 AND pay_amount=25 AND status='COMPLETED' AND recharge_code='CUSTOM-UPGRADE'",
		} {
			var count int
			require.NoError(t, db.QueryRowContext(ctx, `SELECT COUNT(*) FROM `+pq.QuoteIdentifier(table)+` WHERE `+condition).Scan(&count))
			require.Equal(t, 1, count, "custom data must survive in %s", table)
		}
		var snapshot string
		require.NoError(t, db.QueryRowContext(ctx, `SELECT promotion_snapshot::text FROM payment_orders WHERE id=93001`).Scan(&snapshot))
		require.JSONEq(t, promotionSnapshot, snapshot)
	}
	assertCustomData()
	for _, table := range []string{"upstream_suppliers", "upstream_targets", "upstream_account_bindings", "upstream_monitor_history", "upstream_finance_ledger", "upstream_balance_snapshots", "upstream_billing_snapshots", "upstream_monitor_cost_rollups", "intelligence_monitor_plans", "intelligence_monitor_runs"} {
		var exists bool
		require.NoError(t, db.QueryRowContext(ctx, `SELECT to_regclass($1) IS NOT NULL`, "public."+table).Scan(&exists))
		require.True(t, exists, "upgraded table must exist: %s", table)
	}

	// Exercise the final schema alongside preserved custom rows, including the
	// real partitioned usage-log trigger rather than a minimal test-only stub.
	_, err = db.ExecContext(ctx, `
INSERT INTO upstream_suppliers(id,name) VALUES(93001,'Imported supplier');
INSERT INTO upstream_targets(id,supplier_id,name,provider,endpoint,api_key_encrypted,api_key_fingerprint,enabled,interval_seconds)
 VALUES(93001,93001,'Imported key','openai','https://example.test','cipher:fixture','fixture-fingerprint',FALSE,30);
INSERT INTO upstream_account_bindings(target_id,account_id,supplier_id,target_name,supplier_name,valid_from)
 VALUES(93001,93001,93001,'Imported key','Imported supplier',NOW()-INTERVAL '1 minute');
INSERT INTO usage_logs(user_id,api_key_id,account_id,group_id,model,actual_cost,total_cost,account_rate_multiplier,input_tokens,output_tokens)
 VALUES(93001,93001,93001,93001,'fixture-model',3,2,0.5,100,20);
INSERT INTO intelligence_monitor_plans(id,name,source_type,group_id,local_api_key_id,local_key_owner_id,local_api_key_borrowed,created_by,model,enabled,candy_enabled)
 VALUES(93001,'Site pelican','local_group',93001,93001,93001,TRUE,93001,'gpt-6.1-sol',FALSE,TRUE);`)
	require.NoError(t, err)
	var revenue, cost float64
	var tokens int64
	require.NoError(t, db.QueryRowContext(ctx, `SELECT revenue,business_cost,total_tokens FROM upstream_finance_ledger WHERE target_id=93001`).Scan(&revenue, &cost, &tokens))
	require.Equal(t, 3.0, revenue)
	require.Equal(t, 1.0, cost)
	require.Equal(t, int64(120), tokens)
	require.NoError(t, ApplyMigrations(ctx, db), "startup replay must remain idempotent")
	require.Equal(t, after, readStamps(), "startup replay must not rewrite any migration history")
	assertCustomData()
	var model string
	require.NoError(t, db.QueryRowContext(ctx, `SELECT model FROM intelligence_monitor_plans WHERE id=93001`).Scan(&model))
	require.Equal(t, "gpt-6.1-sol", model, "replay must preserve newly configured monitoring")
}
