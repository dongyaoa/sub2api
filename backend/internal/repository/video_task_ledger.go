package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

type videoTaskScanner interface {
	Scan(dest ...any) error
}

func (s *videoTaskStore) Persistent() bool {
	return s != nil && s.db != nil
}

func (s *videoTaskStore) saveDurable(ctx context.Context, task *service.VideoTaskRecord) error {
	if task == nil {
		return errors.New("video task is nil")
	}
	billingStatus := strings.TrimSpace(task.BillingStatus)
	if billingStatus == "" {
		billingStatus = service.VideoTaskBillingPending
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO grok_video_generation_tasks (
			request_id, user_id, api_key_id, group_id, account_id, subscription_id,
			operation, model, upstream_model, prompt, resolution, aspect_ratio,
			duration_seconds, request_payload_hash, status, http_status, task_error,
			last_upstream_error, last_checked_at, video_url, content_type, byte_size,
			browser_playable, playback_format_version, delivery_error, delivered_at,
			billing_status, billing_error, billed_at, completed_at, expires_at,
			created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6,
			$7, $8, $9, $10, $11, $12,
			$13, $14, $15, $16, NULLIF($17, '')::jsonb,
			$18, $19, $20, $21, $22,
			$23, $24, $25, $26,
			$27, $28, $29, $30, $31,
			$32, NOW()
		)
		ON CONFLICT (request_id) DO UPDATE SET
			user_id = EXCLUDED.user_id,
			api_key_id = EXCLUDED.api_key_id,
			group_id = EXCLUDED.group_id,
			account_id = EXCLUDED.account_id,
			subscription_id = EXCLUDED.subscription_id,
			operation = EXCLUDED.operation,
			model = EXCLUDED.model,
			upstream_model = EXCLUDED.upstream_model,
			prompt = EXCLUDED.prompt,
			resolution = EXCLUDED.resolution,
			aspect_ratio = EXCLUDED.aspect_ratio,
			duration_seconds = EXCLUDED.duration_seconds,
			request_payload_hash = EXCLUDED.request_payload_hash,
			status = EXCLUDED.status,
			http_status = EXCLUDED.http_status,
			task_error = EXCLUDED.task_error,
			last_upstream_error = EXCLUDED.last_upstream_error,
			last_checked_at = EXCLUDED.last_checked_at,
			video_url = EXCLUDED.video_url,
			content_type = EXCLUDED.content_type,
			byte_size = EXCLUDED.byte_size,
			browser_playable = EXCLUDED.browser_playable,
			playback_format_version = EXCLUDED.playback_format_version,
			delivery_error = EXCLUDED.delivery_error,
			delivered_at = EXCLUDED.delivered_at,
			billing_status = EXCLUDED.billing_status,
			billing_error = EXCLUDED.billing_error,
			billed_at = EXCLUDED.billed_at,
			completed_at = EXCLUDED.completed_at,
			expires_at = EXCLUDED.expires_at,
			updated_at = NOW()
	`,
		task.ID, task.UserID, task.APIKeyID, nullablePositiveInt64(task.GroupID), task.AccountID, nullablePositiveInt64(task.SubscriptionID),
		task.Metadata.Operation, task.Metadata.Model, nullableString(task.UpstreamModel), task.Metadata.Prompt,
		nullableString(task.Metadata.Resolution), nullableString(task.Metadata.AspectRatio), task.Metadata.Duration,
		nullableString(task.RequestPayloadHash), task.Status, nullablePositiveInt(task.HTTPStatus), string(task.Error),
		nullableString(task.LastUpstreamError), nullableUnixPtr(task.LastCheckedAt), nullableString(task.VideoURL), nullableString(task.ContentType), task.ByteSize,
		task.BrowserPlayable, task.PlaybackFormatVersion, nullableString(task.DeliveryError), nullableUnixPtr(task.DeliveredAt),
		billingStatus, nullableString(task.BillingError), nullableUnixPtr(task.BilledAt), nullableUnixPtr(task.CompletedAt), nullableUnix(task.ExpiresAt),
		time.Unix(task.CreatedAt, 0).UTC(),
	)
	return err
}

func (s *videoTaskStore) getDurable(ctx context.Context, id string) (*service.VideoTaskRecord, error) {
	row := s.db.QueryRowContext(ctx, durableVideoTaskSelect+` WHERE request_id = $1`, strings.TrimSpace(id))
	record, err := scanDurableVideoTask(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, service.ErrVideoTaskNotFound
	}
	return record, err
}

func (s *videoTaskStore) listDurable(ctx context.Context, owner service.VideoTaskOwner, limit int) ([]*service.VideoTaskRecord, error) {
	if limit <= 0 {
		limit = 10
	}
	if limit > 50 {
		limit = 50
	}
	rows, err := s.db.QueryContext(ctx, durableVideoTaskSelect+`
		WHERE user_id = $1 AND api_key_id = $2 AND hidden_at IS NULL
		ORDER BY created_at DESC
		LIMIT $3`, owner.UserID, owner.APIKeyID, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	records := make([]*service.VideoTaskRecord, 0, limit)
	for rows.Next() {
		record, scanErr := scanDurableVideoTask(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		records = append(records, record)
	}
	return records, rows.Err()
}

func (s *videoTaskStore) hideDurable(ctx context.Context, owner service.VideoTaskOwner) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE grok_video_generation_tasks
		SET hidden_at = NOW(), updated_at = NOW()
		WHERE user_id = $1 AND api_key_id = $2 AND hidden_at IS NULL
	`, owner.UserID, owner.APIKeyID)
	return err
}

