package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisClient wraps the go-redis client.
type RedisClient struct {
	client *redis.Client
}

// NewRedisClient creates a new Redis client connection.
func NewRedisClient() *RedisClient {
	redisURL := os.Getenv("REDIS_URL")
	if redisURL == "" {
		redisURL = "redis://redis:6379/0" // Default for docker-compose
	}

	opt, err := redis.ParseURL(redisURL)
	if err != nil {
		fmt.Printf("Failed to parse REDIS_URL: %v\n", err)
		return nil
	}

	client := redis.NewClient(opt)
	return &RedisClient{
		client: client,
	}
}

// Set stores a struct in Redis as JSON.
func (r *RedisClient) Set(ctx context.Context, key string, value interface{}, expiration time.Duration) error {
	if r.client == nil {
		return nil // Graceful degradation if Redis is down
	}

	bytes, err := json.Marshal(value)
	if err != nil {
		return err
	}

	return r.client.Set(ctx, key, bytes, expiration).Err()
}

// Get retrieves a JSON string from Redis and unmarshals it into dest.
func (r *RedisClient) Get(ctx context.Context, key string, dest interface{}) error {
	if r.client == nil {
		return redis.Nil // Simulate cache miss
	}

	val, err := r.client.Get(ctx, key).Result()
	if err != nil {
		return err
	}

	return json.Unmarshal([]byte(val), dest)
}

// Delete removes a key from Redis.
func (r *RedisClient) Delete(ctx context.Context, key string) error {
	if r.client == nil {
		return nil
	}
	return r.client.Del(ctx, key).Err()
}
