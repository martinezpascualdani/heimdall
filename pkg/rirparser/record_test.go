package rirparser

import (
	"testing"
)

func TestParseRecord_ValidIPv4(t *testing.T) {
	line := "ripencc|ES|ipv4|1.2.3.0|256|20240101|allocated"
	rec, err := ParseRecord(line)
	if err != nil {
		t.Fatalf("ParseRecord: %v", err)
	}
	if rec == nil {
		t.Fatal("expected record")
	}
	if rec.Registry != "ripencc" || rec.CC != "ES" || rec.Type != TypeIPv4 {
		t.Errorf("registry/cc/type: %q %q %q", rec.Registry, rec.CC, rec.Type)
	}
	if rec.Start != "1.2.3.0" || rec.Value != "256" || rec.Date != "20240101" || rec.Status != "allocated" {
		t.Errorf("start/value/date/status: %q %q %q %q", rec.Start, rec.Value, rec.Date, rec.Status)
	}
}

func TestParseRecord_ValidIPv6(t *testing.T) {
	line := "apnic|AU|ipv6|2001:db8::|32|20240101|allocated"
	rec, err := ParseRecord(line)
	if err != nil {
		t.Fatalf("ParseRecord: %v", err)
	}
	if rec == nil {
		t.Fatal("expected record")
	}
	if rec.Type != TypeIPv6 || rec.Start != "2001:db8::" || rec.Value != "32" {
		t.Errorf("unexpected: %+v", rec)
	}
}

func TestParseRecord_ASN(t *testing.T) {
	line := "ripencc|DE|asn|12345|1|20240101|allocated"
	rec, err := ParseRecord(line)
	if err != nil {
		t.Fatalf("ParseRecord: %v", err)
	}
	if rec == nil {
		t.Fatal("expected record (caller filters asn)")
	}
	if rec.Type != TypeASN || rec.CC != "DE" {
		t.Errorf("unexpected: %+v", rec)
	}
}

func TestParseRecord_SummaryLineReturnsNil(t *testing.T) {
	line := "ripencc|*|summary|0|0|20240101|summary"
	rec, err := ParseRecord(line)
	if err != nil {
		t.Fatalf("ParseRecord: %v", err)
	}
	if rec != nil {
		t.Errorf("summary line should return nil record, got %+v", rec)
	}
}

func TestParseRecord_CommentReturnsNil(t *testing.T) {
	rec, err := ParseRecord("# comment")
	if err != nil {
		t.Fatalf("ParseRecord: %v", err)
	}
	if rec != nil {
		t.Errorf("comment should return nil, got %+v", rec)
	}
}

func TestParseRecord_BlankReturnsNil(t *testing.T) {
	rec, err := ParseRecord("")
	if err != nil {
		t.Fatalf("ParseRecord: %v", err)
	}
	if rec != nil {
		t.Errorf("blank should return nil, got %+v", rec)
	}
}

func TestParseRecord_TooFewFieldsError(t *testing.T) {
	line := "ripencc|ES|ipv4|1.2.3.0"
	_, err := ParseRecord(line)
	if err == nil {
		t.Fatal("expected error for too few fields")
	}
}

func TestParseRecord_EmptyCC(t *testing.T) {
	// 6 fields: no status; cc can be empty in theory
	line := "ripencc||ipv4|1.2.3.0|256|20240101"
	rec, err := ParseRecord(line)
	if err != nil {
		t.Fatalf("ParseRecord: %v", err)
	}
	if rec == nil {
		t.Fatal("expected record (caller filters empty cc)")
	}
	if rec.CC != "" {
		t.Errorf("expected empty cc, got %q", rec.CC)
	}
	if rec.Status != "" {
		t.Errorf("expected empty status when 6 fields, got %q", rec.Status)
	}
}

func TestParseRecord_StatusOptional(t *testing.T) {
	line := "ripencc|ES|ipv4|1.2.3.0|256|20240101"
	rec, err := ParseRecord(line)
	if err != nil {
		t.Fatalf("ParseRecord: %v", err)
	}
	if rec == nil {
		t.Fatal("expected record")
	}
	if rec.Status != "" {
		t.Errorf("status with 6 fields should be empty, got %q", rec.Status)
	}
}

func TestParseRecord_StatusRaro(t *testing.T) {
	line := "ripencc|ES|ipv4|1.2.3.0|256|20240101|reserved"
	rec, err := ParseRecord(line)
	if err != nil {
		t.Fatalf("ParseRecord: %v", err)
	}
	if rec == nil {
		t.Fatal("expected record")
	}
	if rec.Status != "reserved" {
		t.Errorf("status should be preserved: %q", rec.Status)
	}
}

func TestBlockRawIdentity(t *testing.T) {
	rec := &Record{Registry: "ripencc", CC: "ES", Type: TypeIPv4, Start: "1.2.3.0", Value: "256", Date: "20240101", Status: "allocated"}
	id := rec.BlockRawIdentity()
	expect := "ripencc|ES|ipv4|1.2.3.0|256|20240101|allocated"
	if id != expect {
		t.Errorf("BlockRawIdentity: got %q, want %q", id, expect)
	}
}
