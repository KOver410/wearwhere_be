package repo

import (
	"context"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"
)

type OTPRedis struct{ rdb *redis.Client }

func NewOTPRedis(rdb *redis.Client) *OTPRedis { return &OTPRedis{rdb: rdb} }

func (r *OTPRedis) Save(ctx context.Context, key, code string, ttl time.Duration) error {
	return r.rdb.Set(ctx, key, code, ttl).Err()
}

func (r *OTPRedis) Get(ctx context.Context, key string) (string, error) {
	v, err := r.rdb.Get(ctx, key).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return "", ErrNotFound
		}
		return "", err
	}
	return v, nil
}

func (r *OTPRedis) Delete(ctx context.Context, key string) error {
	return r.rdb.Del(ctx, key).Err()
}

func (r *OTPRedis) IncrCounter(ctx context.Context, key string, ttl time.Duration) (int64, error) {
	n, err := r.rdb.Incr(ctx, key).Result()
	if err != nil {
		return 0, err
	}
	if n == 1 {
		// First time → set TTL
		_ = r.rdb.Expire(ctx, key, ttl).Err()
	}
	return n, nil
}
