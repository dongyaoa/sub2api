package repository

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
)

// Restoring is a new membership interval, not a reopening of old billing rows.
// The original records, credentials, snapshots and account rates remain intact.
func (r *upstreamCenterRepository) RestoreStorage(ctx context.Context, in service.UpstreamStorageRestoreInput) (*service.UpstreamStorageRestoreResult, error) {
	if in.ID <= 0 || (in.Kind != "supplier" && in.Kind != "target" && in.Kind != "intelligence") {
		return nil, service.ErrUpstreamStorageRestoreInvalid
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	if err = lockManualOrderMembership(ctx, tx); err != nil {
		return nil, err
	}
	if in.Kind == "intelligence" {
		result, err := restoreIntelligenceStorage(ctx, tx, in.ID)
		if err != nil {
			return nil, err
		}
		if err = tx.Commit(); err != nil {
			return nil, err
		}
		return result, nil
	}
	targetIDs, err := lockUpstreamRestoreTargets(ctx, tx, in)
	if err != nil {
		return nil, err
	}
	bindings, err := loadUpstreamRestoreBindings(ctx, tx, targetIDs)
	if err != nil {
		return nil, err
	}
	if err = pauseRestoredUpstreamPlans(ctx, tx, targetIDs); err != nil {
		return nil, err
	}
	result := &service.UpstreamStorageRestoreResult{}
	if in.Kind == "supplier" {
		if _, err = tx.ExecContext(ctx, `UPDATE upstream_suppliers SET deleted_at=NULL,updated_at=clock_timestamp() WHERE id=$1`, in.ID); err != nil {
			return nil, err
		}
		result.SuppliersRestored = 1
	}
	if len(targetIDs) > 0 {
		// A restored monitor is paused. Financial polling is independent of
		// enabled, and gets an immediate refresh without any paid generation.
		if _, err = tx.ExecContext(ctx, `UPDATE upstream_targets SET deleted_at=NULL,enabled=FALSE,next_check_at=NULL,check_token='',lease_until=NULL,balance_next_sync_at=clock_timestamp(),balance_lease_until=NULL,balance_lease_token=NULL,updated_at=clock_timestamp() WHERE id=ANY($1)`, pq.Array(targetIDs)); err != nil {
			return nil, upstreamPersistenceError(err)
		}
		result.TargetsRestored = int64(len(targetIDs))
	}
	for _, binding := range bindings {
		if _, err = tx.ExecContext(ctx, `INSERT INTO upstream_account_bindings(target_id,account_id,supplier_id,target_name,supplier_name,valid_from) SELECT t.id,$2,t.supplier_id,t.name,COALESCE(s.name,''),clock_timestamp() FROM upstream_targets t LEFT JOIN upstream_suppliers s ON s.id=t.supplier_id WHERE t.id=$1`, binding.targetID, binding.accountID); err != nil {
			return nil, upstreamPersistenceError(err)
		}
		result.BindingsRestored++
	}
	if err = tx.Commit(); err != nil {
		return nil, upstreamPersistenceError(err)
	}
	return result, nil
}

func lockUpstreamRestoreTargets(ctx context.Context, tx *sql.Tx, in service.UpstreamStorageRestoreInput) ([]int64, error) {
	var supplierID *int64
	if in.Kind == "supplier" {
		supplierID = &in.ID
	} else if err := tx.QueryRowContext(ctx, `SELECT supplier_id FROM upstream_targets WHERE id=$1 AND deleted_at IS NOT NULL`, in.ID).Scan(&supplierID); err != nil {
		return nil, upstreamStorageQueryError(err)
	}
	var supplierDeletedAt *time.Time
	if supplierID != nil {
		// Keep the supplier -> target order used by inventory changes. The
		// membership lock prevents a target from moving between these reads.
		if err := tx.QueryRowContext(ctx, `SELECT deleted_at FROM upstream_suppliers WHERE id=$1 FOR UPDATE`, *supplierID).Scan(&supplierDeletedAt); err != nil {
			return nil, upstreamStorageQueryError(err)
		}
		if in.Kind == "target" && supplierDeletedAt != nil {
			return nil, service.ErrUpstreamStorageParentArchived
		}
		if in.Kind == "supplier" && supplierDeletedAt == nil {
			return nil, service.ErrUpstreamStorageNotFound
		}
	}
	where, args := `id=$1 AND deleted_at IS NOT NULL`, []any{in.ID}
	if in.Kind == "supplier" {
		// ArchiveSupplier uses transaction-stable NOW() for the supplier and
		// its then-active children. An older standalone archive stays archived.
		where, args = `supplier_id=$1 AND deleted_at=$2`, []any{in.ID, *supplierDeletedAt}
	}
	rows, err := tx.QueryContext(ctx, `SELECT id,COALESCE(lease_until>NOW(),FALSE) OR COALESCE(balance_lease_until>NOW(),FALSE) FROM upstream_targets WHERE `+where+` ORDER BY id FOR UPDATE`, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	ids := []int64{}
	for rows.Next() {
		var id int64
		var busy bool
		if err = rows.Scan(&id, &busy); err != nil {
			return nil, err
		}
		if busy {
			return nil, service.ErrUpstreamStorageBusy
		}
		ids = append(ids, id)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	if in.Kind == "target" && len(ids) == 0 {
		return nil, service.ErrUpstreamStorageNotFound
	}
	return ids, nil
}

type upstreamRestoreBinding struct {
	targetID, accountID   int64
	provider, fingerprint string
}

func loadUpstreamRestoreBindings(ctx context.Context, tx *sql.Tx, targetIDs []int64) ([]upstreamRestoreBinding, error) {
	if len(targetIDs) == 0 {
		return nil, nil
	}
	// Only bindings closed by this archive operation belong to the restore.
	// Never extend an old interval across the period while the group was absent.
	rows, err := tx.QueryContext(ctx, `SELECT b.target_id,b.account_id,t.provider,t.api_key_fingerprint FROM upstream_account_bindings b JOIN upstream_targets t ON t.id=b.target_id WHERE b.target_id=ANY($1) AND b.valid_until=t.deleted_at ORDER BY b.account_id,b.target_id`, pq.Array(targetIDs))
	if err != nil {
		return nil, err
	}
	bindings := []upstreamRestoreBinding{}
	for rows.Next() {
		var binding upstreamRestoreBinding
		if err = rows.Scan(&binding.targetID, &binding.accountID, &binding.provider, &binding.fingerprint); err != nil {
			_ = rows.Close()
			return nil, err
		}
		bindings = append(bindings, binding)
	}
	err = rows.Err()
	_ = rows.Close()
	if err != nil {
		return nil, err
	}
	for _, binding := range bindings {
		var kind, provider, key, endpoint string
		err = tx.QueryRowContext(ctx, `SELECT type,platform,COALESCE(credentials->>'api_key',''),COALESCE(credentials->>'base_url','') FROM accounts WHERE id=$1 AND deleted_at IS NULL FOR SHARE`, binding.accountID).Scan(&kind, &provider, &key, &endpoint)
		if err == sql.ErrNoRows {
			return nil, service.ErrUpstreamBindingConflict
		}
		if err != nil {
			return nil, err
		}
		if kind != service.AccountTypeAPIKey || provider != binding.provider || upstreamRestoreAccountFingerprint(provider, endpoint, key) != binding.fingerprint {
			return nil, service.ErrUpstreamBindingConflict
		}
		var bound bool
		if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM upstream_account_bindings WHERE account_id=$1 AND valid_until IS NULL)`, binding.accountID).Scan(&bound); err != nil {
			return nil, err
		}
		if bound {
			return nil, service.ErrUpstreamBindingConflict
		}
	}
	return bindings, nil
}

func upstreamRestoreAccountFingerprint(provider, endpoint, key string) string {
	endpoint = strings.TrimRight(strings.TrimSpace(endpoint), "/")
	if endpoint == "" {
		switch provider {
		case service.PlatformOpenAI:
			endpoint = "https://api.openai.com"
		case service.PlatformGemini:
			endpoint = "https://generativelanguage.googleapis.com"
		default:
			endpoint = "https://api.anthropic.com"
		}
	}
	fingerprint := sha256.Sum256([]byte(endpoint + "\x00" + strings.TrimSpace(key)))
	return hex.EncodeToString(fingerprint[:])
}

func pauseRestoredUpstreamPlans(ctx context.Context, tx *sql.Tx, targetIDs []int64) error {
	if len(targetIDs) == 0 {
		return nil
	}
	rows, err := tx.QueryContext(ctx, `SELECT id FROM intelligence_monitor_plans WHERE upstream_target_id=ANY($1) AND deleted_at IS NULL ORDER BY id FOR UPDATE`, pq.Array(targetIDs))
	if err != nil {
		return err
	}
	planIDs := []int64{}
	for rows.Next() {
		var id int64
		if err = rows.Scan(&id); err != nil {
			_ = rows.Close()
			return err
		}
		planIDs = append(planIDs, id)
	}
	err = rows.Err()
	_ = rows.Close()
	if err != nil || len(planIDs) == 0 {
		return err
	}
	if err = rejectActiveIntelligenceRuns(ctx, tx, planIDs); err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `UPDATE intelligence_monitor_plans SET enabled=FALSE,next_run_at=NULL,candy_next_run_at=NULL,updated_at=clock_timestamp() WHERE id=ANY($1)`, pq.Array(planIDs))
	return err
}
