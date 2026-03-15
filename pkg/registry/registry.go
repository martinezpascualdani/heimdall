package registry

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Registry identifies an RIR.
type Registry string

const (
	RIPENCC  Registry = "ripencc"
	APNIC    Registry = "apnic"
	ARIN     Registry = "arin"
	AFRINIC  Registry = "afrinic"
	LACNIC   Registry = "lacnic"
	IANA     Registry = "iana"
)

// Config holds per-registry configuration for fetching delegated stats.
type Config struct {
	DatasetURL   string        // e.g. https://ftp.ripe.net/pub/stats/ripencc/delegated-ripencc-latest
	ChecksumURL  string        // optional MD5 file URL
	Timeout      time.Duration // download timeout
	MaxSizeBytes int64         // max artifact size (0 = no limit)
	Headers      map[string]string
}

// DefaultRIPENCCConfig returns config for RIPE NCC.
func DefaultRIPENCCConfig() Config {
	return Config{
		DatasetURL:   "https://ftp.ripe.net/pub/stats/ripencc/delegated-ripencc-latest",
		ChecksumURL:  "https://ftp.ripe.net/pub/stats/ripencc/delegated-ripencc-latest.md5",
		Timeout:      10 * time.Minute,
		MaxSizeBytes: 500 * 1024 * 1024, // 500 MiB
		Headers:      nil,
	}
}

// ConfigFor returns the fetch config for the given registry name (e.g. "ripencc", "arin", "apnic", "lacnic", "afrinic").
// Returns DefaultRIPENCCConfig for unknown/empty (backward compatible).
func ConfigFor(registryName string) Config {
	switch Registry(registryName) {
	case RIPENCC:
		return DefaultRIPENCCConfig()
	case ARIN:
		return Config{
			DatasetURL:   "https://ftp.arin.net/pub/stats/arin/delegated-arin-extended-latest",
			ChecksumURL:  "",
			Timeout:      10 * time.Minute,
			MaxSizeBytes: 500 * 1024 * 1024,
			Headers:      nil,
		}
	case APNIC:
		return Config{
			DatasetURL:   "https://ftp.apnic.net/pub/stats/apnic/delegated-apnic-latest",
			ChecksumURL:  "",
			Timeout:      10 * time.Minute,
			MaxSizeBytes: 500 * 1024 * 1024,
			Headers:      nil,
		}
	case LACNIC:
		return Config{
			DatasetURL:   "https://ftp.lacnic.net/pub/stats/lacnic/delegated-lacnic-latest",
			ChecksumURL:  "",
			Timeout:      10 * time.Minute,
			MaxSizeBytes: 500 * 1024 * 1024,
			Headers:      nil,
		}
	case AFRINIC:
		return Config{
			DatasetURL:   "https://ftp.afrinic.net/pub/stats/afrinic/delegated-afrinic-latest",
			ChecksumURL:  "",
			Timeout:      10 * time.Minute,
			MaxSizeBytes: 500 * 1024 * 1024,
			Headers:      nil,
		}
	default:
		return DefaultRIPENCCConfig()
	}
}

// Fetcher fetches the delegated stats artifact for a registry.
type Fetcher interface {
	Fetch(ctx context.Context, cfg Config) (io.ReadCloser, int64, error)
}

// HTTPFetcher uses http.Client to fetch artifacts.
type HTTPFetcher struct {
	Client *http.Client
}

// NewHTTPFetcher creates an HTTPFetcher with optional timeout.
func NewHTTPFetcher(timeout time.Duration) *HTTPFetcher {
	if timeout == 0 {
		timeout = 5 * time.Minute
	}
	return &HTTPFetcher{
		Client: &http.Client{Timeout: timeout},
	}
}

// Fetch downloads the dataset. Returns the body (caller must Close), and size if known (-1 otherwise).
// Respects cfg.Timeout and cfg.MaxSizeBytes (returns error if exceeded).
func (h *HTTPFetcher) Fetch(ctx context.Context, cfg Config) (io.ReadCloser, int64, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, cfg.DatasetURL, nil)
	if err != nil {
		return nil, -1, err
	}
	for k, v := range cfg.Headers {
		req.Header.Set(k, v)
	}
	resp, err := h.Client.Do(req)
	if err != nil {
		return nil, -1, err
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, -1, fmt.Errorf("fetch %s: status %d", cfg.DatasetURL, resp.StatusCode)
	}
	size := resp.ContentLength
	if cfg.MaxSizeBytes > 0 && size >= 0 && size > cfg.MaxSizeBytes {
		resp.Body.Close()
		return nil, -1, fmt.Errorf("artifact size %d exceeds limit %d", size, cfg.MaxSizeBytes)
	}
	var body io.ReadCloser = resp.Body
	if cfg.MaxSizeBytes > 0 && size < 0 {
		body = &maxReader{R: resp.Body, N: cfg.MaxSizeBytes}
	}
	return body, size, nil
}

// maxReader limits reads to N bytes.
type maxReader struct {
	R io.ReadCloser
	N int64
}

func (m *maxReader) Read(p []byte) (n int, err error) {
	if m.N <= 0 {
		return 0, fmt.Errorf("max size exceeded")
	}
	if int64(len(p)) > m.N {
		p = p[:m.N]
	}
	n, err = m.R.Read(p)
	m.N -= int64(n)
	return n, err
}

func (m *maxReader) Close() error {
	return m.R.Close()
}
