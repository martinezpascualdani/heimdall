package rirparser

import (
	"testing"
)

func TestParseHeader_Valid(t *testing.T) {
	line := "2|ripencc|1773529199|125000|20240101|20240102|+00"
	h, err := ParseHeader(line)
	if err != nil {
		t.Fatalf("ParseHeader: %v", err)
	}
	if h.Version != 2 || h.Registry != "ripencc" || h.Serial != 1773529199 || h.Records != 125000 {
		t.Errorf("unexpected header: %+v", h)
	}
	if h.StartDate != "20240101" || h.EndDate != "20240102" || h.UTCOffset != "+00" {
		t.Errorf("unexpected dates/offset: %q %q %q", h.StartDate, h.EndDate, h.UTCOffset)
	}
}

func TestParseHeader_VersionWithDot(t *testing.T) {
	// ARIN uses "2.3" etc.
	line := "2.3|arin|1773493250801|80000|20240101|20240102|+00"
	h, err := ParseHeader(line)
	if err != nil {
		t.Fatalf("ParseHeader: %v", err)
	}
	if h.Version != 2 {
		t.Errorf("expected version 2 (integer part of 2.3), got %d", h.Version)
	}
	if h.Registry != "arin" || h.Serial != 1773493250801 {
		t.Errorf("unexpected header: %+v", h)
	}
}

func TestParseHeader_InvalidTooFewFields(t *testing.T) {
	line := "2|ripencc|1773529199"
	_, err := ParseHeader(line)
	if err == nil {
		t.Fatal("expected error for too few fields")
	}
	if err.Error() == "" {
		t.Error("error message should mention fields")
	}
}

func TestParseHeader_InvalidVersion(t *testing.T) {
	line := "abc|ripencc|1773529199|125000|20240101|20240102|+00"
	_, err := ParseHeader(line)
	if err == nil {
		t.Fatal("expected error for invalid version")
	}
}

func TestParseHeader_InvalidSerial(t *testing.T) {
	line := "2|ripencc|notanumber|125000|20240101|20240102|+00"
	_, err := ParseHeader(line)
	if err == nil {
		t.Fatal("expected error for invalid serial")
	}
}

func TestParseHeader_WhitespaceTrimmed(t *testing.T) {
	line := "  2  |  ripencc  |  1773529199  |  125000  |  20240101  |  20240102  |  +00  "
	h, err := ParseHeader(line)
	if err != nil {
		t.Fatalf("ParseHeader: %v", err)
	}
	if h.Version != 2 || h.Registry != "ripencc" || h.Serial != 1773529199 {
		t.Errorf("whitespace should be trimmed: %+v", h)
	}
}

func TestValidRegistry(t *testing.T) {
	valid := []string{"afrinic", "apnic", "arin", "iana", "lacnic", "ripencc", "RIPENCC", "ARIN"}
	for _, r := range valid {
		if !ValidRegistry(r) {
			t.Errorf("ValidRegistry(%q) should be true", r)
		}
	}
	invalid := []string{"", "unknown", "ripe", "arinx"}
	for _, r := range invalid {
		if ValidRegistry(r) {
			t.Errorf("ValidRegistry(%q) should be false", r)
		}
	}
}
