package database

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/Himank2026/task-execution-engine/backend/config"
)

// ConnectRedis builds a Redis client and verifies it with a ping so startup
// fails immediately if Redis is unreachable — the same fail-fast idea we use for
// MySQL. The returned client is safe for concurrent use across the app.
func ConnectRedis(cfg *config.Config) (*redis.Client, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     cfg.RedisAddr,
		Password: cfg.RedisPassword,
		DB:       0, // default logical database
	})

	// go-redis methods take a context; give the ping a 5s deadline so a dead
	// Redis can't hang startup forever.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("ping redis: %w", err)
	}

	return client, nil
}
