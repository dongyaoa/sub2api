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

func TestImageTaskStoreRoundTripAndTTL(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	store := NewImageTaskStore(rdb)
	task := &service.ImageTaskRecord{
		ID:        "imgtask_123",
		UserID:    7,
		APIKeyID:  9,
		Status:    service.ImageTaskStatusProcessing,
		CreatedAt: 100,
		ExpiresAt: 200,
	}

	require.NoError(t, store.Save(context.Background(), task, 24*time.Hour))
	got, err := store.Get(context.Background(), task.ID)
	require.NoError(t, err)
	require.Equal(t, task, got)
	require.Equal(t, 24*time.Hour, mr.TTL(imageTaskKey(task.ID)))
}

func TestImageTaskStoreListsByOwnerNewestFirstAndClears(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	store := NewImageTaskStore(rdb)
	ctx := context.Background()
	owner := service.ImageTaskOwner{UserID: 7, APIKeyID: 9}
	first := &service.ImageTaskRecord{
		ID: "imgtask_first", UserID: owner.UserID, APIKeyID: owner.APIKeyID,
		Status: service.ImageTaskStatusCompleted, CreatedAt: 100, ExpiresAt: 200,
	}
	second := &service.ImageTaskRecord{
		ID: "imgtask_second", UserID: owner.UserID, APIKeyID: owner.APIKeyID,
		Status: service.ImageTaskStatusProcessing, CreatedAt: 200, ExpiresAt: 300,
	}
	other := &service.ImageTaskRecord{
		ID: "imgtask_other", UserID: owner.UserID, APIKeyID: 10,
		Status: service.ImageTaskStatusCompleted, CreatedAt: 300, ExpiresAt: 400,
	}

	require.NoError(t, store.Save(ctx, first, 7*24*time.Hour))
	require.NoError(t, store.Save(ctx, second, 7*24*time.Hour))
	require.NoError(t, store.Save(ctx, other, 7*24*time.Hour))

	tasks, err := store.List(ctx, owner, 10)
	require.NoError(t, err)
	require.Len(t, tasks, 2)
	require.Equal(t, second.ID, tasks[0].ID)
	require.Equal(t, first.ID, tasks[1].ID)
	require.Equal(t, 7*24*time.Hour, mr.TTL(imageTaskHistoryKey(owner)))

	require.NoError(t, store.Clear(ctx, owner))
	tasks, err = store.List(ctx, owner, 10)
	require.NoError(t, err)
	require.Empty(t, tasks)
	_, err = store.Get(ctx, first.ID)
	require.ErrorIs(t, err, service.ErrImageTaskNotFound)
	_, err = store.Get(ctx, second.ID)
	require.ErrorIs(t, err, service.ErrImageTaskNotFound)
	_, err = store.Get(ctx, other.ID)
	require.NoError(t, err)
}

func TestImageTaskStoreMissing(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	store := NewImageTaskStore(rdb)

	_, err := store.Get(context.Background(), "imgtask_missing")
	require.ErrorIs(t, err, service.ErrImageTaskNotFound)
}
