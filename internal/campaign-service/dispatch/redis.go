package dispatch

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/redis/go-redis/v9"
)

// RedisDispatcher publishes to Redis Streams.
type RedisDispatcher struct {
	Client *redis.Client
	Stream string
}

// NewRedisDispatcher creates a dispatcher. If stream is empty, uses StreamName.
func NewRedisDispatcher(client *redis.Client, stream string) *RedisDispatcher {
	if stream == "" {
		stream = StreamName
	}
	return &RedisDispatcher{Client: client, Stream: stream}
}

// Dispatch publishes the payload and returns the stream message ID.
func (d *RedisDispatcher) Dispatch(ctx context.Context, p *Payload) (streamID string, err error) {
	payloadBytes, err := json.Marshal(p)
	if err != nil {
		return "", fmt.Errorf("marshal payload: %w", err)
	}
	id, err := d.Client.XAdd(ctx, &redis.XAddArgs{
		Stream: d.Stream,
		ID:     "*",
		Values: map[string]interface{}{
			"payload": string(payloadBytes),
		},
	}).Result()
	if err != nil {
		return "", fmt.Errorf("redis XADD: %w", err)
	}
	return id, nil
}
