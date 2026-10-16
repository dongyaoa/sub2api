package repository

import (
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/Wei-Shaw/sub2api/migrations"
	"github.com/stretchr/testify/require"
)

func TestUpstreamMonitorSourcePostgresAuthorizationLifecycle(t *testing.T) {
	db, ctx, _ := upstreamStorageTestDB(t)
	_, err := db.ExecContext(ctx, `ALTER TABLE upstream_suppliers ADD COLUMN sort_order BIGINT;
ALTER TABLE upstream_targets ADD COLUMN sort_order BIGINT;
UPDATE upstream_suppliers SET newapi_credentials_managed=TRUE,newapi_user_id=42,
newapi_access_token_encrypted='site-token',newapi_api_base='https://example.com' WHERE id=1;
SELECT setval(pg_get_serial_sequence('upstream_targets','id'),(SELECT MAX(id) FROM upstream_targets));`)
	require.NoError(t, err)
	center := &upstreamCenterRepository{db: db}
	finance := &upstreamFinanceRepository{db: db}
	sourceID := int64(1)
	target := &service.UpstreamTarget{SourceTargetID: &sourceID, Name: "Imported monitor", Provider: "openai",
		APIMode: "chat_completions", Endpoint: "https://example.com/v1", APIKeyEncrypted: "copied-key",
		APIKeyFingerprint: "copied-key", Models: []string{"model"}, AccountIDs: []int64{},
		IntervalSeconds: 300, TimeoutSeconds: 45, DegradedThresholdMs: 6000, WalletRef: "default"}
	require.NoError(t, center.SaveTarget(ctx, target))
	assertAuthorization := func(token string) {
		t.Helper()
		stored, err := center.GetTarget(ctx, target.ID)
		require.NoError(t, err)
		require.Equal(t, token, stored.NewAPIAccessTokenEncrypted)
		require.Equal(t, token != "", stored.NewAPICredentialsInherited)
		require.Nil(t, stored.SupplierID)
		require.Empty(t, stored.AccountIDs)
		require.Equal(t, "copied-key", stored.APIKeyEncrypted)
	}
	assertAuthorization("site-token")
	stored, err := center.GetTarget(ctx, target.ID)
	require.NoError(t, err)
	require.Equal(t, sourceID, *stored.SourceTargetID)
	// Reapplying the migration is harmless and preserves the source reference.
	migration, err := migrations.FS.ReadFile("272_upstream_monitor_source_group.sql")
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, string(migration))
	require.NoError(t, err)
	assertAuthorization("site-token")

	before, err := finance.GetTarget(ctx, target.ID)
	require.NoError(t, err)
	now := time.Now().UTC()
	claimed, err := finance.ClaimBalance(ctx, target.ID, "old-lease", now, now.Add(time.Minute))
	require.NoError(t, err)
	require.True(t, claimed)
	_, err = db.ExecContext(ctx, `UPDATE upstream_suppliers SET newapi_access_token_encrypted='rotated-token' WHERE id=1`)
	require.NoError(t, err)
	assertAuthorization("rotated-token")
	require.ErrorIs(t, finance.SaveBalance(ctx, before, service.UpstreamBalanceIdentity(before), &service.UpstreamBalanceSnapshot{Status: "ok", Kind: "wallet", Currency: "USD", SyncedAt: &now}, "old-lease", now.Add(time.Minute)), service.ErrUpstreamFinanceIdentityChanged)

	for _, field := range []string{"source group", "source supplier"} {
		t.Run(field+" archive and restore", func(t *testing.T) {
			table := "upstream_targets"
			if field == "source supplier" {
				table = "upstream_suppliers"
			}
			_, err = db.ExecContext(ctx, `UPDATE `+table+` SET deleted_at=NOW() WHERE id=1`)
			require.NoError(t, err)
			assertAuthorization("")
			_, err = db.ExecContext(ctx, `UPDATE `+table+` SET deleted_at=NULL WHERE id=1`)
			require.NoError(t, err)
			assertAuthorization("rotated-token")
		})
	}

	_, err = db.ExecContext(ctx, `UPDATE upstream_targets SET endpoint='https://example.com/other/v1' WHERE id=1`)
	require.NoError(t, err)
	assertAuthorization("")
	_, err = db.ExecContext(ctx, `UPDATE upstream_targets SET endpoint='https://example.com/v1' WHERE id=1`)
	require.NoError(t, err)
	assertAuthorization("rotated-token")
	_, err = db.ExecContext(ctx, `UPDATE upstream_targets SET deleted_at=NOW() WHERE id=$1`, target.ID)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `UPDATE upstream_suppliers SET newapi_user_id=0,newapi_access_token_encrypted='' WHERE id=1`)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `UPDATE upstream_targets SET deleted_at=NULL WHERE id=$1`, target.ID)
	require.NoError(t, err)
	assertAuthorization("")
	_, err = db.ExecContext(ctx, `UPDATE upstream_suppliers SET newapi_user_id=42,newapi_access_token_encrypted='restored-token' WHERE id=1`)
	require.NoError(t, err)
	assertAuthorization("restored-token")

	// Clearing the reference must not retain a copy of site authorization.
	stored, err = center.GetTarget(ctx, target.ID)
	require.NoError(t, err)
	stored.SourceTargetID = nil
	require.NoError(t, center.SaveTarget(ctx, stored))
	assertAuthorization("")
	stored, err = center.GetTarget(ctx, target.ID)
	require.NoError(t, err)
	stored.SourceTargetID = &sourceID
	require.NoError(t, center.SaveTarget(ctx, stored))
	assertAuthorization("restored-token")
	// A replacement console token can be configured in the same detach/save.
	stored, err = center.GetTarget(ctx, target.ID)
	require.NoError(t, err)
	stored.SourceTargetID = nil
	stored.NewAPIUserID, stored.NewAPIAccessTokenEncrypted = 42, "explicit-independent-token"
	require.NoError(t, center.SaveTarget(ctx, stored))
	stored, err = center.GetTarget(ctx, target.ID)
	require.NoError(t, err)
	require.Nil(t, stored.SourceTargetID)
	require.Equal(t, "explicit-independent-token", stored.NewAPIAccessTokenEncrypted)
	require.False(t, stored.NewAPICredentialsInherited)
	stored.SourceTargetID = &sourceID
	require.NoError(t, center.SaveTarget(ctx, stored))
	assertAuthorization("restored-token")
	_, err = db.ExecContext(ctx, `DELETE FROM upstream_targets WHERE id=1`)
	require.NoError(t, err)
	assertAuthorization("")
	stored, err = center.GetTarget(ctx, target.ID)
	require.NoError(t, err)
	require.Nil(t, stored.SourceTargetID)
}

