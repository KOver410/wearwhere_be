package service

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// QuotaGate tracks per-user daily message usage.
type QuotaGate interface {
	// Count returns today's current usage without mutating it.
	Count(ctx context.Context, userID uuid.UUID) (int, error)
	// Incr increments today's usage and returns the new value, setting a 24h TTL
	// on first use of the day.
	Incr(ctx context.Context, userID uuid.UUID) (int, error)
}

// RedisQuotaGate keys usage by user + UTC date: ai:quota:{userID}:{yyyymmdd}.
type RedisQuotaGate struct{ rdb *redis.Client }

func NewRedisQuotaGate(rdb *redis.Client) *RedisQuotaGate { return &RedisQuotaGate{rdb: rdb} }

func quotaKey(userID uuid.UUID) string {
	return fmt.Sprintf("ai:quota:%s:%s", userID.String(), time.Now().UTC().Format("20060102"))
}

func (g *RedisQuotaGate) Count(ctx context.Context, userID uuid.UUID) (int, error) {
	n, err := g.rdb.Get(ctx, quotaKey(userID)).Int()
	if err == redis.Nil {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return n, nil
}

func (g *RedisQuotaGate) Incr(ctx context.Context, userID uuid.UUID) (int, error) {
	key := quotaKey(userID)
	n, err := g.rdb.Incr(ctx, key).Result()
	if err != nil {
		return 0, err
	}
	if n == 1 {
		// First message today: expire the counter ~25h later so it self-cleans.
		_ = g.rdb.Expire(ctx, key, 25*time.Hour).Err()
	}
	return int(n), nil
}
