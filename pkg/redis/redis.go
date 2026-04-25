package redis

import (
	"context"
	"fmt"
	"time"

	goredis "github.com/redis/go-redis/v9"
	"github.com/tienlao/agregator/pkg/config"
)

func NewClient(ctx context.Context, cfg config.RedisConfig) (*goredis.Client, error) {
	pool := cfg.PoolSize
	if pool <= 0 {
		pool = 32
	}
	minIdle := cfg.MinIdleConns
	if minIdle < 0 {
		minIdle = 0
	}
	if minIdle > pool {
		minIdle = pool
	}
	dialSec := cfg.DialTimeout
	if dialSec <= 0 {
		dialSec = 5
	}
	readSec := cfg.ReadTimeout
	if readSec <= 0 {
		readSec = 5
	}
	writeSec := cfg.WriteTimeout
	if writeSec <= 0 {
		writeSec = 5
	}

	client := goredis.NewClient(&goredis.Options{
		Addr:         cfg.Addr,
		Password:     cfg.Password,
		DB:           cfg.DB,
		PoolSize:     pool,
		MinIdleConns: minIdle,
		DialTimeout:  time.Duration(dialSec) * time.Second,
		ReadTimeout:  time.Duration(readSec) * time.Second,
		WriteTimeout: time.Duration(writeSec) * time.Second,
	})

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("ping redis: %w", err)
	}

	return client, nil
}
