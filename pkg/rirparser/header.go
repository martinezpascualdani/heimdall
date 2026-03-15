package rirparser

import (
	"fmt"
	"strconv"
	"strings"
)

// Header represents the version line of an RIR delegated stats file.
// Format: version|registry|serial|records|startdate|enddate|UTCoffset
type Header struct {
	Version   int    // format version (currently 2)
	Registry  string // e.g. ripencc, apnic
	Serial    int64  // serial number within the RIR
	Records   int64  // number of record lines
	StartDate string // yyyymmdd
	EndDate   string // yyyymmdd
	UTCOffset string // e.g. +01
}

// ParseHeader parses the first version line of an RIR delegated file.
// Returns error if the line does not match the expected format.
func ParseHeader(line string) (*Header, error) {
	parts := strings.Split(line, "|")
	if len(parts) < 7 {
		return nil, fmt.Errorf("header: expected at least 7 pipe-separated fields, got %d", len(parts))
	}
	trim := func(s string) string { return strings.TrimSpace(s) }
	versionStr := trim(parts[0])
	if i := strings.Index(versionStr, "."); i >= 0 {
		versionStr = versionStr[:i] // e.g. "2.3" -> "2" (ARIN uses extended version)
	}
	version, err := strconv.Atoi(versionStr)
	if err != nil {
		return nil, fmt.Errorf("header version: %w", err)
	}
	serial, err := strconv.ParseInt(trim(parts[2]), 10, 64)
	if err != nil {
		return nil, fmt.Errorf("header serial: %w", err)
	}
	records, err := strconv.ParseInt(trim(parts[3]), 10, 64)
	if err != nil {
		return nil, fmt.Errorf("header records: %w", err)
	}
	return &Header{
		Version:   version,
		Registry:  trim(parts[1]),
		Serial:    serial,
		Records:   records,
		StartDate: trim(parts[4]),
		EndDate:   trim(parts[5]),
		UTCOffset: trim(parts[6]),
	}, nil
}

// ValidRegistry returns true if registry is one of the known RIRs.
func ValidRegistry(registry string) bool {
	switch strings.ToLower(registry) {
	case "afrinic", "apnic", "arin", "iana", "lacnic", "ripencc":
		return true
	}
	return false
}
