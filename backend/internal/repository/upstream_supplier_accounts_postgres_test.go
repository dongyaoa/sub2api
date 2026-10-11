package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestUpstreamSupplierAccountsPostgresScopeAndPagination(t *testing.T) {
	dsn := os.Getenv("UPSTREAM_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("UPSTREAM_TEST_DATABASE_URL is not configured")
	}
	db, err := sql.Open("postgres", dsn)
	require.NoError(t, err)
	db.SetMaxOpenConns(1)
	defer func() { _ = db.Close() }()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	schema := "upstream_supplier_accounts_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	_, err = db.ExecContext(ctx, `CREATE SCHEMA "`+schema+`"`)
	require.NoError(t, err)
	defer func() { _, _ = db.ExecContext(context.Background(), `DROP SCHEMA "`+schema+`" CASCADE`) }()
	_, err = db.ExecContext(ctx, `SET search_path TO "`+schema+`"`)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `CREATE TABLE accounts (
 id BIGINT PRIMARY KEY,name TEXT NOT NULL,platform TEXT NOT NULL DEFAULT 'openai',
 type TEXT NOT NULL DEFAULT 'apikey',credentials JSONB NOT NULL DEFAULT '{}',
 parent_account_id BIGINT,extra JSONB NOT NULL DEFAULT '{}',deleted_at TIMESTAMPTZ);
INSERT INTO accounts(id,name,credentials) VALUES
 (1,'First','{"base_url":"https://site.example","api_key":"never-return-this-secret"}'),
 (2,'Second','{"base_url":" HTTPS://SITE.EXAMPLE:443/v1/ ","api_key":"key"}'),
 (3,'Subdomain','{"base_url":"https://child.site.example/v1","api_key":"key"}'),
 (4,'Other port','{"base_url":"https://site.example:8443/v1","api_key":"key"}'),
 (5,'HTTP','{"base_url":"http://site.example/v1","api_key":"key"}'),
 (6,'User info','{"base_url":"https://site.example@evil.example/v1","api_key":"key"}'),
 (7,'Query credentials','{"base_url":"https://site.example/v1?token=secret","api_key":"key"}'),
 (8,'Missing key','{"base_url":"https://site.example/v1"}'),
 (9,'Synthetic','{"base_url":"https://site.example/v1","api_key":"key"}'),
 (10,'Child account','{"base_url":"https://site.example/v1","api_key":"key"}'),
 (11,'OAuth','{"base_url":"https://site.example/v1","api_key":"key"}'),
 (12,'Other platform','{"base_url":"https://site.example/v1","api_key":"key"}'),
 (13,'Deleted','{"base_url":"https://site.example/v1","api_key":"key"}'),
 (14,'Console API','{"base_url":"https://api.site.example/prefix/v1","api_key":"key"}'),
 (15,'Official default','{"api_key":"official-key"}'),
 (16,'Host suffix','{"base_url":"https://site.example.evil/v1","api_key":"key"}'),
 (17,'Token %_ stored','{"base_url":"https://site.example:00443/messages","api_key":"key"}'),
 (18,'Whitespace key','{"base_url":"https://site.example/v1","api_key":" \t "}'),
 (19,'Backslash','{"base_url":"https://site.example/\\path","api_key":"key"}'),
 (20,'IPv6','{"base_url":"https://[2001:db8::1]:443/v1","api_key":"key"}');
UPDATE accounts SET extra='{"synthetic_ui_test":true}' WHERE id=9;
UPDATE accounts SET parent_account_id=1 WHERE id=10;
UPDATE accounts SET type='oauth' WHERE id=11;
UPDATE accounts SET platform='grok' WHERE id=12;
UPDATE accounts SET deleted_at=NOW() WHERE id=13;`)
	require.NoError(t, err)
	repo := &upstreamCenterRepository{db: db}
	query := service.UpstreamSupplierAccountsQuery{Origins: []string{"https://site.example", "https://api.site.example"}, Page: 1, PageSize: 100}
	items, total, err := repo.ListSupplierAccounts(ctx, query)
	require.NoError(t, err)
	require.Equal(t, int64(4), total)
	ids := make([]int64, 0, len(items))
	for _, item := range items {
		ids = append(ids, item.ID)
	}
	require.ElementsMatch(t, []int64{1, 2, 14, 17}, ids)
	encoded, err := json.Marshal(items)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), "never-return-this-secret")
	require.NotContains(t, string(encoded), "api_key")

	for _, tc := range []struct {
		origin string
		id     int64
	}{
		{"https://api.openai.com", 15}, {"https://site.example:8443", 4},
		{"http://site.example", 5}, {"https://[2001:db8::1]", 20},
	} {
		items, total, err = repo.ListSupplierAccounts(ctx, service.UpstreamSupplierAccountsQuery{Origins: []string{tc.origin}, Page: 1, PageSize: 100})
		require.NoError(t, err)
		require.Equal(t, int64(1), total, tc.origin)
		require.Equal(t, tc.id, items[0].ID)
	}
	query.Search = "%_"
	items, total, err = repo.ListSupplierAccounts(ctx, query)
	require.NoError(t, err)
	require.Equal(t, int64(1), total, "search treats SQL wildcard characters literally")
	require.Equal(t, int64(17), items[0].ID)
	query.Search = ""
	_, err = db.ExecContext(ctx, `INSERT INTO accounts(id,name,credentials)
 SELECT 1000+n,'A unrelated '||n,jsonb_build_object('base_url','https://other.example/v1','api_key','key') FROM generate_series(1,125) n;
INSERT INTO accounts(id,name,credentials)
 SELECT 2000+n,'Z matching '||LPAD(n::text,3,'0'),jsonb_build_object('base_url','https://site.example/v1','api_key','key') FROM generate_series(1,121) n;`)
	require.NoError(t, err)
	items, total, err = repo.ListSupplierAccounts(ctx, query)
	require.NoError(t, err)
	require.Equal(t, int64(125), total, "unrelated accounts are filtered before pagination")
	require.Len(t, items, 100)
	seen := map[int64]bool{}
	for _, item := range items {
		seen[item.ID] = true
	}
	query.Page = 2
	items, total, err = repo.ListSupplierAccounts(ctx, query)
	require.NoError(t, err)
	require.Equal(t, int64(125), total)
	require.Len(t, items, 25)
	for _, item := range items {
		require.False(t, seen[item.ID], "stable ordering must not repeat accounts across pages")
	}
	query.Page = int(^uint(0) >> 1)
	items, total, err = repo.ListSupplierAccounts(ctx, query)
	require.NoError(t, err)
	require.Empty(t, items)
	require.Equal(t, int64(125), total)
}
