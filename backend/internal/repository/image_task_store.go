package repository

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/redis/go-redis/v9"
)

const (
	imageTaskKeyPrefix        = "image_task:"
	imageTaskHistoryKeyPrefix = "image_task_history:"
	maxImageTaskHistoryItems  = 100
)

type imageTaskStore struct {
	rdb *redis.Client
}

func NewImageTaskStore(rdb *redis.Client) service.ImageTaskStore {
	return &imageTaskStore{rdb: rdb}
}

func (s *imageTaskStore) Save(ctx context.Context, task *service.ImageTaskRecord, ttl time.Duration) error {
	data, err := json.Marshal(task)
	if err != nil {
		return err
	}
	historyKey := imageTaskHistoryKey(service.ImageTaskOwner{UserID: task.UserID, APIKeyID: task.APIKeyID})
	pipe := s.rdb.TxPipeline()
	pipe.Set(ctx, imageTaskKey(task.ID), data, ttl)
	pipe.ZAdd(ctx, historyKey, redis.Z{Score: float64(task.CreatedAt), Member: task.ID})
	pipe.ZRemRangeByRank(ctx, historyKey, 0, -(maxImageTaskHistoryItems + 1))
	pipe.Expire(ctx, historyKey, ttl)
	_, err = pipe.Exec(ctx)
	return err
}

func (s *imageTaskStore) Get(ctx context.Context, id string) (*service.ImageTaskRecord, error) {
	data, err := s.rdb.Get(ctx, imageTaskKey(id)).Bytes()
	if err != nil {
		if err == redis.Nil {
			return nil, service.ErrImageTaskNotFound
		}
		return nil, err
	}
	var task service.ImageTaskRecord
	if err := json.Unmarshal(data, &task); err != nil {
		return nil, err
	}
	return &task, nil
}

func (s *imageTaskStore) List(ctx context.Context, owner service.ImageTaskOwner, limit int) ([]*service.ImageTaskRecord, error) {
	if limit <= 0 {
		limit = 10
	}
	fetchLimit := int64(limit * 3)
	if fetchLimit > maxImageTaskHistoryItems {
		fetchLimit = maxImageTaskHistoryItems
	}
	historyKey := imageTaskHistoryKey(owner)
	ids, err := s.rdb.ZRevRange(ctx, historyKey, 0, fetchLimit-1).Result()
	if err != nil || len(ids) == 0 {
		return nil, err
	}
	keys := make([]string, len(ids))
	for i, id := range ids {
		keys[i] = imageTaskKey(id)
	}
	values, err := s.rdb.MGet(ctx, keys...).Result()
	if err != nil {
		return nil, err
	}
	tasks := make([]*service.ImageTaskRecord, 0, limit)
	staleIDs := make([]any, 0)
	for i, value := range values {
		raw, ok := value.(string)
		if !ok || strings.TrimSpace(raw) == "" {
			staleIDs = append(staleIDs, ids[i])
			continue
		}
		var task service.ImageTaskRecord
		if json.Unmarshal([]byte(raw), &task) != nil {
			staleIDs = append(staleIDs, ids[i])
			continue
		}
		if task.UserID != owner.UserID || task.APIKeyID != owner.APIKeyID {
			continue
		}
		tasks = append(tasks, &task)
		if len(tasks) >= limit {
			break
		}
	}
	if len(staleIDs) > 0 {
		_ = s.rdb.ZRem(ctx, historyKey, staleIDs...).Err()
	}
	return tasks, nil
}

func (s *imageTaskStore) Clear(ctx context.Context, owner service.ImageTaskOwner) error {
	historyKey := imageTaskHistoryKey(owner)
	ids, err := s.rdb.ZRange(ctx, historyKey, 0, -1).Result()
	if err != nil {
		return err
	}
	keys := make([]string, 0, len(ids)+1)
	keys = append(keys, historyKey)
	for _, id := range ids {
		keys = append(keys, imageTaskKey(id))
	}
	return s.rdb.Del(ctx, keys...).Err()
}

func imageTaskKey(id string) string {
	return imageTaskKeyPrefix + strings.TrimSpace(id)
}

func imageTaskHistoryKey(owner service.ImageTaskOwner) string {
	return imageTaskHistoryKeyPrefix + strconv.FormatInt(owner.UserID, 10) + ":" + strconv.FormatInt(owner.APIKeyID, 10)
}
