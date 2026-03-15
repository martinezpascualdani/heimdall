package engine

import (
	"context"
	"encoding/json"
)

// JobPayload is the parsed payload from an execution job (prefixes + engine + port(s) or range).
type JobPayload struct {
	Prefixes      []string `json:"prefixes"`
	Engine        string   `json:"engine"`
	Port          int      `json:"port,omitempty"`           // single port; 0 = use Ports or default
	Ports         []int    `json:"ports,omitempty"`         // list of ports (e.g. portscan-basic)
	PortRangeStart int `json:"port_range_start,omitempty"` // portscan-full: start (inclusive)
	PortRangeEnd   int `json:"port_range_end,omitempty"`   // portscan-full: end (inclusive)
}

// ParseJobPayload parses the job payload JSON.
func ParseJobPayload(raw json.RawMessage) (*JobPayload, error) {
	var p JobPayload
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, err
	}
	return &p, nil
}

// Observation is a single result (e.g. IP + port open or host signal).
type Observation struct {
	IP     string `json:"ip"`
	Port   int    `json:"port,omitempty"`
	Status string `json:"status,omitempty"`
}

// PortDiscoveryResult is the result of a port discovery run.
type PortDiscoveryResult struct {
	Observations []Observation `json:"observations"`
	Error        string        `json:"error,omitempty"`
}

// PortDiscoveryEngine runs port discovery (e.g. ZMap) and returns normalized observations.
type PortDiscoveryEngine interface {
	Run(ctx context.Context, payload *JobPayload) (*PortDiscoveryResult, error)
}
