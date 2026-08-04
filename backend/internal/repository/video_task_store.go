package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/redis/go-redis/v9"
)

const (
	videoTaskKeyPrefix        = "video_task:"
	videoTaskHistoryKeyPrefix = "video_task_history:"
	maxVideoTaskHistoryItems  = 100
)

type videoTaskStore struct {
	rdb *redis.Client
	db  *sql.DB
}

func NewVideoTaskStore(rdb *redis.Client) service.VideoTaskStore {
	return &videoTaskStore{rdb: rdb}
}

func ProvideVideoTaskStore(rdb *redis.Client, db *sql.DB) service.VideoTaskStore {
	return &videoTaskStore{rdb: rdb, db: db}
}

func (s *videoTaskStore) Save(ctx context.Context, task *service.VideoTaskRecord, ttl time.Duration) error {
	durableSaved := false
	if s.db != nil {
		if err := s.saveDurable(ctx, task); err != nil {
			return err
		}
		durableSaved = true
	}
	if s.rdb == nil {
		if durableSaved {
			return nil
		}
		return errors.New("video task storage is unavailable")
	}
	data, err := json.Marshal(task)
	if err != nil {
		if durableSaved {
			return nil
		}
		return err
	}
	historyKey := videoTaskHistoryKey(service.VideoTaskOwner{UserID: task.UserID, APIKeyID: task.APIKeyID})
	pipe := s.rdb.TxPipeline()
	pipe.Set(ctx, videoTaskKey(task.ID), data, ttl)
	pipe.ZAdd(ctx, historyKey, redis.Z{Score: float64(task.CreatedAt), Member: task.ID})
	pipe.ZRemRangeByRank(ctx, historyKey, 0, -(maxVideoTaskHistoryItems + 1))
	pipe.Expire(ctx, historyKey, ttl)
	_, err = pipe.Exec(ctx)
	if durableSaved {
		return nil
	}
	return err
}

func (s *videoTaskStore) Get(ctx context.Context, id string) (*service.VideoTaskRecord, error) {
	if s.rdb == nil {
		if s.db != nil {
			return s.getDurable(ctx, id)
		}
		return nil, errors.New("video task storage is unavailable")
	}
	data, err := s.rdb.Get(ctx, videoTaskKey(id)).Bytes()
	if err != nil {
		if s.db != nil {
			return s.getDurable(ctx, id)
		}
		if err == redis.Nil {
			return nil, service.ErrVideoTaskNotFound
		}
		return nil, err
	}
	var task service.VideoTaskRecord
	if err := json.Unmarshal(data, &task); err != nil {
		return nil, err
	}
	return &task, nil
}

func (s *videoTaskStore) List(ctx context.Context, owner service.VideoTaskOwner, limit int) ([]*service.VideoTaskRecord, error) {
	if s.db != nil {
		return s.listDurable(ctx, owner, limit)
	}
	if s.rdb == nil {
		return nil, errors.New("video task storage is unavailable")
	}
	if limit <= 0 {
		limit = 10
	}
	fetchLimit := int64(min(limit*3, maxVideoTaskHistoryItems))
	historyKey := videoTaskHistoryKey(owner)
	ids, err := s.rdb.ZRevRange(ctx, historyKey, 0, fetchLimit-1).Result()
	if err != nil || len(ids) == 0 {
		return nil, err
	}
	keys := make([]string, len(ids))
	for index, id := range ids {
		keys[index] = videoTaskKey(id)
	}
	values, err := s.rdb.MGet(ctx, keys...).Result()
	if err != nil {
		return nil, err
	}
	tasks := make([]*service.VideoTaskRecord, 0, limit)
	staleIDs := make([]any, 0)
	for index, value := range values {
		raw, ok := value.(string)
		if !ok || strings.TrimSpace(raw) == "" {
			staleIDs = append(staleIDs, ids[index])
			continue
		}
		var task service.VideoTaskRecord
		if json.Unmarshal([]byte(raw), &task) != nil {
			staleIDs = append(staleIDs, ids[index])
			continue
		}
		if task.UserID == owner.UserID && task.APIKeyID == owner.APIKeyID {
			tasks = append(tasks, &task)
		}
		if len(tasks) >= limit {
			break
		}
	}
	if len(staleIDs) > 0 {
		_ = s.rdb.ZRem(ctx, historyKey, staleIDs...).Err()
	}
	return tasks, nil
}

func (s *videoTaskStore) Clear(ctx context.Context, owner service.VideoTaskOwner) error {
	durableHidden := false
	if s.db != nil {
		if err := s.hideDurable(ctx, owner); err != nil {
			return err
		}
		durableHidden = true
	}
	if s.rdb == nil {
		if durableHidden {
			return nil
		}
		return errors.New("video task storage is unavailable")
	}
	historyKey := videoTaskHistoryKey(owner)
	ids, err := s.rdb.ZRange(ctx, historyKey, 0, -1).Result()
	if err != nil {
		if durableHidden {
			return nil
		}
		return err
	}
	keys := []string{historyKey}
	for _, id := range ids {
		keys = append(keys, videoTaskKey(id))
	}
	err = s.rdb.Del(ctx, keys...).Err()
	if durableHidden {
		return nil
	}
	return err
}

func videoTaskKey(id string) string {
	return videoTaskKeyPrefix + strings.TrimSpace(id)
}

func videoTaskHistoryKey(owner service.VideoTaskOwner) string {
	return videoTaskHistoryKeyPrefix + strconv.FormatInt(owner.UserID, 10) + ":" + strconv.FormatInt(owner.APIKeyID, 10)
}
