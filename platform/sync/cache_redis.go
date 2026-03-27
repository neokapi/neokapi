package sync

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisHashCache implements HashCache using Redis hash maps.
// Keys: sync:items:{projectID} and sync:blocks:{projectID}:{itemName}
type RedisHashCache struct {
	client *redis.Client
	ttl    time.Duration
}

// NewRedisHashCache creates a Redis-backed hash cache.
func NewRedisHashCache(client *redis.Client, ttl time.Duration) *RedisHashCache {
	return &RedisHashCache{client: client, ttl: ttl}
}

func (c *RedisHashCache) itemKey(projectID string) string {
	return "sync:items:" + projectID
}

func (c *RedisHashCache) blockKey(projectID, itemName string) string {
	return "sync:blocks:" + projectID + ":" + itemName
}

func (c *RedisHashCache) GetItemHashes(ctx context.Context, projectID string) (map[string]string, bool) {
	result, err := c.client.HGetAll(ctx, c.itemKey(projectID)).Result()
	if err != nil || len(result) == 0 {
		return nil, false
	}
	return result, true
}

func (c *RedisHashCache) GetBlockHashes(ctx context.Context, projectID, itemName string) (map[string]string, bool) {
	result, err := c.client.HGetAll(ctx, c.blockKey(projectID, itemName)).Result()
	if err != nil || len(result) == 0 {
		return nil, false
	}
	return result, true
}

func (c *RedisHashCache) SetItemHashes(ctx context.Context, projectID string, hashes map[string]string) {
	key := c.itemKey(projectID)
	pipe := c.client.Pipeline()
	pipe.Del(ctx, key)
	if len(hashes) > 0 {
		fields := make([]string, 0, len(hashes)*2)
		for k, v := range hashes {
			fields = append(fields, k, v)
		}
		pipe.HSet(ctx, key, fields)
		pipe.Expire(ctx, key, c.ttl)
	}
	_, _ = pipe.Exec(ctx)
}

func (c *RedisHashCache) SetBlockHashes(ctx context.Context, projectID, itemName string, hashes map[string]string) {
	key := c.blockKey(projectID, itemName)
	pipe := c.client.Pipeline()
	pipe.Del(ctx, key)
	if len(hashes) > 0 {
		fields := make([]string, 0, len(hashes)*2)
		for k, v := range hashes {
			fields = append(fields, k, v)
		}
		pipe.HSet(ctx, key, fields)
		pipe.Expire(ctx, key, c.ttl)
	}
	_, _ = pipe.Exec(ctx)
}

func (c *RedisHashCache) InvalidateProject(ctx context.Context, projectID string) {
	// Delete item hashes.
	c.client.Del(ctx, c.itemKey(projectID))

	// Delete all block hash keys for this project (scan for pattern).
	pattern := "sync:blocks:" + projectID + ":*"
	iter := c.client.Scan(ctx, 0, pattern, 100).Iterator()
	var keys []string
	for iter.Next(ctx) {
		keys = append(keys, iter.Val())
	}
	if len(keys) > 0 {
		c.client.Del(ctx, keys...)
	}
}
