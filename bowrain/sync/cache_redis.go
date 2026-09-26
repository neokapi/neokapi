package sync

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisHashCache implements HashCache using Redis hash maps.
//
// Keys: sync:{projectID}:gen holds the project's generation, and each cached
// set lives under that generation, per stream:
// sync:{projectID}:g{n}:items:{stream} and
// sync:{projectID}:g{n}:blocks:{stream}:{itemName}.
type RedisHashCache struct {
	client *redis.Client
	ttl    time.Duration
}

// NewRedisHashCache creates a Redis-backed hash cache.
func NewRedisHashCache(client *redis.Client, ttl time.Duration) *RedisHashCache {
	return &RedisHashCache{client: client, ttl: ttl}
}

func genKey(projectID string) string { return "sync:" + projectID + ":gen" }

func genPrefix(projectID string, gen int64) string {
	return "sync:" + projectID + ":g" + strconv.FormatInt(gen, 10) + ":"
}

// View pins the project's current generation. A generation that cannot be
// read yields a view that misses and stores nothing, so an unreachable Redis
// costs a store read and never a wrong answer.
func (c *RedisHashCache) View(ctx context.Context, projectID, stream string) HashView {
	gen, err := c.client.Get(ctx, genKey(projectID)).Int64()
	if err == redis.Nil {
		gen, err = 0, nil
	}
	if err != nil {
		return nopView{}
	}
	return &redisView{cache: c, prefix: genPrefix(projectID, gen), stream: stream}
}

// InvalidateProject advances the project's generation, then drops what older
// generations hold. A view taken before the advance files into a generation
// no later view reads, so hashes read from the store before a write can never
// answer for the content after it.
func (c *RedisHashCache) InvalidateProject(ctx context.Context, projectID string) {
	gen, err := c.client.Incr(ctx, genKey(projectID)).Result()
	if err != nil {
		return
	}
	current := genPrefix(projectID, gen)
	iter := c.client.Scan(ctx, 0, "sync:"+projectID+":g*", 100).Iterator()
	var stale []string
	for iter.Next(ctx) {
		if key := iter.Val(); key != genKey(projectID) && !strings.HasPrefix(key, current) {
			stale = append(stale, key)
		}
	}
	if len(stale) > 0 {
		c.client.Del(ctx, stale...)
	}
}

type redisView struct {
	cache  *RedisHashCache
	prefix string
	stream string
}

func (v *redisView) itemKey() string { return v.prefix + "items:" + v.stream }

func (v *redisView) blockKey(itemName string) string {
	return v.prefix + "blocks:" + v.stream + ":" + itemName
}

func (v *redisView) GetItemHashes(ctx context.Context) (map[string]string, bool) {
	return v.get(ctx, v.itemKey())
}

func (v *redisView) GetBlockHashes(ctx context.Context, itemName string) (map[string]string, bool) {
	return v.get(ctx, v.blockKey(itemName))
}

func (v *redisView) SetItemHashes(ctx context.Context, hashes map[string]string) {
	v.set(ctx, v.itemKey(), hashes)
}

func (v *redisView) SetBlockHashes(ctx context.Context, itemName string, hashes map[string]string) {
	v.set(ctx, v.blockKey(itemName), hashes)
}

func (v *redisView) get(ctx context.Context, key string) (map[string]string, bool) {
	result, err := v.cache.client.HGetAll(ctx, key).Result()
	if err != nil || len(result) == 0 {
		return nil, false
	}
	return result, true
}

func (v *redisView) set(ctx context.Context, key string, hashes map[string]string) {
	pipe := v.cache.client.Pipeline()
	pipe.Del(ctx, key)
	if len(hashes) > 0 {
		fields := make([]string, 0, len(hashes)*2)
		for k, h := range hashes {
			fields = append(fields, k, h)
		}
		pipe.HSet(ctx, key, fields)
		pipe.Expire(ctx, key, v.cache.ttl)
	}
	_, _ = pipe.Exec(ctx)
}

// nopView caches nothing.
type nopView struct{}

func (nopView) GetItemHashes(context.Context) (map[string]string, bool)          { return nil, false }
func (nopView) GetBlockHashes(context.Context, string) (map[string]string, bool) { return nil, false }
func (nopView) SetItemHashes(context.Context, map[string]string)                 {}
func (nopView) SetBlockHashes(context.Context, string, map[string]string)        {}
