package repository

import (
	"context"
	"database/sql"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

// Completed works still belong to the archived plan. Restoring the plan makes
// them visible again without rewriting runs or scheduling new paid requests.
func restoreIntelligenceStorage(ctx context.Context, tx *sql.Tx, id int64) (*service.UpstreamStorageRestoreResult, error) {
	var source, model, encryptedKey string
	var targetID, groupID, accountID, localKeyID, localOwnerID *int64
	var borrowed bool
	if err := tx.QueryRowContext(ctx, `SELECT source_type,model,api_key_encrypted,upstream_target_id,group_id,account_id,local_api_key_id,local_key_owner_id,local_api_key_borrowed FROM intelligence_monitor_plans WHERE id=$1 AND deleted_at IS NOT NULL FOR UPDATE`, id).Scan(&source, &model, &encryptedKey, &targetID, &groupID, &accountID, &localKeyID, &localOwnerID, &borrowed); err != nil {
		return nil, upstreamStorageQueryError(err)
	}
	if err := rejectActiveIntelligenceRuns(ctx, tx, []int64{id}); err != nil {
		return nil, err
	}
	var sourceID *int64
	var sourceColumn string
	var duplicateError error
	switch source {
	case "upstream":
		sourceID, sourceColumn, duplicateError = targetID, "upstream_target_id", service.ErrIntelligenceUpstreamPlanExists
	case "local_group":
		sourceID, sourceColumn, duplicateError = groupID, "group_id", service.ErrIntelligenceLocalPlanExists
	case "openai_oauth":
		sourceID, sourceColumn, duplicateError = accountID, "account_id", service.ErrIntelligenceOAuthPlanExists
	}
	if sourceID != nil {
		var duplicate bool
		// sourceColumn is chosen only from the internal constants above.
		if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM intelligence_monitor_plans WHERE source_type=$1 AND `+sourceColumn+`=$2 AND deleted_at IS NULL AND id<>$3 AND model=$4)`, source, *sourceID, id, model).Scan(&duplicate); err != nil {
			return nil, err
		}
		if duplicate {
			return nil, duplicateError
		}
	}
	result := &service.UpstreamStorageRestoreResult{PlansRestored: 1}
	clearLocalKey := false
	switch source {
	case "external":
		// Archival deliberately cleared the secret. No archived plaintext is
		// reconstructed from historical requests, and no key is reactivated.
		result.RequiresConfiguration = encryptedKey == ""
	case "upstream":
		var available bool
		if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM upstream_targets t LEFT JOIN upstream_suppliers s ON s.id=t.supplier_id WHERE t.id=$1 AND t.deleted_at IS NULL AND (t.supplier_id IS NULL OR s.deleted_at IS NULL))`, targetID).Scan(&available); err != nil {
			return nil, err
		}
		result.RequiresConfiguration = !available
	case "local_group":
		var groupAvailable, keyAvailable bool
		if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM groups WHERE id=$1 AND deleted_at IS NULL AND status='active')`, groupID).Scan(&groupAvailable); err != nil {
			return nil, err
		}
		if localKeyID != nil && localOwnerID != nil {
			if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM api_keys WHERE id=$1 AND user_id=$2 AND group_id=$3 AND deleted_at IS NULL AND status='active' AND (expires_at IS NULL OR expires_at>NOW()) AND (quota=0 OR quota_used<quota))`, *localKeyID, *localOwnerID, groupID).Scan(&keyAvailable); err != nil {
				return nil, err
			}
		}
		clearLocalKey = !keyAvailable
		result.RequiresConfiguration = !groupAvailable || !keyAvailable
	case "openai_oauth":
		var available bool
		if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM accounts WHERE id=$1 AND deleted_at IS NULL AND platform='openai' AND type='oauth')`, accountID).Scan(&available); err != nil {
			return nil, err
		}
		result.RequiresConfiguration = !available
	}
	// Missing sources do not prevent viewing retained works. A paused plan can
	// be edited or its source restored before the administrator enables it.
	if _, err := tx.ExecContext(ctx, `UPDATE intelligence_monitor_plans SET deleted_at=NULL,enabled=FALSE,next_run_at=NULL,candy_next_run_at=NULL,local_api_key_id=CASE WHEN $2 THEN NULL ELSE local_api_key_id END,local_key_owner_id=CASE WHEN $2 THEN NULL ELSE local_key_owner_id END,local_api_key_borrowed=CASE WHEN $2 THEN FALSE ELSE local_api_key_borrowed END,updated_at=clock_timestamp() WHERE id=$1`, id, clearLocalKey); err != nil {
		return nil, err
	}
	return result, nil
}