func TestUpstreamMonitorSourcePostgresCannotCarryAuthorizationToOtherEndpoint(t *testing.T) {
	db, ctx, _ := upstreamStorageTestDB(t)
	_, err := db.ExecContext(ctx, `UPDATE upstream_suppliers SET newapi_credentials_managed=TRUE,newapi_user_id=42,
newapi_access_token_encrypted='site-token',newapi_api_base='https://example.com' WHERE id=1;
INSERT INTO upstream_targets(id,source_target_id,name,provider,endpoint,api_key_encrypted,api_key_fingerprint) VALUES
(4,1,'Other host','openai','https://other.example/v1','key','key4'),
(5,1,'Other prefix','openai','https://example.com/other/v1','key','key5'),
(6,1,'Other protocol','gemini','https://example.com/v1','key','key6'),
(7,2,'Independent source','openai','https://example.com/v1','key','key7');`)
	require.NoError(t, err)
	for _, id := range []int64{4, 5, 6, 7} {
		stored, err := (&upstreamCenterRepository{db: db}).GetTarget(ctx, id)
		require.NoError(t, err)
		require.Empty(t, stored.NewAPIAccessTokenEncrypted)
		require.False(t, stored.NewAPICredentialsInherited)
	}
	_, err = db.ExecContext(ctx, `UPDATE upstream_targets SET supplier_id=1 WHERE id=4`)
	require.Error(t, err, "source references are only valid on independent monitors")
}
