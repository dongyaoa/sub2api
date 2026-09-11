package repository

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestGatewayCacheRecentRequestsBoundedAndChronological(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	store, ok := NewGatewayCache(client).(service.RecentRequestStore)
	require.True(t, ok)
	ctx := context.Background()
	base := time.Now().UTC().Truncate(time.Second)
	// Intentionally write newest first; completion order must survive worker races.
	for i := 25; i >= 0; i-- {
		require.NoError(t, store.RecordRecentRequest(ctx, 1, service.RecentRequestRecord{
			CreatedAt: base.Add(time.Duration(i) * time.Second), StatusCode: 200, Success: true,
		}))
	}
	got, err := store.GetRecentRequests(ctx, []int64{1, 2, 1}, 50)
	require.NoError(t, err)
	require.Len(t, got, 2)
	require.Len(t, got[1], 20)
	require.Empty(t, got[2])
	require.Equal(t, base.Add(25*time.Second), got[1][0].CreatedAt)
	require.Equal(t, base.Add(6*time.Second), got[1][19].CreatedAt)
	require.Equal(t, int64(1), got[1][0].AccountID)
	require.Equal(t, 7*24*time.Hour, mr.TTL(recentRequestsKey(1)))
	got, err = store.GetRecentRequests(ctx, []int64{1}, 0)
	require.NoError(t, err)
	require.Len(t, got[1], 10)
	mr.FastForward(7 * 24 * time.Hour)
	got, err = store.GetRecentRequests(ctx, []int64{1}, 10)
	require.NoError(t, err)
	require.Empty(t, got[1])
}

func TestGatewayCacheRecentRequestsKeepsIdenticalAttemptsAndRedacts(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	store, ok := NewGatewayCache(client).(service.RecentRequestStore)
	require.True(t, ok)
	ctx := context.Background()
	record := service.RecentRequestRecord{
		CreatedAt: time.Now().UTC(), StatusCode: 400,
		ErrorMessage: `{"error":{"message":"invalid level; access_token=secret-token"}}`,
	}
	require.NoError(t, store.RecordRecentRequest(ctx, 1, record))
	require.NoError(t, store.RecordRecentRequest(ctx, 1, record))
	got, err := store.GetRecentRequests(ctx, []int64{1}, 10)
	require.NoError(t, err)
	require.Len(t, got[1], 2)
	require.Contains(t, got[1][0].ErrorMessage, "invalid level")
	require.NotContains(t, got[1][0].ErrorMessage, "secret-token")
}
