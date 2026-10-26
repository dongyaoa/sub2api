package repository

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

type upstreamArchiveCursor struct {
	Before time.Time `json:"before"`
	Rank   int       `json:"rank"`
	ID     int64     `json:"id"`
}

// Only select roots. Purging a supplier also removes its target/plan children.
// A cursor scans the entire archive without holding a large transaction or
// revisiting busy roots forever. New archives are outside this operation.
const upstreamArchiveRootsSQL = `
SELECT 1 AS rank,'supplier' AS kind,s.id,s.name,s.deleted_at FROM upstream_suppliers s WHERE s.deleted_at IS NOT NULL
UNION ALL
SELECT 2,'target',t.id,t.name,t.deleted_at FROM upstream_targets t LEFT JOIN upstream_suppliers s ON s.id=t.supplier_id
WHERE t.deleted_at IS NOT NULL AND s.deleted_at IS NULL
UNION ALL
SELECT 3,'intelligence',p.id,COALESCE(a.name,p.name),p.deleted_at FROM intelligence_monitor_plans p
LEFT JOIN upstream_targets t ON t.id=p.upstream_target_id LEFT JOIN upstream_suppliers s ON s.id=t.supplier_id
LEFT JOIN accounts a ON p.source_type='openai_oauth' AND a.id=p.account_id AND a.deleted_at IS NULL
WHERE p.deleted_at IS NOT NULL AND t.deleted_at IS NULL AND s.deleted_at IS NULL`

func (r *upstreamCenterRepository) PurgeArchivedStorage(ctx context.Context, in service.UpstreamStoragePurgeArchivesInput) (*service.UpstreamStoragePurgeArchivesResult, []string, error) {
	cursor := upstreamArchiveCursor{Before: time.Now().UTC()}
	if in.Cursor != "" {
		raw, err := base64.RawURLEncoding.DecodeString(in.Cursor)
		if err != nil || json.Unmarshal(raw, &cursor) != nil || cursor.Before.IsZero() || cursor.Before.After(time.Now().Add(time.Minute)) || cursor.Rank < 1 || cursor.Rank > 3 || cursor.ID <= 0 {
			return nil, nil, service.ErrUpstreamStorageInvalid
		}
	}
	rows, err := r.db.QueryContext(ctx, `SELECT rank,kind,id,name FROM (`+upstreamArchiveRootsSQL+`) archived WHERE deleted_at<=$1 AND (rank,id)>($2,$3) ORDER BY rank,id LIMIT 26`, cursor.Before, cursor.Rank, cursor.ID)
	if err != nil {
		return nil, nil, err
	}
	type item struct {
		rank  int
		input service.UpstreamStoragePurgeInput
	}
	items := []item{}
	for rows.Next() {
		var value item
		if err = rows.Scan(&value.rank, &value.input.Kind, &value.input.ID, &value.input.ConfirmName); err != nil {
			_ = rows.Close()
			return nil, nil, err
		}
		items = append(items, value)
	}
	err = rows.Err()
	_ = rows.Close()
	if err != nil {
		return nil, nil, err
	}
	result := &service.UpstreamStoragePurgeArchivesResult{HasMore: len(items) > 25}
	if len(items) > 25 {
		items = items[:25]
	}
	keys := []string{}
	started := time.Now()
	for index, value := range items {
		// Reserve time to return partial progress and invalidate committed keys.
		remaining := 35*time.Second - time.Since(started)
		if index > 0 && remaining < 5*time.Second {
			result.HasMore = true
			break
		}
		itemTimeout := 25 * time.Second
		if remaining < itemTimeout {
			itemTimeout = remaining
		}
		itemCtx, cancel := context.WithTimeout(ctx, itemTimeout)
		itemKeys, purgeErr := r.purgeStorage(itemCtx, value.input, &cursor.Before)
		itemDeadlineExceeded := errors.Is(itemCtx.Err(), context.DeadlineExceeded)
		cancel()
		keys = append(keys, itemKeys...)
		if purgeErr == nil {
			result.DeletedCount++
		} else if errors.Is(purgeErr, service.ErrUpstreamStorageBusy) || itemDeadlineExceeded || errors.Is(purgeErr, context.DeadlineExceeded) {
			result.BusyCount++
		} else if !errors.Is(purgeErr, service.ErrUpstreamStorageNotFound) && !errors.Is(purgeErr, service.ErrUpstreamStorageConfirm) {
			return result, keys, purgeErr
		}
		cursor.Rank, cursor.ID = value.rank, value.input.ID
	}
	if result.HasMore {
		raw, _ := json.Marshal(cursor)
		result.NextCursor = base64.RawURLEncoding.EncodeToString(raw)
	}
	if err = r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM (`+upstreamStorageArchivesSQL+`) archived`).Scan(&result.RemainingCount); err != nil {
		return result, keys, err
	}
	return result, keys, nil
}
