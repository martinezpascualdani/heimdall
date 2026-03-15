package ingest

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"github.com/martinezpascualdani/heimdall/internal/execution-service/domain"
	"github.com/martinezpascualdani/heimdall/internal/execution-service/targetclient"
)

const (
	StreamName         = "heimdall:campaign:runs"
	ConsumerGroup      = "execution-service"
	ConsumerNamePrefix = "execution-service-"
)

// IngestPayload matches the campaign-service dispatch payload (v1 contract).
type IngestPayload struct {
	RunID                   string          `json:"run_id"`
	CampaignID              string          `json:"campaign_id"`
	TargetID                string          `json:"target_id"`
	TargetMaterializationID string          `json:"target_materialization_id"`
	ScanProfileSlug         string          `json:"scan_profile_slug"`
	ScanProfileConfig       json.RawMessage `json:"scan_profile_config"`
}

// Store is the persistence interface required by the ingest.
type Store interface {
	GetExecutionByRunID(runID uuid.UUID) (*domain.Execution, error)
	CreateExecution(e *domain.Execution) error
	CreateExecutionWithJobs(e *domain.Execution, jobs []*domain.ExecutionJob) error
}

// Config holds ingest configuration.
type Config struct {
	PrefixesPageSize  int
	PrefixesMax       int
	JobsPrefixBatch   int
	ReadBlockDuration time.Duration
}

func (c *Config) setDefaults() {
	if c.PrefixesPageSize <= 0 {
		c.PrefixesPageSize = 1000
	}
	if c.JobsPrefixBatch <= 0 {
		c.JobsPrefixBatch = 10
	}
	if c.ReadBlockDuration <= 0 {
		c.ReadBlockDuration = 5 * time.Second
	}
}

// Consumer consumes run messages from Redis and creates Execution + Jobs.
type Consumer struct {
	Redis  *redis.Client
	Store  Store
	Target *targetclient.Client
	Config Config
	logger *log.Logger
}

// NewConsumer creates an ingest consumer.
func NewConsumer(rdb *redis.Client, store Store, target *targetclient.Client, cfg Config) *Consumer {
	cfg.setDefaults()
	return &Consumer{Redis: rdb, Store: store, Target: target, Config: cfg, logger: log.Default()}
}

// SetLogger sets the logger (optional).
func (c *Consumer) SetLogger(l *log.Logger) {
	if l != nil {
		c.logger = l
	}
}

// EnsureConsumerGroup creates the stream consumer group if it does not exist.
func (c *Consumer) EnsureConsumerGroup(ctx context.Context) error {
	err := c.Redis.XGroupCreateMkStream(ctx, StreamName, ConsumerGroup, "0").Err()
	if err != nil && err.Error() != "BUSYGROUP Consumer Group name already exists" {
		return err
	}
	return nil
}

