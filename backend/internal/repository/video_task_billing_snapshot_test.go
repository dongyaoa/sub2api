package repository

import (
	"context"
	"database/sql/driver"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestVideoTaskStoreDurableBillingModelSurvivesRedisMiss(t *testing.T) {
	for _, billingModel := range []string{"muse-priced", ""} {
		t.Run("billing-model="+billingModel, func(t *testing.T) {
			ctx := context.Background()
			mr := miniredis.RunT(t)
			rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
			t.Cleanup(func() { _ = rdb.Close() })
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			t.Cleanup(func() { _ = db.Close() })
			store := ProvideVideoTaskStore(rdb, db)
			task := &service.VideoTaskRecord{
				ID: "mapped-task", UserID: 7, APIKeyID: 9, AccountID: 11,
				Status: service.VideoTaskStatusProcessing, CreatedAt: 100, ExpiresAt: 200,
				BillingModel: billingModel,
				Metadata:     service.VideoTaskMetadata{Model: "muse-video", Resolution: "auto", Duration: 60},
			}
			args := make([]driver.Value, 34)
			for i := range args {
				args[i] = sqlmock.AnyArg()
			}
			args[0], args[7], args[32] = task.ID, task.Metadata.Model, nullableString(billingModel)
			// Subsequent saves may come from a stale Redis record. The first
			// durable billing model must survive both blank and changed values.
			mock.ExpectExec(regexp.QuoteMeta("billing_model = COALESCE(NULLIF(grok_video_generation_tasks.billing_model, ''), EXCLUDED.billing_model)")).
				WithArgs(args...).WillReturnResult(sqlmock.NewResult(0, 1))
			require.NoError(t, store.Save(ctx, task, time.Hour))
			cached, err := store.Get(ctx, task.ID)
			require.NoError(t, err)
			require.Equal(t, billingModel, cached.BillingModel)

			mr.Del(videoTaskKey(task.ID))
			mock.ExpectQuery(regexp.QuoteMeta(durableVideoTaskSelect + " WHERE request_id = $1")).
				WithArgs(task.ID).WillReturnRows(videoTaskBillingSnapshotRows(billingModel))
			restored, err := store.Get(ctx, task.ID)
			require.NoError(t, err)
			require.Equal(t, billingModel, restored.BillingModel)
			require.Equal(t, "muse-video", restored.Metadata.Model)
			require.Equal(t, "auto", restored.Metadata.Resolution)
			require.Equal(t, 60, restored.Metadata.Duration)
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func videoTaskBillingSnapshotRows(billingModel string) *sqlmock.Rows {
	return sqlmock.NewRows([]string{
		"request_id", "user_id", "api_key_id", "group_id", "account_id",
		"subscription_id", "upstream_model", "request_payload_hash",
		"status", "http_status", "task_error", "last_upstream_error",
		"last_checked_at", "created_at", "completed_at", "expires_at",
		"operation", "model", "prompt", "resolution", "aspect_ratio", "duration_seconds",
		"video_url", "content_type", "byte_size", "browser_playable",
		"playback_format_version", "delivery_error", "delivered_at",
		"billing_status", "billing_error", "billed_at", "billing_model", "seed",
	}).AddRow(
		"mapped-task", int64(7), int64(9), int64(3), int64(11),
		int64(0), "muse-video", "hash",
		service.VideoTaskStatusProcessing, 200, nil, "",
		nil, int64(100), nil, int64(200),
		"text", "muse-video", "Scene", "auto", "16:9", 60,
		"", "", int64(0), false,
		0, "", nil,
		service.VideoTaskBillingPending, "", nil, billingModel, nil,
	)
}
