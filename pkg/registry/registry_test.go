package registry

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestConfigFor_KnownRegistries(t *testing.T) {
	tests := []struct {
		name       string
		registry   string
		wantURL    string
		wantChecksum bool // RIPE has checksum URL
	}{
		{"ripencc", "ripencc", "https://ftp.ripe.net/pub/stats/ripencc/delegated-ripencc-latest", true},
		{"arin", "arin", "https://ftp.arin.net/pub/stats/arin/delegated-arin-extended-latest", false},
		{"apnic", "apnic", "https://ftp.apnic.net/pub/stats/apnic/delegated-apnic-latest", false},
		{"lacnic", "lacnic", "https://ftp.lacnic.net/pub/stats/lacnic/delegated-lacnic-latest", false},
		{"afrinic", "afrinic", "https://ftp.afrinic.net/pub/stats/afrinic/delegated-afrinic-latest", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := ConfigFor(tt.registry)
			if cfg.DatasetURL != tt.wantURL {
				t.Errorf("ConfigFor(%q) DatasetURL = %q, want %q", tt.registry, cfg.DatasetURL, tt.wantURL)
			}
			if tt.wantChecksum && cfg.ChecksumURL == "" {
				t.Errorf("ConfigFor(%q) should have ChecksumURL", tt.registry)
			}
			if cfg.Timeout != 10*time.Minute {
				t.Errorf("Timeout = %v", cfg.Timeout)
			}
		})
	}
}

func TestConfigFor_UnknownOrEmpty_ReturnsRIPEDefault(t *testing.T) {
	cfgEmpty := ConfigFor("")
	cfgUnknown := ConfigFor("unknown")
	cfgRipe := DefaultRIPENCCConfig()
	if cfgEmpty.DatasetURL != cfgRipe.DatasetURL {
		t.Errorf("ConfigFor(\"\") should return RIPE default, got %q", cfgEmpty.DatasetURL)
	}
	if cfgUnknown.DatasetURL != cfgRipe.DatasetURL {
		t.Errorf("ConfigFor(\"unknown\") should return RIPE default, got %q", cfgUnknown.DatasetURL)
	}
}

func TestHTTPFetcher_Fetch_Success(t *testing.T) {
	body := "2|ripencc|123|0|20240101|20240102|+00\n"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(body))
	}))
	defer srv.Close()

	fetcher := NewHTTPFetcher(5 * time.Second)
	cfg := Config{DatasetURL: srv.URL, Timeout: 5 * time.Second}
	ctx := context.Background()

	rc, size, err := fetcher.Fetch(ctx, cfg)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	defer rc.Close()
	if size != int64(len(body)) {
		t.Errorf("size = %d, want %d", size, len(body))
	}
	b, _ := io.ReadAll(rc)
	if string(b) != body {
		t.Errorf("body = %q", string(b))
	}
}

func TestHTTPFetcher_Fetch_404(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	fetcher := NewHTTPFetcher(5 * time.Second)
	cfg := Config{DatasetURL: srv.URL}
	_, _, err := fetcher.Fetch(context.Background(), cfg)
	if err == nil {
		t.Fatal("expected error on 404")
	}
	if !strings.Contains(err.Error(), "404") {
		t.Errorf("error should mention 404: %v", err)
	}
}

func TestHTTPFetcher_NewHTTPFetcher_ZeroTimeout(t *testing.T) {
	f := NewHTTPFetcher(0)
	if f.Client == nil || f.Client.Timeout != 5*time.Minute {
		t.Errorf("zero timeout should default to 5m: %v", f.Client.Timeout)
	}
}
