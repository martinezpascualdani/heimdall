package importsvc

import (
	"bufio"
	"compress/gzip"
	"encoding/json"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/martinezpascualdani/heimdall/internal/routing-service/domain"
)

// Pfx2asCallback is called for each parsed BGP prefix origin (streaming).
type Pfx2asCallback func(*domain.BGPPrefixOrigin) error

// ParsePfx2asStream reads a gzip stream of tab-separated lines (prefix, prefix_length, AS) and calls fn for each record.
func ParsePfx2asStream(r io.Reader, datasetID uuid.UUID, source, ipVersion string, fn Pfx2asCallback) (count int64, err error) {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return 0, err
	}
	defer gz.Close()
	sc := bufio.NewScanner(gz)
	sc.Buffer(nil, 512*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.Split(line, "\t")
		if len(parts) < 3 {
			continue
		}
		prefixStr := strings.TrimSpace(parts[0])
		lenStr := strings.TrimSpace(parts[1])
		asStr := strings.TrimSpace(parts[2])
		if prefixStr == "" || asStr == "" {
			continue
		}
		prefixLen, err := strconv.Atoi(lenStr)
		if err != nil || prefixLen < 0 || prefixLen > 128 {
			continue
		}
		cidr := prefixStr + "/" + lenStr
		asnRaw := asStr
		asnType := "single"
		var primaryASN *int64
		if first := firstASNFromField(asStr); first >= 0 {
			primaryASN = &first
		}
		if strings.Contains(asStr, "_") || strings.Contains(asStr, ",") || strings.Contains(asStr, " ") {
			asnType = "multi"
			if primaryASN == nil {
				if f := firstASNFromField(asStr); f >= 0 {
					primaryASN = &f
				}
			}
		}
		o := &domain.BGPPrefixOrigin{
			ID:           uuid.New(),
			DatasetID:    datasetID,
			Source:       source,
			IPVersion:    ipVersion,
			Prefix:       cidr,
			PrefixLength: prefixLen,
			ASNRaw:       asnRaw,
			PrimaryASN:   primaryASN,
			ASNType:      asnType,
			CreatedAt:    time.Now(),
		}
		if err := fn(o); err != nil {
			return count, err
		}
		count++
	}
	return count, sc.Err()
}

func firstASNFromField(s string) int64 {
	// Replace common separators with space and take first number
	s = strings.ReplaceAll(s, "_", " ")
	s = strings.ReplaceAll(s, ",", " ")
	for _, f := range strings.Fields(s) {
		n, err := strconv.ParseInt(f, 10, 64)
		if err == nil && n >= 0 {
			return n
		}
	}
	return -1
}

// AsOrgLine is one line of CAIDA as-org2info JSONL. Format uses "type":"ASN" for AS entries,
// "asn" as string or number, and "organizationId" (not org_id). We only process type=="ASN".
type AsOrgLine struct {
	Type string `json:"type"`
	ASN  any    `json:"asn"` // CAIDA sends string e.g. "1"; accept string or number
	OrgID string `json:"org_id,omitempty"`
	OrganizationID string `json:"organizationId,omitempty"` // CAIDA field name
	OrgName string `json:"org_name,omitempty"`
	Name   string `json:"name,omitempty"`
}

// ASOrgCallback is called for each parsed AS org line (streaming).
type ASOrgCallback func(*domain.ASNMetadata) error

// parseAsnFromLine returns the ASN as int64 from the line (ASN can be string or number in JSON).
func parseAsnFromLine(line *AsOrgLine) (int64, bool) {
	if line.ASN == nil {
		return 0, false
	}
	switch v := line.ASN.(type) {
	case float64:
		if v >= 1 && v <= 1<<32-1 {
			return int64(v), true
		}
		return 0, false
	case string:
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil || n <= 0 {
			return 0, false
		}
		return n, true
	default:
		return 0, false
	}
}

// ParseASOrgStream reads gzip JSONL and calls fn for each ASN metadata record (type "ASN" only).
func ParseASOrgStream(r io.Reader, source string, sourceDatasetID *uuid.UUID, fn ASOrgCallback) (count int64, err error) {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return 0, err
	}
	defer gz.Close()
	sc := bufio.NewScanner(gz)
	sc.Buffer(nil, 256*1024)
	for sc.Scan() {
		var line AsOrgLine
		if err := json.Unmarshal(sc.Bytes(), &line); err != nil {
			continue
		}
		if line.Type != "ASN" {
			continue
		}
		asn, ok := parseAsnFromLine(&line)
		if !ok {
			continue
		}
		orgID := line.OrgID
		if orgID == "" {
			orgID = line.OrganizationID
		}
		t := time.Now()
		m := &domain.ASNMetadata{
			ASN:              asn,
			Source:           source,
			SourceDatasetID:  sourceDatasetID,
			CreatedAt:        t,
			UpdatedAt:        t,
		}
		if line.Name != "" {
			m.ASName = &line.Name
		}
		if orgID != "" {
			m.OrgID = &orgID
		}
		if line.OrgName != "" {
			m.OrgName = &line.OrgName
		}
		if err := fn(m); err != nil {
			return count, err
		}
		count++
	}
	return count, sc.Err()
}