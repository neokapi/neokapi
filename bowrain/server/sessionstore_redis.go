package server

import (
	"context"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisSessionStore implements SessionStateStore using Redis with SETEX
// for automatic key expiry. Suitable for multi-instance deployments.
type RedisSessionStore struct {
	client *redis.Client
}

// NewRedisSessionStore creates a Redis-backed session store. The redisURL
// should be a Redis connection string (e.g. "redis://localhost:6379" or
// "rediss://host:10000" for TLS). If password is non-empty, it overrides
// any password in the URL.
func NewRedisSessionStore(redisURL, password string) (*RedisSessionStore, error) {
	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, err
	}
	if password != "" {
		opts.Password = password
	}

	client := redis.NewClient(opts)

	// Verify connectivity.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		client.Close()
		return nil, err
	}

	return &RedisSessionStore{client: client}, nil
}

func (s *RedisSessionStore) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	return s.client.Set(ctx, key, value, ttl).Err()
}

func (s *RedisSessionStore) Get(ctx context.Context, key string) ([]byte, error) {
	val, err := s.client.Get(ctx, key).Bytes()
	if errors.Is(err, redis.Nil) {
		return nil, ErrSessionNotFound
	}
	return val, err
}

func (s *RedisSessionStore) Delete(ctx context.Context, key string) error {
	return s.client.Del(ctx, key).Err()
}

// Close closes the Redis client connection.
func (s *RedisSessionStore) Close() error {
	return s.client.Close()
}
