package outbox

import (
	"context"
	"log"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"github.com/martinezpascualdani/heimdall/pkg/events"
)

const (
	StreamName      = "heimdall:execution:job_completed"
	PublishInterval = 2 * time.Second
	BatchSize       = 50
)

// OutboxStore is the interface required by the outbox-publisher.
type OutboxStore interface {
	FetchUnpublishedOutboxEvents(limit int) ([]events.OutboxEventRow, error)
	MarkOutboxPublished(id uuid.UUID, streamID string) error
}

// Publisher (outbox-publisher) publishes outbox_events to Redis. Only marks published_at and stream_id after XADD succeeds.
// v1 assumption: a single active outbox-publisher per execution-service DB; otherwise two publishers could fetch the same
// row after the fetch tx commits and both XADD, causing duplicate messages in Redis (inventory stays correct via idempotency).
type Publisher struct {
	Store OutboxStore
	Redis *redis.Client
}

// NewPublisher creates an outbox-publisher.
func NewPublisher(store OutboxStore, rdb *redis.Client) *Publisher {
	return &Publisher{Store: store, Redis: rdb}
}

// Run runs the publisher loop until ctx is done. Call from a goroutine.
func (p *Publisher) Run(ctx context.Context) {
	ticker := time.NewTicker(PublishInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := p.publishOnce(ctx); err != nil {
				log.Printf("outbox-publisher: publish batch: %v", err)
			}
		}
	}
}

func (p *Publisher) publishOnce(ctx context.Context) error {
	rows, err := p.Store.FetchUnpublishedOutboxEvents(BatchSize)
	if err != nil {
		return err
	}
	for _, row := range rows {
		streamID, err := p.Redis.XAdd(ctx, &redis.XAddArgs{
			Stream: StreamName,
			ID:     "*",
			Values: map[string]interface{}{
				"payload": string(row.Payload),
			},
		}).Result()
		if err != nil {
			log.Printf("outbox-publisher: XADD failed for outbox id=%s: %v", row.ID, err)
			continue
		}
		if err := p.Store.MarkOutboxPublished(row.ID, streamID); err != nil {
			log.Printf("outbox-publisher: MarkOutboxPublished failed for id=%s: %v", row.ID, err)
		}
	}
	return nil
}
