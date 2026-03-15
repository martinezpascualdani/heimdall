package importsvc

import (
	"bytes"
	"compress/gzip"
	"testing"

	"github.com/google/uuid"
	"github.com/martinezpascualdani/heimdall/internal/routing-service/domain"
)

func TestParsePfx2asStream_SingleAndMulti(t *testing.T) {
	input := "1.0.0.0\t24\t13335\n2.0.0.0\t16\t10_20_30\n2001:db8::\t32\t65536\n"
	var buf bytes.Buffer
	w := gzip.NewWriter(&buf)
	w.Write([]byte(input))
	w.Close()

	datasetID := uuid.New()
	var types []string
	_, err := ParsePfx2asStream(&buf, datasetID, "caida_pfx2as_ipv4", "ipv4", func(o *domain.BGPPrefixOrigin) error {
		types = append(types, o.ASNType)
		return nil
	})
	if err != nil {
		t.Fatalf("ParsePfx2asStream: %v", err)
	}
	if len(types) != 3 {
		t.Fatalf("expected 3 records, got %d", len(types))
	}
	if types[0] != "single" {
		t.Errorf("first line (13335) expected asn_type single, got %q", types[0])
	}
	if types[1] != "multi" {
		t.Errorf("second line (10_20_30) expected asn_type multi, got %q", types[1])
	}
	if types[2] != "single" {
		t.Errorf("third line expected asn_type single, got %q", types[2])
	}
}

func TestParsePfx2asStream_SingleLine(t *testing.T) {
	input := "10.0.0.0\t8\t12345\n"
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	gz.Write([]byte(input))
	gz.Close()

	datasetID := uuid.New()
	var last *domain.BGPPrefixOrigin
	count, err := ParsePfx2asStream(&buf, datasetID, "caida_pfx2as_ipv4", "ipv4", func(o *domain.BGPPrefixOrigin) error {
		last = o
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 || last == nil {
		t.Fatalf("expected 1 record, got count=%d", count)
	}
	if last.Prefix != "10.0.0.0/8" || last.ASNRaw != "12345" || last.ASNType != "single" {
		t.Errorf("got prefix=%q asn_raw=%q asn_type=%q", last.Prefix, last.ASNRaw, last.ASNType)
	}
	if last.PrimaryASN == nil || *last.PrimaryASN != 12345 {
		t.Errorf("expected primary_asn 12345, got %v", last.PrimaryASN)
	}
}

func TestParseASOrgStream_CAIDAFormat(t *testing.T) {
	// CAIDA as-org2info: type "ASN", asn as string, organizationId (not org_id)
	lines := `{"type":"Organization","organizationId":"X-ARIN","name":"Foo Inc.","country":"US"}
{"type":"ASN","asn":"15169","name":"GOOGLE","organizationId":"GOGL-2-ARIN","source":"ARIN"}
{"type":"ASN","asn":"12345","name":"Example AS","organizationId":"EX-ARIN","source":"ARIN"}
`
	var buf bytes.Buffer
	w := gzip.NewWriter(&buf)
	w.Write([]byte(lines))
	w.Close()

	datasetID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	var count int64
	var asns []int64
	_, err := ParseASOrgStream(&buf, "caida_as_org", &datasetID, func(m *domain.ASNMetadata) error {
		count++
		asns = append(asns, m.ASN)
		return nil
	})
	if err != nil {
		t.Fatalf("ParseASOrgStream: %v", err)
	}
	if count != 2 {
		t.Fatalf("expected 2 ASN records (skip Organization), got %d", count)
	}
	if asns[0] != 15169 || asns[1] != 12345 {
		t.Errorf("expected ASNs 15169, 12345, got %v", asns)
	}
}
