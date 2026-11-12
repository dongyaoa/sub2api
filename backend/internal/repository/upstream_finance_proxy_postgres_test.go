package repository

import (
	"context"
	"database/sql"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestUpstreamFinanceProxyPostgresBindingAndSourceIdentity(t *testing.T) {
	dsn := os.Getenv("UPSTREAM_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("UPSTREAM_TEST_DATABASE_URL is not configured")
	}
	db, err := sql.Open("postgres", dsn)
	require.NoError(t, err)
	db.SetMaxOpenConns(1)
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	schema := "upstream_proxy_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	_, err = db.ExecContext(ctx, `CREATE SCHEMA "`+schema+`"`)
	require.NoError(t, err)
	defer func() { _, _ = db.ExecContext(context.Background(), `DROP SCHEMA "`+schema+`" CASCADE`) }()
	_, err = db.ExecContext(ctx, `SET search_path TO "`+schema+`"`)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `CREATE TABLE upstream_suppliers(id BIGINT PRIMARY KEY,deleted_at TIMESTAMPTZ);
CREATE TABLE upstream_targets(id BIGINT PRIMARY KEY,supplier_id BIGINT,source_target_id BIGINT,provider TEXT NOT NULL,endpoint TEXT NOT NULL,api_key_fingerprint TEXT NOT NULL,deleted_at TIMESTAMPTZ);
CREATE TABLE upstream_account_bindings(target_id BIGINT,account_id BIGINT,valid_until TIMESTAMPTZ);
INSERT INTO upstream_suppliers(id) VALUES(1);
INSERT INTO upstream_targets(id,supplier_id,source_target_id,provider,endpoint,api_key_fingerprint) VALUES
(1,1,NULL,'openai','https://source.example','key-one'),
(2,NULL,1,'openai','https://source.example','key-one'),
(3,NULL,NULL,'openai','https://manual.example','manual-key');
INSERT INTO upstream_account_bindings(target_id,account_id,valid_until) VALUES(1,5,NULL),(1,6,NOW());`)
	require.NoError(t, err)
	repo := &upstreamFinanceRepository{db: db}
	for _, id := range []int64{1, 2} {
		ids, err := repo.ProxyAccountIDs(ctx, id)
		require.NoError(t, err)
		require.Equal(t, []int64{5}, ids)
	}
	ids, err := repo.ProxyAccountIDs(ctx, 3)
	require.NoError(t, err)
	require.Empty(t, ids)
	_, err = repo.ProxyAccountIDs(ctx, 404)
	require.ErrorIs(t, err, service.ErrUpstreamFinanceTargetNotFound)
	_, err = db.ExecContext(ctx, `UPDATE upstream_targets SET api_key_fingerprint='new-key' WHERE id=1`)
	require.NoError(t, err)
	_, err = repo.ProxyAccountIDs(ctx, 2)
	require.ErrorIs(t, err, service.ErrUpstreamProxyAccountMismatch)
	_, err = db.ExecContext(ctx, `UPDATE upstream_targets SET api_key_fingerprint='key-one',deleted_at=NOW() WHERE id=1`)
	require.NoError(t, err)
	_, err = repo.ProxyAccountIDs(ctx, 2)
	require.ErrorIs(t, err, service.ErrUpstreamProxyUnavailable)
	_, err = db.ExecContext(ctx, `UPDATE upstream_targets SET deleted_at=NULL WHERE id=1; UPDATE upstream_suppliers SET deleted_at=NOW() WHERE id=1`)
	require.NoError(t, err)
	_, err = repo.ProxyAccountIDs(ctx, 2)
	require.ErrorIs(t, err, service.ErrUpstreamProxyUnavailable)
	_, err = repo.ProxyAccountIDs(ctx, 1)
	require.ErrorIs(t, err, service.ErrUpstreamFinanceTargetNotFound)
}