const durableVideoTaskSelect = `
	SELECT
		request_id, user_id, api_key_id, COALESCE(group_id, 0), account_id,
		COALESCE(subscription_id, 0), COALESCE(upstream_model, ''), COALESCE(request_payload_hash, ''),
		status, COALESCE(http_status, 0), task_error, COALESCE(last_upstream_error, ''),
		CASE WHEN last_checked_at IS NULL THEN NULL ELSE EXTRACT(EPOCH FROM last_checked_at)::bigint END,
		EXTRACT(EPOCH FROM created_at)::bigint,
		CASE WHEN completed_at IS NULL THEN NULL ELSE EXTRACT(EPOCH FROM completed_at)::bigint END,
		CASE WHEN expires_at IS NULL THEN 0 ELSE EXTRACT(EPOCH FROM expires_at)::bigint END,
		COALESCE(operation, 'text'), model, prompt, COALESCE(resolution, ''), COALESCE(aspect_ratio, ''), duration_seconds,
		COALESCE(video_url, ''), COALESCE(content_type, ''), byte_size, browser_playable,
		playback_format_version, COALESCE(delivery_error, ''),
		CASE WHEN delivered_at IS NULL THEN NULL ELSE EXTRACT(EPOCH FROM delivered_at)::bigint END,
		billing_status, COALESCE(billing_error, ''),
		CASE WHEN billed_at IS NULL THEN NULL ELSE EXTRACT(EPOCH FROM billed_at)::bigint END
	FROM grok_video_generation_tasks`

func scanDurableVideoTask(scanner videoTaskScanner) (*service.VideoTaskRecord, error) {
	var record service.VideoTaskRecord
	var taskError []byte
	var lastCheckedAt, completedAt, deliveredAt, billedAt sql.NullInt64
	err := scanner.Scan(
		&record.ID, &record.UserID, &record.APIKeyID, &record.GroupID, &record.AccountID,
		&record.SubscriptionID, &record.UpstreamModel, &record.RequestPayloadHash,
		&record.Status, &record.HTTPStatus, &taskError, &record.LastUpstreamError,
		&lastCheckedAt, &record.CreatedAt, &completedAt, &record.ExpiresAt,
		&record.Metadata.Operation, &record.Metadata.Model, &record.Metadata.Prompt,
		&record.Metadata.Resolution, &record.Metadata.AspectRatio, &record.Metadata.Duration,
		&record.VideoURL, &record.ContentType, &record.ByteSize, &record.BrowserPlayable,
		&record.PlaybackFormatVersion, &record.DeliveryError, &deliveredAt,
		&record.BillingStatus, &record.BillingError, &billedAt,
	)
	if err != nil {
		return nil, err
	}
	if len(taskError) > 0 {
		record.Error = append(json.RawMessage(nil), taskError...)
	}
	record.LastCheckedAt = nullUnixPtr(lastCheckedAt)
	record.CompletedAt = nullUnixPtr(completedAt)
	record.DeliveredAt = nullUnixPtr(deliveredAt)
	record.BilledAt = nullUnixPtr(billedAt)
	return &record, nil
}

