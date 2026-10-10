package repository

import (
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/Wei-Shaw/sub2api/migrations"
	"github.com/stretchr/testify/require"
)

func TestUpstreamSupplierNewAPIPostgres(t *testing.T) {
	db, ctx, _ := upstreamStorageTestDB(t)
	_, err := db.ExecContext(ctx, `UPDATE upstream_targets SET newapi_user_id=42,newapi_access_token_encrypted='legacy-one' WHERE id=1;
INSERT INTO upstream_targets(id,supplier_id,name,provider,endpoint,api_key_encrypted,api_key_fingerprint,newapi_user_id,newapi_access_token_encrypted,updated_at) VALUES
(4,1,'second group','anthropic','https://EXAMPLE.com:443/v1/messages','key4','key4',42,'legacy-two',clock_timestamp()),
(5,2,'first account','openai','https://other.example.com/v1','key5','key5',11,'user11',clock_timestamp()),
(6,2,'second account','openai','https://other.example.com/v1','key6','key6',12,'user12',clock_timestamp());`)
	require.NoError(t, err)
	migration, err := migrations.FS.ReadFile("269_upstream_supplier_newapi.sql")
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, string(migration))
	require.NoError(t, err)
	center := &upstreamCenterRepository{db: db}
	finance := &upstreamFinanceRepository{db: db}
	site, err := center.GetSupplier(ctx, 1)
	require.NoError(t, err)
	require.Equal(t, int64(42), site.NewAPIUserID)
	require.Equal(t, "legacy-two", site.NewAPIAccessTokenEncrypted)
	require.True(t, site.NewAPIAccessTokenConfigured)
	require.Equal(t, "https://example.com", site.NewAPIAPIBase)
	for _, id := range []int64{1, 4} {
		target, err := center.GetTarget(ctx, id)
		require.NoError(t, err)
		require.True(t, target.NewAPICredentialsInherited)
		require.Equal(t, "legacy-two", target.NewAPIAccessTokenEncrypted)
	}
	conflicted, err := center.GetSupplier(ctx, 2)
	require.NoError(t, err)
	require.True(t, conflicted.NewAPILegacyConflict)
	require.False(t, conflicted.NewAPIAccessTokenConfigured)
	for id, want := range map[int64]int64{5: 11, 6: 12} {
		target, err := center.GetTarget(ctx, id)
		require.NoError(t, err)
		require.Equal(t, want, target.NewAPIUserID, "conflicting users must remain separate")
	}
	// A rerun does not rewrite the chosen secret or invalidate existing identity.
	before, err := finance.GetTarget(ctx, 1)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, string(migration))
	require.NoError(t, err)
	after, err := finance.GetTarget(ctx, 1)
	require.NoError(t, err)
	require.Equal(t, service.UpstreamBalanceIdentity(before), service.UpstreamBalanceIdentity(after))
	require.Equal(t, before.ProfitIdentitySince, after.ProfitIdentitySince)

	// New groups inherit without needing a console credential of their own.
	_, err = db.ExecContext(ctx, `INSERT INTO upstream_targets(id,supplier_id,name,provider,endpoint,api_key_encrypted,api_key_fingerprint) VALUES
(7,1,'new group','gemini','https://example.com/v1beta','key7','key7'),
(8,1,'other prefix','openai','https://example.com/other/v1','key8','key8'),
(9,1,'other host','openai','https://other-site.example/v1','key9','key9');`)
	require.NoError(t, err)
	for id, inherited := range map[int64]bool{7: true, 8: false, 9: false} {
		target, err := center.GetTarget(ctx, id)
		require.NoError(t, err)
		require.Equal(t, inherited, target.NewAPICredentialsInherited)
		if !inherited {
			require.Empty(t, target.NewAPIAccessTokenEncrypted)
		}
	}

	// Rotation updates archived groups too and invalidates any old sync lease.
	_, err = db.ExecContext(ctx, `UPDATE upstream_targets SET deleted_at=NOW() WHERE id=4`)
	require.NoError(t, err)
	now := time.Now().UTC()
	claimed, err := finance.ClaimBalance(ctx, 1, "old-lease", now, now.Add(time.Minute))
	require.NoError(t, err)
	require.True(t, claimed)
	site.NewAPIAccessTokenEncrypted = "rotated-site"
	require.NoError(t, center.SaveSupplier(ctx, site))
	current, err := finance.GetTarget(ctx, 1)
	require.NoError(t, err)
	require.Equal(t, "rotated-site", current.NewAPIAccessTokenEncrypted)
	require.NotEqual(t, service.UpstreamBalanceIdentity(before), service.UpstreamBalanceIdentity(current))
	require.ErrorIs(t, finance.SaveBalance(ctx, before, service.UpstreamBalanceIdentity(before), &service.UpstreamBalanceSnapshot{Status: "ok", Kind: "wallet", Currency: "USD", SyncedAt: &now}, "old-lease", now.Add(time.Minute)), service.ErrUpstreamFinanceIdentityChanged)
	_, err = db.ExecContext(ctx, `UPDATE upstream_targets SET deleted_at=NULL WHERE id=4`)
	require.NoError(t, err)
	restored, err := center.GetTarget(ctx, 4)
	require.NoError(t, err)
	require.Equal(t, "rotated-site", restored.NewAPIAccessTokenEncrypted)

	// Changing target prefix or supplier never carries the old site's token.
	_, err = db.ExecContext(ctx, `UPDATE upstream_targets SET endpoint='https://example.com/other/v1' WHERE id=7;
UPDATE upstream_targets SET supplier_id=NULL WHERE id=4;`)
	require.NoError(t, err)
	for _, id := range []int64{4, 7} {
		target, err := center.GetTarget(ctx, id)
		require.NoError(t, err)
		require.Empty(t, target.NewAPIAccessTokenEncrypted)
		require.False(t, target.NewAPICredentialsInherited)
	}
	// Explicitly disabling site authorization clears every inherited group.
	site.NewAPIUserID, site.NewAPIAccessTokenEncrypted = 0, ""
	require.NoError(t, center.SaveSupplier(ctx, site))
	var remaining int
	require.NoError(t, db.QueryRowContext(ctx, `SELECT COUNT(*) FROM upstream_targets WHERE supplier_id=1 AND newapi_user_id<>0`).Scan(&remaining))
	require.Zero(t, remaining)
}

func TestUpstreamSupplierNewAPISQLRejectsOtherCredentialRecipients(t *testing.T) {
	db, ctx, _ := upstreamStorageTestDB(t)
	for endpoint, want := range map[string]string{
		"https://EXAMPLE.com:443/prefix/v1/messages": "https://example.com/prefix",
		"https://example.com/prefix/v1beta":          "https://example.com/prefix",
		"https://example.com/%2fsecret":              "",
		"https://example.com/a/../secret":            "",
		"https://example.com//secret":                "",
		"https://user@example.com":                   "",
		"https://example.com?q=secret":               "",
		"http://example.com":                         "",
	} {
		var got string
		require.NoError(t, db.QueryRowContext(ctx, `SELECT upstream_newapi_base($1)`, endpoint).Scan(&got))
		require.Equal(t, want, got, endpoint)
	}
}
