package iso3166

import "testing"

func TestValidAlpha2(t *testing.T) {
	for _, cc := range []string{"ES", "es", "US", "FR", "DE", "GB", "AQ"} {
		if !ValidAlpha2(cc) {
			t.Errorf("expected valid: %q", cc)
		}
	}
	invalid := []string{"", "A", "ABC", "e1", "123", "QQ", "ZZ"}
	for _, cc := range invalid {
		if ValidAlpha2(cc) {
			t.Errorf("expected invalid: %q", cc)
		}
	}
}

func TestValidAlpha2_Normalization(t *testing.T) {
	if !ValidAlpha2("es") {
		t.Error("es should be valid (normalized to ES)")
	}
	if !ValidAlpha2("Es") {
		t.Error("Es should be valid")
	}
}

func TestValidAlpha2_InvalidCodes(t *testing.T) {
	// Plan: ZZ, QQ → no existen en ISO 3166-1 alpha-2, inválido
	if ValidAlpha2("QQ") {
		t.Error("QQ should be invalid")
	}
	if ValidAlpha2("ZZ") {
		t.Error("ZZ should be invalid (often reserved/not assigned)")
	}
}
