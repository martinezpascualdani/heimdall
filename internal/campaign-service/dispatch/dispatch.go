package dispatch

import (
	"context"
	"encoding/json"
	"time"
)

const (
	// StreamName is the Redis stream for campaign run dispatch.
	StreamName = "heimdall:campaign:runs"
)

// Payload is the stable contract for a run dispatch message (v1).
type Payload struct {
	RunID                   string          `json:"run_id"`
	CampaignID              string          `json:"campaign_id"`
	TargetID                string          `json:"target_id"`
	TargetMaterializationID string          `json:"target_materialization_id"`
	ScanProfileSlug         string          `json:"scan_profile_slug"`
	ScanProfileConfig       json.RawMessage `json:"scan_profile_config"`
	CreatedAt               time.Time       `json:"created_at"`
	DispatchAttempt         int             `json:"dispatch_attempt"`
	TraceID                 string          `json:"trace_id,omitempty"`
}

// Dispatcher publishes run payloads to the queue. Campaign-service only publishes; consumers use consumer groups.
type Dispatcher interface {
	// Dispatch publishes the payload to the stream. Returns the Redis stream message ID (for dispatch_ref) or error.
	Dispatch(ctx context.Context, p *Payload) (streamID string, err error)
}
