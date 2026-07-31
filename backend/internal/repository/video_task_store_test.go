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

func TestVideoTaskStoreListsNewestByOwnerAndClears(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	store := NewVideoTaskStore(rdb)
	ctx := context.Background()
	owner := service.VideoTaskOwner{UserID: 7, APIKeyID: 9}
	first := &service.VideoTaskRecord{ID: "video-1", UserID: 7, APIKeyID: 9, Status: service.VideoTaskStatusCompleted, CreatedAt: 100}
	second := &service.VideoTaskRecord{ID: "video-2", UserID: 7, APIKeyID: 9, Status: service.VideoTaskStatusProcessing, CreatedAt: 200}
	other := &service.VideoTaskRecord{ID: "video-other", UserID: 7, APIKeyID: 10, Status: service.VideoTaskStatusCompleted, CreatedAt: 300}

	require.NoError(t, store.Save(ctx, first, 7*24*time.Hour))
	require.NoError(t, store.Save(ctx, second, 7*24*time.Hour))
	require.NoError(t, store.Save(ctx, other, 7*24*time.Hour))
	tasks, err := store.List(ctx, owner, 10)
	require.NoError(t, err)
	require.Len(t, tasks, 2)
	require.Equal(t, "video-2", tasks[0].ID)
	require.Equal(t, "video-1", tasks[1].ID)
	require.Equal(t, 7*24*time.Hour, mr.TTL(videoTaskHistoryKey(owner)))

	require.NoError(t, store.Clear(ctx, owner))
	_, err = store.Get(ctx, "video-1")
	require.ErrorIs(t, err, service.ErrVideoTaskNotFound)
	_, err = store.Get(ctx, "video-other")
	require.NoError(t, err)
}
