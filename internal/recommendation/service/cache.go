package service

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"github.com/wearwhere/wearwhere_be/internal/recommendation/domain"
)

// Cache stores the assembled feed per user per day. Implementations must treat
// a miss as (nil, false, nil) — not an error.
type Cache interface {
	Get(ctx context.Context, userID uuid.UUID) (*domain.RecommendationsResponse, bool, error)
	Set(ctx context.Context, userID uuid.UUID, resp *domain.RecommendationsResponse) error
	Invalidate(ctx context.Context, userID uuid.UUID) error
}

// RedisCache is the production Cache. Key: rec:feed:{user}:{yyyymmdd}. The key
// is stamped with the UTC date, so a new key is used each day and yesterday's
// key is never read — the 24h TTL only garbage-collects stale entries, which
// realises the spec's "update daily" / end-of-day freshness.
type RedisCache struct{ rdb *redis.Client }

func NewRedisCache(rdb *redis.Client) *RedisCache { return &RedisCache{rdb: rdb} }

func dayKey(userID uuid.UUID, now time.Time) string {
	return "rec:feed:" + userID.String() + ":" + now.UTC().Format("20060102")
}

func (c *RedisCache) Get(ctx context.Context, userID uuid.UUID) (*domain.RecommendationsResponse, bool, error) {
	raw, err := c.rdb.Get(ctx, dayKey(userID, time.Now())).Bytes()
	if err == redis.Nil {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	var resp domain.RecommendationsResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, false, nil
	}
	return &resp, true, nil
}

func (c *RedisCache) Set(ctx context.Context, userID uuid.UUID, resp *domain.RecommendationsResponse) error {
	raw, err := json.Marshal(resp)
	if err != nil {
		return err
	}
	return c.rdb.Set(ctx, dayKey(userID, time.Now()), raw, 24*time.Hour).Err()
}

func (c *RedisCache) Invalidate(ctx context.Context, userID uuid.UUID) error {
	return c.rdb.Del(ctx, dayKey(userID, time.Now())).Err()
}
