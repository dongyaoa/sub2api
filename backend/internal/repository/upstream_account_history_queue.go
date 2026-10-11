package repository

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

// Each committed cursor and its ledger inserts are one transaction. Row locks
// release on a process crash, so another instance resumes the same batch safely.
func (r *upstreamFinanceRepository) BackfillAccountHistoryBatch(ctx context.Context) (worked bool, err error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err = tx.ExecContext(ctx, `SET LOCAL lock_timeout = '250ms'`); err != nil {
		return false, err
	}
	var bindingID, accountID int64
	var upper *time.Time
	var version time.Time
	err = tx.QueryRowContext(ctx, `SELECT j.binding_id,b.account_id,j.upper_bound,j.updated_at
 FROM upstream_account_history_jobs j JOIN upstream_account_bindings b ON b.id=j.binding_id
 WHERE j.completed_at IS NULL AND j.next_attempt_at <= clock_timestamp()
 ORDER BY j.next_attempt_at,j.updated_at,j.binding_id LIMIT 1 FOR UPDATE OF j SKIP LOCKED`).Scan(&bindingID, &accountID, &upper, &version)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	defer func() {
		if err == nil {
			return
		}
		_ = tx.Rollback()
		// Do not expose SQL/credential details. A failed batch stays visibly pending
		// and retries with a capped delay; no in-memory lease can strand the job.
		retryCtx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_, _ = r.db.ExecContext(retryCtx, `UPDATE upstream_account_history_jobs SET
 attempts=LEAST(attempts+1,60),next_attempt_at=clock_timestamp()+make_interval(secs=>LEAST(60,5*(attempts+1))),
 last_error='batch_retry',updated_at=clock_timestamp() WHERE binding_id=$1 AND completed_at IS NULL AND updated_at=$2`, bindingID, version)
	}()
	if upper == nil {
		// Only this short initialization holds the write barrier, never a history
		// scan. Saving a target does not acquire or wait for this lock.
		_, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended('upstream-finance-history-import:' || $1::text,0))`, accountID)
		if err != nil {
			return false, err
		}
		_, err = tx.ExecContext(ctx, `UPDATE upstream_account_history_jobs SET upper_bound=clock_timestamp(),
 updated_at=clock_timestamp(),next_attempt_at=clock_timestamp(),attempts=0,last_error='' WHERE binding_id=$1`, bindingID)
		if err != nil {
			return false, err
		}
		return true, tx.Commit()
	}
	// Keep an archive or identity change from moving the interval while this
	// bounded batch is read. Never lock account/target rows in the background.
	if _, err = tx.ExecContext(ctx, `SELECT id FROM upstream_account_bindings WHERE id=$1 FOR SHARE`, bindingID); err != nil {
		return false, err
	}
	_, err = tx.ExecContext(ctx, `WITH binding AS MATERIALIZED (
 SELECT b.*,j.lower_bound,j.upper_bound,j.cursor_at,j.cursor_id FROM upstream_account_bindings b
 JOIN upstream_account_history_jobs j ON j.binding_id=b.id WHERE b.id=$1
), batch AS MATERIALIZED (
 SELECT u.* FROM binding b CROSS JOIN LATERAL (
 SELECT u.* FROM usage_logs u
 WHERE u.account_id=b.account_id AND u.created_at >= b.lower_bound
 AND u.created_at < LEAST(b.upper_bound,b.valid_until)
 AND u.created_at <= COALESCE(b.cursor_at,b.upper_bound)
 AND (b.cursor_at IS NULL OR (u.created_at,u.id)<(b.cursor_at,b.cursor_id))
 ORDER BY u.created_at DESC,u.id DESC LIMIT 1000 FOR SHARE OF u
 ) u
), inserted AS (
 INSERT INTO upstream_finance_ledger
 (usage_id,created_at,target_id,target_name,supplier_id,supplier_name,account_id,group_id,user_id,api_key_id,
 model,request_id,revenue,business_cost,billing_type,total_tokens)
 SELECT u.id,u.created_at,b.target_id,b.target_name,b.supplier_id,b.supplier_name,u.account_id,u.group_id,u.user_id,u.api_key_id,
 COALESCE(NULLIF(u.requested_model,''),u.model),COALESCE(u.request_id,''),u.actual_cost,
 COALESCE(u.account_stats_cost,u.total_cost)*COALESCE(u.account_rate_multiplier,1),u.billing_type,
 u.input_tokens::bigint+u.output_tokens::bigint+u.cache_creation_tokens::bigint+u.cache_read_tokens::bigint
 FROM batch u CROSS JOIN binding b
 WHERE NOT EXISTS(SELECT 1 FROM upstream_account_bindings old WHERE old.account_id=u.account_id AND old.id!=b.id
 AND u.created_at >= old.valid_from AND (old.valid_until IS NULL OR u.created_at<old.valid_until))
 ON CONFLICT(usage_id,created_at) DO NOTHING RETURNING 1
), progress AS (
 SELECT COUNT(*) AS scanned,(SELECT COUNT(*) FROM inserted) AS imported,
 (SELECT created_at FROM batch ORDER BY created_at,id LIMIT 1) AS cursor_at,
 (SELECT id FROM batch ORDER BY created_at,id LIMIT 1) AS cursor_id FROM batch
)
 UPDATE upstream_account_history_jobs j SET scanned_rows=j.scanned_rows+p.scanned,imported_rows=j.imported_rows+p.imported,
 cursor_at=COALESCE(p.cursor_at,j.cursor_at),cursor_id=COALESCE(p.cursor_id,j.cursor_id),
 completed_at=CASE WHEN p.scanned<1000 THEN clock_timestamp() ELSE NULL END,
 updated_at=clock_timestamp(),next_attempt_at=clock_timestamp(),attempts=0,last_error=''
 FROM progress p WHERE j.binding_id=$1`, bindingID)
	if err != nil {
		return false, err
	}
	return true, tx.Commit()
}

// Count unfinished imports only when they can affect the requested calendar
// window and current ownership. Archived targets never hold totals pending.
func (r *upstreamFinanceRepository) PendingAccountHistory(ctx context.Context, from, to time.Time) ([]service.UpstreamAccountHistoryWindow, error) {
	rows, err := r.financeReader(ctx).QueryContext(ctx, `SELECT b.target_id,j.lower_bound,
 LEAST(COALESCE(j.upper_bound,clock_timestamp()),b.valid_until),j.cursor_at
 FROM upstream_account_history_jobs j JOIN upstream_account_bindings b ON b.id=j.binding_id
 JOIN upstream_targets t ON t.id=b.target_id JOIN upstream_suppliers s ON s.id=t.supplier_id
 WHERE j.completed_at IS NULL AND t.deleted_at IS NULL AND s.deleted_at IS NULL
 AND b.supplier_id=t.supplier_id AND j.lower_bound<$2
 AND LEAST(COALESCE(j.upper_bound,clock_timestamp()),b.valid_until)>$1
 AND (j.cursor_at IS NULL OR j.cursor_at >= $1)`, from, to)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := []service.UpstreamAccountHistoryWindow{}
	for rows.Next() {
		var window service.UpstreamAccountHistoryWindow
		if err = rows.Scan(&window.TargetID, &window.From, &window.Until, &window.CursorAt); err != nil {
			return nil, err
		}
		out = append(out, window)
	}
	return out, rows.Err()
}
