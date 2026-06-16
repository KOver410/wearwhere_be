//go:build integration

package service_test

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"

	"github.com/wearwhere/wearwhere_be/internal/recommendation/domain"
	"github.com/wearwhere/wearwhere_be/internal/recommendation/service"
)

func redisAddr() string {
	if a := os.Getenv("REDIS_ADDR"); a != "" {
		return a
	}
	return "localhost:6379"
}

func TestRedisCache_RoundTripAndInvalidate(t *testing.T) {
	ctx := context.Background()
	rdb := redis.NewClient(&redis.Options{Addr: redisAddr()})
	if err := rdb.Ping(ctx).Err(); err != nil {
		t.Skipf("redis not reachable at %s: %v", redisAddr(), err)
	}
	defer rdb.Close()

	c := service.NewRedisCache(rdb)
	uid := uuid.New()

	_, ok, err := c.Get(ctx, uid)
	require.NoError(t, err)
	require.False(t, ok)

	resp := &domain.RecommendationsResponse{
		Items:  []domain.RecProductCard{{ID: uuid.New().String(), Name: "X"}},
		Source: "personalized",
	}
	require.NoError(t, c.Set(ctx, uid, resp))

	got, ok, err := c.Get(ctx, uid)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "personalized", got.Source)
	require.Len(t, got.Items, 1)

	require.NoError(t, c.Invalidate(ctx, uid))
	_, ok, err = c.Get(ctx, uid)
	require.NoError(t, err)
	require.False(t, ok)
}
