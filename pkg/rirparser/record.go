package rirparser

import (
	"fmt"
	"strings"
)

// RecordType is the type of resource in a delegated record.
type RecordType string

const (
	TypeIPv4 RecordType = "ipv4"
	TypeIPv6 RecordType = "ipv6"
	TypeASN  RecordType = "asn"
)

// Record represents one line of an RIR delegated file after the header.
// Format: registry|cc|type|start|value|date|status[|extensions...]
type Record struct {
	Registry string     // e.g. ripencc
	CC       string     // ISO 3166 2-letter country code
	Type     RecordType // ipv4, ipv6, asn
	Start    string     // first address or AS number
	Value    string     // count (IPv4 hosts) or prefix length (IPv6) or AS count
	Date     string     // YYYYMMDD
	Status   string     // allocated, assigned, available, reserved, etc.
}

// ParseRecord parses a single record line. Returns nil if the line is a summary
// line, comment, or blank. Caller should filter by type (ipv4/ipv6) and non-empty cc.
func ParseRecord(line string) (*Record, error) {
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, "#") {
		return nil, nil
	}
	parts := strings.Split(line, "|")
	if len(parts) < 6 {
		return nil, fmt.Errorf("record: expected at least 6 fields, got %d", len(parts))
	}
	trim := func(s string) string { return strings.TrimSpace(s) }
	recType := trim(parts[2])
	if recType == "summary" || strings.TrimSpace(parts[1]) == "*" {
		return nil, nil // summary line
	}
	status := ""
	if len(parts) > 6 {
		status = trim(parts[6])
	}
	return &Record{
		Registry: trim(parts[0]),
		CC:       trim(parts[1]),
		Type:     RecordType(recType),
		Start:    trim(parts[3]),
		Value:    trim(parts[4]),
		Date:     trim(parts[5]),
		Status:   status,
	}, nil
}

// BlockRawIdentity returns the canonical identity for deduplication.
// IPv4: start|count|status|cc; IPv6: prefix|prefix_length|status|cc
func (r *Record) BlockRawIdentity() string {
	return strings.Join([]string{r.Registry, r.CC, string(r.Type), r.Start, r.Value, r.Date, r.Status}, "|")
}
