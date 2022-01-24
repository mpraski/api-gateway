package store

import (
	"context"
	"fmt"
	"time"

	"github.com/go-redis/redis/v8"
)

type (
	RedisStore struct {
		client *redis.Client
	}

	RedisConfig struct {
		Port int
		Host string
	}
)

func NewRedisStore(config RedisConfig) *RedisStore {
	return &RedisStore{client: redis.NewClient(&redis.Options{
		Addr: fmt.Sprintf("%s:%d", config.Host, config.Port),
	})}
}

func (r *RedisStore) Get(ctx context.Context, key string) (string, error) {
	return r.client.Get(ctx, key).Result()
}

func (r *RedisStore) Set(ctx context.Context, key, value string, expiration time.Duration) error {
	return r.client.Set(ctx, key, value, expiration).Err()
}

func (r *RedisStore) Del(ctx context.Context, key string) error {
	return r.client.Del(ctx, key).Err()
}
