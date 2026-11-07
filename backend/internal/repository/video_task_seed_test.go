package repository

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestVideoTaskSeedSurvivesRedisAndDurableHistory(t *testing.T) {
	zero, negative, positive := int64(0), int64(-1), int64(9007199254740991)
	for _, tc := range []struct {
		name string
		seed *int64
	}{
		{"omitted", nil}, {"zero", &zero}, {"negative", &negative}, {"positive", &positive},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			mr := miniredis.RunT(t)
			rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
			t.Cleanup(func() { _ = rdb.Close() })
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			t.Cleanup(func() { _ = db.Close() })
			store := ProvideVideoTaskStore(rdb, db)
			task := &service.VideoTaskRecord{
				ID: "seed-task", UserID: 7, APIKeyID: 9, AccountID: 11,
				Status: service.VideoTaskStatusProcessing, CreatedAt: 100, ExpiresAt: 200,
				Metadata: service.VideoTaskMetadata{Model: "gemini-omni-1.2-flash", Resolution: "4k", Duration: 15, Seed: tc.seed},
			}
			var seedValue driver.Value
			if tc.seed != nil {
				seedValue = *tc.seed
			}
			args := make([]driver.Value, 34)
			for i := range args {
				args[i] = sqlmock.AnyArg()
			}
			args[0], args[7], args[33] = task.ID, task.Metadata.Model, seedValue
			// Zero and negative seeds must reach PostgreSQL unchanged, and an
			// update from a stale task must not erase the original seed.
			mock.ExpectExec(regexp.QuoteMeta("seed = COALESCE(grok_video_generation_tasks.seed, EXCLUDED.seed)")).
				WithArgs(args...).WillReturnResult(sqlmock.NewResult(0, 1))
			require.NoError(t, store.Save(ctx, task, time.Hour))
			cached, err := store.Get(ctx, task.ID)
			require.NoError(t, err)
			require.Equal(t, tc.seed, cached.Metadata.Seed)
			raw, err := json.Marshal(cached.Metadata)
			require.NoError(t, err)
			if tc.seed == nil {
				require.NotContains(t, string(raw), `"seed"`)
			} else {
				require.Contains(t, string(raw), `"seed":`)
			}

			// Both the cache-miss lookup and the durable list used after a page
			// reload need the seed, independent of browser/session snapshots.
			mr.Del(videoTaskKey(task.ID))
			mock.ExpectQuery(regexp.QuoteMeta(durableVideoTaskSelect + " WHERE request_id = $1")).
				WithArgs(task.ID).WillReturnRows(videoTaskSeedRows(seedValue))
			restored, err := store.Get(ctx, task.ID)
			require.NoError(t, err)
			require.Equal(t, tc.seed, restored.Metadata.Seed)
			require.Equal(t, "4k", restored.Metadata.Resolution)
			mock.ExpectQuery(regexp.QuoteMeta(durableVideoTaskSelect)+`\s+WHERE user_id = \$1 AND api_key_id = \$2 AND hidden_at IS NULL`).
				WithArgs(int64(7), int64(9), 10).WillReturnRows(videoTaskSeedRows(seedValue))
			history, err := store.List(ctx, service.VideoTaskOwner{UserID: 7, APIKeyID: 9}, 10)
			require.NoError(t, err)
			require.Len(t, history, 1)
			require.Equal(t, tc.seed, history[0].Metadata.Seed)
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func videoTaskSeedRows(seed driver.Value) *sqlmock.Rows {
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
		"seed-task", int64(7), int64(9), int64(3), int64(11),
		int64(0), "gemini-omni-1.2-flash", "hash",
		service.VideoTaskStatusProcessing, 200, nil, "",
		nil, int64(100), nil, int64(200),
		"text", "gemini-omni-1.2-flash", "Scene", "4k", "16:9", 15,
		"", "", int64(0), false,
		0, "", nil,
		service.VideoTaskBillingPending, "", nil, "gemini-omni-1.2-flash", seed,
	)
}
