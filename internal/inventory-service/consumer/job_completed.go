package consumer

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"github.com/martinezpascualdani/heimdall/internal/inventory-service/storage"
	"github.com/martinezpascualdani/heimdall/pkg/events"
)

const (
	StreamName         = "heimdall:execution:job_completed"
	ConsumerGroup      = "inventory-service"
	ConsumerNamePrefix = "inventory-service-"
	ReadBlockDuration  = 5 * time.Second
)

// Store is the interface required by the job_completed consumer.
type Store interface {
	IngestJobCompleted(executionID, jobID, runID, campaignID, targetID, targetMatID uuid.UUID, scanProfile string, observedAt time.Time, observations []storage.IngestObservation) error
}

// JobCompletedConsumer consumes job_completed events from Redis and ingests into inventory.
// It uses XReadGroup with ">" (new messages only). Messages already in PEL from a dead consumer are not
// reclaimed in v1; XAUTOCLAIM or similar can be added later to process stuck PEL entries.
type JobCompletedConsumer struct {
	Redis *redis.Client
	Store Store
}

// NewJobCompletedConsumer creates a consumer. Consumer name will be inventory-service-<hostname>-<short-uuid>.
func NewJobCompletedConsumer(rdb *redis.Client, store Store) *JobCompletedConsumer {
	return &JobCompletedConsumer{Redis: rdb, Store: store}
}

// EnsureConsumerGroup creates the consumer group with start id "0" (consume from beginning of stream).
func (c *JobCompletedConsumer) EnsureConsumerGroup(ctx context.Context) error {
	err := c.Redis.XGroupCreateMkStream(ctx, StreamName, ConsumerGroup, "0").Err()
	if err != nil && err.Error() != "BUSYGROUP Consumer Group name already exists" {
		return err
	}
	return nil
}

// Run runs the consumer loop until ctx is done. Call from a goroutine.
func (c *JobCompletedConsumer) Run(ctx context.Context) {
	consumerName := consumerName()
	for {
		select {
		case <-ctx.Done():
			return
		default:
			processed, err := c.ProcessOne(ctx, consumerName)
			if err != nil {
				log.Printf("inventory-service consumer: %v", err)
			}
			if !processed && err == nil {
				// No message; avoid tight loop
				select {
				case <-ctx.Done():
					return
				case <-time.After(100 * time.Millisecond):
				}
			}
		}
	}
}

func consumerName() string {
	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = "unknown"
	}
	return ConsumerNamePrefix + hostname + "-" + uuid.New().String()[:8]
}

// ProcessOne reads one message, processes it, and ACKs only on success or ErrAlreadyIngested or invalid payload (ACK to avoid block).
func (c *JobCompletedConsumer) ProcessOne(ctx context.Context, consumerName string) (processed bool, err error) {
	streams, err := c.Redis.XReadGroup(ctx, &redis.XReadGroupArgs{
		Group:    ConsumerGroup,
		Consumer: consumerName,
		Streams:  []string{StreamName, ">"},
		Count:    1,
		Block:    ReadBlockDuration,
	}).Result()
	if err != nil {
		if err == redis.Nil {
			return false, nil
		}
		return false, err
	}
	if len(streams) == 0 || len(streams[0].Messages) == 0 {
		return false, nil
	}
	msg := streams[0].Messages[0]
	id := msg.ID
	payloadStr, ok := msg.Values["payload"].(string)
	if !ok {
		log.Printf("inventory-service consumer: message %s missing payload, acking (invalid)", id)
		_ = c.Redis.XAck(ctx, StreamName, ConsumerGroup, id).Err()
		return true, nil
	}
	var evt events.JobCompletedEvent
	if err := json.Unmarshal([]byte(payloadStr), &evt); err != nil {
		log.Printf("inventory-service consumer: invalid payload json %s: %v (ack to avoid block)", id, err)
		_ = c.Redis.XAck(ctx, StreamName, ConsumerGroup, id).Err()
		return true, nil
	}
	if !validateEvent(&evt) {
		log.Printf("inventory-service consumer: structurally invalid payload %s (ack + alert)", id)
		_ = c.Redis.XAck(ctx, StreamName, ConsumerGroup, id).Err()
		return true, nil
	}
	executionID, _ := uuid.Parse(evt.ExecutionID)
	jobID, _ := uuid.Parse(evt.JobID)
	runID, _ := uuid.Parse(evt.RunID)
	campaignID, _ := uuid.Parse(evt.CampaignID)
	targetID, _ := uuid.Parse(evt.TargetID)
	targetMatID, _ := uuid.Parse(evt.TargetMaterializationID)
	obs := make([]storage.IngestObservation, len(evt.Observations))
	for i, o := range evt.Observations {
		obs[i] = storage.IngestObservation{IP: o.IP, Port: o.Port, Status: o.Status}
	}
	err = c.Store.IngestJobCompleted(executionID, jobID, runID, campaignID, targetID, targetMatID, evt.ScanProfileSlug, evt.ObservedAt, obs)
	if err != nil {
		if errors.Is(err, storage.ErrAlreadyIngested) {
			_ = c.Redis.XAck(ctx, StreamName, ConsumerGroup, id).Err()
			return true, nil
		}
		// Transient: do not ACK so message stays in PEL for retry
		return false, err
	}
	_ = c.Redis.XAck(ctx, StreamName, ConsumerGroup, id).Err()
	return true, nil
}

func validateEvent(evt *events.JobCompletedEvent) bool {
	if evt.EventType != events.JobCompletedEventType || evt.PayloadVersion != events.JobCompletedPayloadVersion {
		return false
	}
	if evt.ExecutionID == "" || evt.JobID == "" || evt.RunID == "" || evt.CampaignID == "" || evt.TargetID == "" || evt.TargetMaterializationID == "" {
		return false
	}
	if evt.ObservedAt.IsZero() {
		return false
	}
	if _, err := uuid.Parse(evt.ExecutionID); err != nil {
		return false
	}
	if _, err := uuid.Parse(evt.JobID); err != nil {
		return false
	}
	return true
}
