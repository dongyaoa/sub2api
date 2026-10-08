package repository

import (
	"context"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

func (r *intelligenceMonitorRepository) ListIntelligenceLocalChannels(ctx context.Context, groupID int64) ([]service.IntelligenceLocalChannel, error) {
	// Configuration must survive temporary unavailability and disabled accounts.
	// Group membership is also what the scheduler uses; do not read credentials
	// or expose unrelated accounts when populating this administrator picker.
	rows, err := r.db.QueryContext(ctx, `SELECT a.id,a.name,a.platform,a.type,a.status
FROM accounts a
JOIN account_groups ag ON ag.account_id=a.id
JOIN groups g ON g.id=ag.group_id
WHERE ag.group_id=$1 AND a.deleted_at IS NULL AND g.deleted_at IS NULL
ORDER BY ag.priority,a.priority,a.id`, groupID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	items := []service.IntelligenceLocalChannel{}
	for rows.Next() {
		var item service.IntelligenceLocalChannel
		if err = rows.Scan(&item.AccountID, &item.Name, &item.Platform, &item.Type, &item.Status); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}