// ProcessOne reads one message from the stream (blocking), processes it, and ACKs on success.
func (c *Consumer) ProcessOne(ctx context.Context) (processed bool, err error) {
	consumerName := ConsumerNamePrefix + uuid.New().String()[:8]
	streams, err := c.Redis.XReadGroup(ctx, &redis.XReadGroupArgs{
		Group:    ConsumerGroup,
		Consumer: consumerName,
		Streams:  []string{StreamName, ">"},
		Count:    1,
		Block:    c.Config.ReadBlockDuration,
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
		c.logger.Printf("ingest: message %s missing payload, acking", id)
		_ = c.Redis.XAck(ctx, StreamName, ConsumerGroup, id).Err()
		return true, nil
	}
	var p IngestPayload
	if err := json.Unmarshal([]byte(payloadStr), &p); err != nil {
		c.logger.Printf("ingest: invalid payload %s: %v", id, err)
		return false, err
	}
	runID, err := uuid.Parse(p.RunID)
	if err != nil {
		c.logger.Printf("ingest: invalid run_id in message %s", id)
		_ = c.Redis.XAck(ctx, StreamName, ConsumerGroup, id).Err()
		return true, nil
	}
	campaignID, _ := uuid.Parse(p.CampaignID)
	targetID, _ := uuid.Parse(p.TargetID)
	matID, _ := uuid.Parse(p.TargetMaterializationID)
	if campaignID == uuid.Nil || targetID == uuid.Nil || matID == uuid.Nil {
		c.logger.Printf("ingest: missing/invalid UUIDs in message %s", id)
		_ = c.Redis.XAck(ctx, StreamName, ConsumerGroup, id).Err()
		return true, nil
	}

	existing, _ := c.Store.GetExecutionByRunID(runID)
	if existing != nil {
		c.logger.Printf("ingest: execution already exists for run_id %s, acking", runID)
		_ = c.Redis.XAck(ctx, StreamName, ConsumerGroup, id).Err()
		return true, nil
	}

	prefixes, err := c.Target.GetAllPrefixes(ctx, targetID, matID, c.Config.PrefixesPageSize, c.Config.PrefixesMax)
	if err != nil {
		c.logger.Printf("ingest: target-service prefixes failed for run %s: %v", runID, err)
		return false, err
	}

	exec := &domain.Execution{
		RunID:                   runID,
		CampaignID:              campaignID,
		TargetID:                targetID,
		TargetMaterializationID: matID,
		ScanProfileSlug:         p.ScanProfileSlug,
		ScanProfileConfig:       p.ScanProfileConfig,
	}

	if len(prefixes) == 0 {
		exec.Status = domain.ExecutionStatusCompleted
		exec.TotalJobs = 0
		if err := c.Store.CreateExecution(exec); err != nil {
			return false, err
		}
		_ = c.Redis.XAck(ctx, StreamName, ConsumerGroup, id).Err()
		return true, nil
	}

	port := extractPortFromScanProfileConfig(p.ScanProfileConfig)
	portsList := extractPortsFromScanProfileConfig(p.ScanProfileConfig)
	slug := p.ScanProfileSlug

	var jobs []*domain.ExecutionJob
	batchSize := c.Config.JobsPrefixBatch
	for i := 0; i < len(prefixes); i += batchSize {
		end := i + batchSize
		if end > len(prefixes) {
			end = len(prefixes)
		}
		batch := prefixes[i:end]
		basePl := map[string]interface{}{"prefixes": batch, "engine": slug}

		switch slug {
		case "portscan-basic":
			if len(portsList) > 0 {
				basePl["ports"] = portsList
			} else {
				basePl["ports"] = defaultPortscanBasicPorts
			}
		case "portscan-full", "portscan-medium":
			// One job per port-range chunk (e.g. 1-5000, 5001-10000, ...) so each job has bounded work
			for chunkStart := 1; chunkStart <= 65535; chunkStart += portscanFullChunkSize {
				chunkEnd := chunkStart + portscanFullChunkSize - 1
				if chunkEnd > 65535 {
					chunkEnd = 65535
				}
				pl := make(map[string]interface{})
				for k, v := range basePl {
					pl[k] = v
				}
				pl["port_range_start"] = chunkStart
				pl["port_range_end"] = chunkEnd
				payloadBytes, _ := json.Marshal(pl)
				jobs = append(jobs, &domain.ExecutionJob{
					Payload:     payloadBytes,
					Status:      domain.JobStatusPending,
					MaxAttempts: 3,
				})
			}
			continue
		default:
			if port > 0 {
				basePl["port"] = port
			}
		}

		payloadBytes, _ := json.Marshal(basePl)
		jobs = append(jobs, &domain.ExecutionJob{
			Payload:     payloadBytes,
			Status:      domain.JobStatusPending,
			MaxAttempts: 3,
		})
	}
	exec.TotalJobs = len(jobs)
	if err := c.Store.CreateExecutionWithJobs(exec, jobs); err != nil {
		return false, err
	}
	_ = c.Redis.XAck(ctx, StreamName, ConsumerGroup, id).Err()
	return true, nil
}

// defaultPortscanBasicPorts: FTP, SSH, HTTP, HTTPS, RDP, MySQL, HTTP-alt, HTTPS-alt, Postgres, MongoDB
var defaultPortscanBasicPorts = []int{21, 22, 80, 443, 3389, 3306, 8080, 8443, 5432, 27017}

const portscanFullChunkSize = 5000 // ports per job for portscan-full (1-5000, 5001-10000, ...)

// extractPortsFromScanProfileConfig reads optional "ports" array from scan profile config. Returns nil if absent.
func extractPortsFromScanProfileConfig(raw json.RawMessage) []int {
	if len(raw) == 0 {
		return nil
	}
	var m map[string]interface{}
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil
	}
	v, ok := m["ports"]
	if !ok {
		return nil
	}
	arr, ok := v.([]interface{})
	if !ok {
		return nil
	}
	var out []int
	for _, x := range arr {
		n, ok := toInt(x)
		if ok && n > 0 && n <= 65535 {
			out = append(out, n)
		}
	}
	return out
}

// extractPortFromScanProfileConfig reads optional "port" or "target_port" from scan profile config JSON. Returns 0 if absent or invalid.
func extractPortFromScanProfileConfig(raw json.RawMessage) int {
	if len(raw) == 0 {
		return 0
	}
	var m map[string]interface{}
	if err := json.Unmarshal(raw, &m); err != nil {
		return 0
	}
	if v, ok := m["port"]; ok {
		if n, ok := toInt(v); ok && n > 0 && n <= 65535 {
			return n
		}
	}
	if v, ok := m["target_port"]; ok {
		if n, ok := toInt(v); ok && n > 0 && n <= 65535 {
			return n
		}
	}
	return 0
}

func toInt(v interface{}) (int, bool) {
	switch x := v.(type) {
	case float64:
		return int(x), true
	case int:
		return x, true
	default:
		return 0, false
	}
}

// Run runs the consumer loop until ctx is cancelled.
func (c *Consumer) Run(ctx context.Context) {
	if err := c.EnsureConsumerGroup(ctx); err != nil {
		c.logger.Printf("ingest: ensure consumer group: %v", err)
		return
	}
	for {
		select {
		case <-ctx.Done():
			return
		default:
			_, err := c.ProcessOne(ctx)
			if err != nil {
				c.logger.Printf("ingest: process one: %v", err)
			}
		}
	}
}
