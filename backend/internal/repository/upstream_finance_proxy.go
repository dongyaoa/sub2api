package repository

import (
	"context"
	"database/sql"
	"errors"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
)

// Proxy ownership follows a current binding, including a monitor's explicit
// source group. A broken source reference cannot silently fall back to direct.
func (r *upstreamFinanceRepository) ProxyAccountIDs(ctx context.Context, targetID int64) ([]int64, error) {
	var available, matching bool
	var ids pq.Int64Array
	err := r.financeReader(ctx).QueryRowContext(ctx, `SELECT
 source.id IS NOT NULL AND source.deleted_at IS NULL AND
 (t.source_target_id IS NULL OR source.supplier_id IS NOT NULL) AND
 (source.supplier_id IS NULL OR (owner.id IS NOT NULL AND owner.deleted_at IS NULL)),
 COALESCE(source.provider=t.provider AND source.endpoint=t.endpoint AND source.api_key_fingerprint=t.api_key_fingerprint,FALSE),
 ARRAY(SELECT b.account_id FROM upstream_account_bindings b
 WHERE b.target_id=source.id AND b.valid_until IS NULL ORDER BY b.account_id)
 FROM upstream_targets t
 LEFT JOIN upstream_suppliers current_owner ON current_owner.id=t.supplier_id
 LEFT JOIN upstream_targets source ON source.id=COALESCE(t.source_target_id,t.id)
 LEFT JOIN upstream_suppliers owner ON owner.id=source.supplier_id
 WHERE t.id=$1 AND t.deleted_at IS NULL
 AND (t.supplier_id IS NULL OR (current_owner.id IS NOT NULL AND current_owner.deleted_at IS NULL))`, targetID).Scan(&available, &matching, &ids)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, service.ErrUpstreamFinanceTargetNotFound
	}
	if err != nil {
		return nil, err
	}
	if !available {
		return nil, service.ErrUpstreamProxyUnavailable
	}
	if !matching {
		return nil, service.ErrUpstreamProxyAccountMismatch
	}
	return []int64(ids), nil
}
