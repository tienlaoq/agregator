package ratelimit

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

type Limiter interface {
	Allow(ctx context.Context, key string, limit int, window time.Duration) (bool, error)
}

type RedisLimiter struct {
	rdb *redis.Client
}

func NewRedisLimiter(rdb *redis.Client) *RedisLimiter {
	return &RedisLimiter{rdb: rdb}
}

// Allow increments per-key counter with TTL window.
func (l *RedisLimiter) Allow(ctx context.Context, key string, limit int, window time.Duration) (bool, error) {
	if l == nil || l.rdb == nil || limit <= 0 || window <= 0 {
		return true, nil
	}
	script := redis.NewScript(`
local k = KEYS[1]
local max = tonumber(ARGV[1])
local ttl = tonumber(ARGV[2])
local cur = redis.call("INCR", k)
if cur == 1 then
  redis.call("PEXPIRE", k, ttl)
end
if cur > max then
  return 0
end
return 1
`)
	v, err := script.Run(ctx, l.rdb, []string{key}, limit, int(window/time.Millisecond)).Int()
	if err != nil {
		return false, fmt.Errorf("redis limiter: %w", err)
	}
	return v == 1, nil
}

