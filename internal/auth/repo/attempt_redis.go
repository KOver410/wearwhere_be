package repo

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

type AttemptRedis struct{ rdb *redis.Client }

func NewAttemptRedis(rdb *redis.Client) *AttemptRedis { return &AttemptRedis{rdb: rdb} }

func attemptKey(contact string) string { return fmt.Sprintf("login:attempts:%s", contact) }
func lockKey(contact string) string    { return fmt.Sprintf("login:lock:%s", contact) }

func (r *AttemptRedis) IncrFailedLogin(ctx context.Context, contact string, ttl time.Duration) (int64, error) {
	key := attemptKey(contact)
	n, err := r.rdb.Incr(ctx, key).Result()
	if err != nil {
		return 0, err
	}
	if n == 1 {
		_ = r.rdb.Expire(ctx, key, ttl).Err()
	}
	return n, nil
}

func (r *AttemptRedis) ResetFailedLogin(ctx context.Context, contact string) error {
	return r.rdb.Del(ctx, attemptKey(contact)).Err()
}

func (r *AttemptRedis) Lock(ctx context.Context, contact string, ttl time.Duration) error {
	return r.rdb.Set(ctx, lockKey(contact), "1", ttl).Err()
}

func (r *AttemptRedis) IsLocked(ctx context.Context, contact string) (bool, error) {
	_, err := r.rdb.Get(ctx, lockKey(contact)).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}