func (s *videoTaskStore) AdminList(ctx context.Context, query service.VideoTaskAdminQuery) (*service.VideoTaskAdminResult, error) {
	if !s.Persistent() {
		return nil, service.ErrVideoTaskUnavailable
	}
	if query.Page <= 0 {
		query.Page = 1
	}
	if query.PageSize <= 0 {
		query.PageSize = 20
	}
	if query.PageSize > 100 {
		query.PageSize = 100
	}

	where, args := buildVideoTaskAdminWhere(query)
	join := `
		FROM grok_video_generation_tasks v
		LEFT JOIN users u ON u.id = v.user_id
		LEFT JOIN api_keys k ON k.id = v.api_key_id
		LEFT JOIN groups g ON g.id = v.group_id
		LEFT JOIN accounts a ON a.id = v.account_id
		LEFT JOIN LATERAL (
			SELECT id, actual_cost
			FROM usage_logs
			WHERE request_id = v.request_id AND api_key_id = v.api_key_id
			ORDER BY created_at DESC
			LIMIT 1
		) ul ON TRUE `

	var total int64
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) `+join+where, args...).Scan(&total); err != nil {
		return nil, err
	}

	listArgs := append([]any{}, args...)
	limitPos := len(listArgs) + 1
	listArgs = append(listArgs, query.PageSize)
	offsetPos := len(listArgs) + 1
	listArgs = append(listArgs, (query.Page-1)*query.PageSize)
	rows, err := s.db.QueryContext(ctx, `
		SELECT
			v.request_id, v.user_id, COALESCE(u.email, ''), v.api_key_id, COALESCE(k.name, ''),
			COALESCE(v.group_id, 0), COALESCE(g.name, ''), v.account_id, COALESCE(a.name, ''),
			v.operation, v.model, COALESCE(v.upstream_model, ''), v.prompt,
			COALESCE(v.resolution, ''), COALESCE(v.aspect_ratio, ''), v.duration_seconds,
			v.status,
			CASE
				WHEN v.video_url IS NOT NULL AND v.video_url <> '' AND v.browser_playable THEN 'delivered'
				WHEN v.status = 'failed' THEN 'failed'
				WHEN v.status = 'completed' THEN 'awaiting_storage'
				ELSE 'processing'
			END,
			v.billing_status, COALESCE(v.http_status, 0), v.task_error,
			COALESCE(v.last_upstream_error, ''), COALESCE(v.delivery_error, ''), COALESCE(v.billing_error, ''),
			COALESCE(v.video_url, ''), COALESCE(v.content_type, ''), v.byte_size, v.browser_playable,
			COALESCE(ul.actual_cost, 0), COALESCE(ul.id, 0),
			v.created_at, v.last_checked_at, v.completed_at, v.delivered_at, v.billed_at
		`+join+where+fmt.Sprintf(` ORDER BY v.created_at DESC LIMIT $%d OFFSET $%d`, limitPos, offsetPos), listArgs...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	items := make([]*service.VideoTaskAdminItem, 0, query.PageSize)
	for rows.Next() {
		item, scanErr := scanVideoTaskAdminItem(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	summary := service.VideoTaskAdminSummary{}
	if err := s.db.QueryRowContext(ctx, `
		SELECT
			COUNT(*),
			COUNT(*) FILTER (WHERE v.status = 'processing'),
			COUNT(*) FILTER (WHERE v.video_url IS NOT NULL AND v.video_url <> '' AND v.browser_playable),
			COUNT(*) FILTER (WHERE v.status = 'failed'),
			COUNT(*) FILTER (WHERE v.billing_status = 'charged' AND (v.video_url IS NULL OR v.video_url = '' OR NOT v.browser_playable)),
			COALESCE(SUM(CASE WHEN v.billing_status = 'charged' THEN COALESCE(ul.actual_cost, 0) ELSE 0 END), 0)
		`+join+where, args...).Scan(
		&summary.Total, &summary.Processing, &summary.Delivered, &summary.Failed,
		&summary.ChargedWithoutOutput, &summary.TotalCharged,
	); err != nil {
		return nil, err
	}

	return &service.VideoTaskAdminResult{Items: items, Total: total, Summary: summary}, nil
}

func buildVideoTaskAdminWhere(query service.VideoTaskAdminQuery) (string, []any) {
	conditions := make([]string, 0, 8)
	args := make([]any, 0, 8)
	addValue := func(condition string, value any) {
		args = append(args, value)
		conditions = append(conditions, fmt.Sprintf(condition, len(args)))
	}
	if search := strings.TrimSpace(query.Search); search != "" {
		args = append(args, "%"+search+"%")
		position := len(args)
		conditions = append(conditions, fmt.Sprintf(
			`(v.request_id ILIKE $%d OR COALESCE(u.email, '') ILIKE $%d OR CAST(v.user_id AS TEXT) ILIKE $%d OR v.prompt ILIKE $%d)`,
			position, position, position, position,
		))
	}
	if status := strings.TrimSpace(query.Status); status != "" {
		addValue(`v.status = $%d`, status)
	}
	if status := strings.TrimSpace(query.BillingStatus); status != "" {
		addValue(`v.billing_status = $%d`, status)
	}
	if model := strings.TrimSpace(query.Model); model != "" {
		addValue(`v.model = $%d`, model)
	}
	if query.AccountID > 0 {
		addValue(`v.account_id = $%d`, query.AccountID)
	}
	if query.StartTime != nil {
		addValue(`v.created_at >= $%d`, query.StartTime.UTC())
	}
	if query.EndTime != nil {
		addValue(`v.created_at < $%d`, query.EndTime.UTC())
	}
	switch strings.TrimSpace(query.DeliveryStatus) {
	case "delivered":
		conditions = append(conditions, `(v.video_url IS NOT NULL AND v.video_url <> '' AND v.browser_playable)`)
	case "failed":
		conditions = append(conditions, `v.status = 'failed'`)
	case "awaiting_storage":
		conditions = append(conditions, `v.status = 'completed' AND (v.video_url IS NULL OR v.video_url = '' OR NOT v.browser_playable)`)
	case "processing":
		conditions = append(conditions, `v.status = 'processing'`)
	case "charged_without_output":
		conditions = append(conditions, `v.billing_status = 'charged' AND (v.video_url IS NULL OR v.video_url = '' OR NOT v.browser_playable)`)
	}
	if len(conditions) == 0 {
		return "", args
	}
	return " WHERE " + strings.Join(conditions, " AND "), args
}

func scanVideoTaskAdminItem(scanner videoTaskScanner) (*service.VideoTaskAdminItem, error) {
	item := &service.VideoTaskAdminItem{}
	var taskError []byte
	var lastCheckedAt, completedAt, deliveredAt, billedAt sql.NullTime
	err := scanner.Scan(
		&item.RequestID, &item.UserID, &item.UserEmail, &item.APIKeyID, &item.APIKeyName,
		&item.GroupID, &item.GroupName, &item.AccountID, &item.AccountName,
		&item.Operation, &item.Model, &item.UpstreamModel, &item.Prompt,
		&item.Resolution, &item.AspectRatio, &item.DurationSeconds,
		&item.Status, &item.DeliveryStatus, &item.BillingStatus, &item.HTTPStatus, &taskError,
		&item.LastUpstreamError, &item.DeliveryError, &item.BillingError,
		&item.VideoURL, &item.ContentType, &item.ByteSize, &item.BrowserPlayable,
		&item.ActualCost, &item.UsageLogID,
		&item.CreatedAt, &lastCheckedAt, &completedAt, &deliveredAt, &billedAt,
	)
	if err != nil {
		return nil, err
	}
	if len(taskError) > 0 {
		item.TaskError = append(json.RawMessage(nil), taskError...)
	}
	item.LastCheckedAt = nullTimePtr(lastCheckedAt)
	item.CompletedAt = nullTimePtr(completedAt)
	item.DeliveredAt = nullTimePtr(deliveredAt)
	item.BilledAt = nullTimePtr(billedAt)
	return item, nil
}

func nullableString(value string) any {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return value
}

func nullablePositiveInt64(value int64) any {
	if value <= 0 {
		return nil
	}
	return value
}

func nullablePositiveInt(value int) any {
	if value <= 0 {
		return nil
	}
	return value
}

func nullableUnix(value int64) any {
	if value <= 0 {
		return nil
	}
	return time.Unix(value, 0).UTC()
}

func nullableUnixPtr(value *int64) any {
	if value == nil {
		return nil
	}
	return nullableUnix(*value)
}

func nullUnixPtr(value sql.NullInt64) *int64 {
	if !value.Valid {
		return nil
	}
	result := value.Int64
	return &result
}

func nullTimePtr(value sql.NullTime) *time.Time {
	if !value.Valid {
		return nil
	}
	result := value.Time.UTC()
	return &result
}
