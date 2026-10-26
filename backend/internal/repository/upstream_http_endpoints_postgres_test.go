package repository

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/migrations"
	"github.com/stretchr/testify/require"
)

func TestUpstreamHTTPNewAPIPostgresNormalizationAndAuthorization(t *testing.T) {
	db, ctx, _ := upstreamStorageTestDB(t)
	_, err := db.ExecContext(ctx, `UPDATE upstream_suppliers SET newapi_credentials_managed=TRUE,newapi_user_id=42,
newapi_access_token_encrypted='existing-https-token',newapi_api_base='https://example.com' WHERE id=1`)
	require.NoError(t, err)
	migration, err := migrations.FS.ReadFile("273_upstream_http_endpoints.sql")
	require.NoError(t, err)
	for i := 0; i < 2; i++ {
		_, err = db.ExecContext(ctx, string(migration))
		require.NoError(t, err)
	}
	var token string
	require.NoError(t, db.QueryRowContext(ctx, `SELECT newapi_access_token_encrypted FROM upstream_targets WHERE id=1`).Scan(&token))
	require.Equal(t, "existing-https-token", token, "migration must preserve existing HTTPS credentials")
	for endpoint, want := range map[string]string{
		"https://EXAMPLE.com:443/prefix/v1/messages":   "https://example.com/prefix",
		"http://EXAMPLE.com:80/prefix/v1/messages":     "http://example.com/prefix",
		"http://216.158.77.26:7070/v1":                 "http://216.158.77.26:7070",
		"http://example.com:443/v1":                    "http://example.com:443",
		"https://example.com:80/v1":                    "https://example.com:80",
		"http://[2606:4700:4700::1111]:80/v1":          "http://[2606:4700:4700::1111]",
		"http://[2606:4700:4700::1111]:7070/prefix/v1": "http://[2606:4700:4700::1111]:7070/prefix",
		"http://example.com:0":                         "", "http://example.com:65536": "", "http://example.com:": "",
		"http://example.com:invalid": "", "http://[invalid]:80": "", "http://[8.8.8.8]:80": "",
		"http://example.com/%2fsecret": "", "http://example.com/a/../b": "", "http://example.com//b": "",
		"http://user@example.com": "", "http://example.com?": "", "http://example.com#": "",
	} {
		var got string
		require.NoError(t, db.QueryRowContext(ctx, `SELECT upstream_newapi_base($1)`, endpoint).Scan(&got), endpoint)
		require.Equal(t, want, got, endpoint)
	}
	_, err = db.ExecContext(ctx, `UPDATE upstream_suppliers SET newapi_credentials_managed=TRUE,newapi_user_id=43,
newapi_access_token_encrypted='http-token',newapi_api_base='http://216.158.77.26:7070/prefix' WHERE id=2;
INSERT INTO upstream_targets(id,supplier_id,name,provider,endpoint,api_key_encrypted,api_key_fingerprint) VALUES
(4,2,'HTTP group','openai','http://216.158.77.26:7070/prefix/v1','key4','key4'),
(5,2,'Other scheme','openai','https://216.158.77.26:7070/prefix/v1','key5','key5'),
(6,2,'Other port','openai','http://216.158.77.26:7071/prefix/v1','key6','key6'),
(7,2,'Other path','openai','http://216.158.77.26:7070/other/v1','key7','key7');`)
	require.NoError(t, err)
	for id, want := range map[int64]string{4: "http-token", 5: "", 6: "", 7: ""} {
		require.NoError(t, db.QueryRowContext(ctx, `SELECT newapi_access_token_encrypted FROM upstream_targets WHERE id=$1`, id).Scan(&token))
		require.Equal(t, want, token)
	}
	_, err = db.ExecContext(ctx, `UPDATE upstream_targets SET endpoint='https://216.158.77.26:7070/prefix/v1' WHERE id=4`)
	require.NoError(t, err)
	require.NoError(t, db.QueryRowContext(ctx, `SELECT newapi_access_token_encrypted FROM upstream_targets WHERE id=4`).Scan(&token))
	require.Empty(t, token, "changing scheme must revoke HTTP-site authorization")
}
