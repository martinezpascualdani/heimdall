package rirparser

import (
	"strings"
	"testing"
)

func TestParseStream_ValidMultiLine(t *testing.T) {
	input := "2|ripencc|1773529199|3|20240101|20240102|+00\n" +
		"ripencc|ES|ipv4|1.2.3.0|256|20240101|allocated\n" +
		"ripencc|ES|ipv6|2001:db8::|32|20240101|allocated\n" +
		"ripencc|DE|asn|12345|1|20240101|allocated\n"
	result, err := ParseStream(strings.NewReader(input))
	if err != nil {
		t.Fatalf("ParseStream: %v", err)
	}
	if result.Header == nil || result.Header.Registry != "ripencc" || result.Header.Serial != 1773529199 {
		t.Errorf("unexpected header: %+v", result.Header)
	}
	var records []*Record
	for rec := range result.Records {
		records = append(records, rec)
	}
	// drain Err in case no error
	go func() {
		for range result.Err {
		}
	}()
	if len(records) != 3 {
		t.Fatalf("expected 3 records (ipv4, ipv6, asn), got %d", len(records))
	}
	if records[0].Type != TypeIPv4 || records[0].CC != "ES" {
		t.Errorf("first record: %+v", records[0])
	}
	if records[1].Type != TypeIPv6 {
		t.Errorf("second record: %+v", records[1])
	}
	if records[2].Type != TypeASN || records[2].CC != "DE" {
		t.Errorf("third record: %+v", records[2])
	}
}

func TestParseStream_SummaryLineSkipped(t *testing.T) {
	input := "2|ripencc|1|2|20240101|20240102|+00\n" +
		"ripencc|*|summary|0|0|20240101|summary\n" +
		"ripencc|ES|ipv4|1.0.0.0|256|20240101|allocated\n"
	result, err := ParseStream(strings.NewReader(input))
	if err != nil {
		t.Fatalf("ParseStream: %v", err)
	}
	var records []*Record
	for rec := range result.Records {
		records = append(records, rec)
	}
	for range result.Err {
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 data record (summary skipped), got %d", len(records))
	}
	if records[0].Type != TypeIPv4 || records[0].Start != "1.0.0.0" {
		t.Errorf("unexpected: %+v", records[0])
	}
}

func TestParseStream_CommentAndBlankSkippedBeforeHeader(t *testing.T) {
	input := "# comment\n\n   \n2|apnic|20260315|100|20240101|20240102|+00\n" +
		"apnic|AU|ipv4|1.0.0.0|256|20240101|allocated\n"
	result, err := ParseStream(strings.NewReader(input))
	if err != nil {
		t.Fatalf("ParseStream: %v", err)
	}
	if result.Header.Registry != "apnic" || result.Header.Serial != 20260315 {
		t.Errorf("header after comments: %+v", result.Header)
	}
	var n int
	for range result.Records {
		n++
	}
	for range result.Err {
	}
	if n != 1 {
		t.Fatalf("expected 1 record, got %d", n)
	}
}

func TestParseStream_NoVersionLine(t *testing.T) {
	input := "# only comments\n# here\n"
	_, err := ParseStream(strings.NewReader(input))
	if err == nil {
		t.Fatal("expected error when no version line")
	}
	if !strings.Contains(err.Error(), "version") {
		t.Errorf("error should mention version: %v", err)
	}
}

func TestParseStream_InvalidHeader(t *testing.T) {
	input := "not|enough|fields\n"
	_, err := ParseStream(strings.NewReader(input))
	if err == nil {
		t.Fatal("expected error for invalid header")
	}
}

func TestParseStream_CorruptLineInStream(t *testing.T) {
	input := "2|ripencc|1|2|20240101|20240102|+00\n" +
		"ripencc|ES|ipv4|1.0.0.0|256|20240101|allocated\n" +
		"corrupt|too|few\n" +
		"ripencc|US|ipv4|2.0.0.0|256|20240101|allocated\n"
	result, err := ParseStream(strings.NewReader(input))
	if err != nil {
		t.Fatalf("ParseStream: %v", err)
	}
	var records []*Record
	var streamErr error
	for rec := range result.Records {
		records = append(records, rec)
	}
	for e := range result.Err {
		streamErr = e
		break
	}
	if streamErr == nil {
		t.Fatal("expected error from corrupt line")
	}
	// We may have 1 or 2 records before the error (implementation may buffer)
	if len(records) < 1 {
		t.Errorf("expected at least 1 record before error, got %d", len(records))
	}
}
