// Package caida provides adapters to fetch CAIDA datasets: RouteViews Prefix-to-AS (pfx2as) IPv4/IPv6 and AS Organizations.
// See https://www.caida.org/catalog/datasets/routeviews-prefix2as/ and CAIDA AUA for terms of use.
package caida

import (
	"bufio"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"net/http"
	"path"
	"strings"
	"time"
)

const (
	// DefaultPfx2asIPv4Base is the base URL for RouteViews IPv4 prefix2as (no trailing slash).
	DefaultPfx2asIPv4Base = "https://publicdata.caida.org/datasets/routing/routeviews-prefix2as"
	// DefaultPfx2asIPv6Base is the base URL for RouteViews IPv6 prefix2as (no trailing slash).
	DefaultPfx2asIPv6Base = "https://publicdata.caida.org/datasets/routing/routeviews6-prefix2as"
	// DefaultASOrgLatestURL is the default URL for the latest AS org snapshot (jsonl).
	DefaultASOrgLatestURL = "https://publicdata.caida.org/datasets/as-organizations/latest.as-org2info.jsonl.gz"
)

// Pfx2asFetchResult is the result of a pfx2as fetch: body to read, source_version and artifact name for storage.
type Pfx2asFetchResult struct {
	Body         io.ReadCloser
	SourceVersion string // e.g. routeviews-rv2-20260313-1200.pfx2as.gz
	ArtifactName string // same as SourceVersion for storage filename
}

// ASOrgFetchResult is the result of an AS org fetch.
type ASOrgFetchResult struct {
	Body         io.ReadCloser
	SourceVersion string // derived from URL or Last-Modified
	ArtifactName string
}

// CreationLogLine is one non-comment line from pfx2as-creation.log (seqnum, timestamp, path).
func parseCreationLogLastLine(r io.Reader) (path string, err error) {
	sc := bufio.NewScanner(r)
	var last string
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		last = line
	}
	if err := sc.Err(); err != nil {
		return "", err
	}
	if last == "" {
		return "", fmt.Errorf("no data line in creation log")
	}
	// Format: seqnum timestamp path (space-separated)
	parts := strings.Fields(last)
	if len(parts) < 3 {
		return "", fmt.Errorf("invalid creation log line: %s", last)
	}
	return parts[2], nil
}

// FetchPfx2asCreationLog GETs the creation log and returns the path (e.g. 2026/03/routeviews-rv2-20260313-1200.pfx2as.gz) of the latest file.
func FetchPfx2asCreationLog(ctx context.Context, baseURL string, client *http.Client) (filePath string, err error) {
	if client == nil {
		client = &http.Client{Timeout: 60 * time.Second}
	}
	url := strings.TrimSuffix(baseURL, "/") + "/pfx2as-creation.log"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("pfx2as-creation.log: %s", resp.Status)
	}
	return parseCreationLogLastLine(resp.Body)
}

// FetchPfx2asLatest discovers the latest file from the creation log and downloads it. Returns body (gzip), source_version (basename of path), artifactName.
func FetchPfx2asLatest(ctx context.Context, baseURL string, client *http.Client) (*Pfx2asFetchResult, error) {
	if client == nil {
		client = &http.Client{Timeout: 60 * time.Second}
	}
	filePath, err := FetchPfx2asCreationLog(ctx, baseURL, client)
	if err != nil {
		return nil, err
	}
	// filePath is e.g. 2026/03/routeviews-rv2-20260313-1200.pfx2as.gz
	downloadURL := strings.TrimSuffix(baseURL, "/") + "/" + filePath
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("pfx2as download: %s", resp.Status)
	}
	artifactName := path.Base(filePath) // e.g. routeviews-rv2-20260313-1200.pfx2as.gz
	return &Pfx2asFetchResult{
		Body:          resp.Body,
		SourceVersion: artifactName,
		ArtifactName:  artifactName,
	}, nil
}

// FetchASOrgLatest GETs the AS org snapshot from the given URL. source_version is derived from the URL path (filename) or, if not possible, from Last-Modified header.
func FetchASOrgLatest(ctx context.Context, url string, client *http.Client) (*ASOrgFetchResult, error) {
	if url == "" {
		url = DefaultASOrgLatestURL
	}
	if client == nil {
		client = &http.Client{Timeout: 120 * time.Second}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("as-org download: %s", resp.Status)
	}
	// Derive source_version: do not store literal "latest". Use filename or Last-Modified.
	artifactName := path.Base(strings.TrimSuffix(url, "?"))
	if idx := strings.Index(artifactName, "?"); idx > 0 {
		artifactName = artifactName[:idx]
	}
	sourceVersion := artifactName
	if strings.Contains(strings.ToLower(artifactName), "latest") {
		// URL was a "latest" pointer: use Last-Modified or system time so we never store "latest"
		if lm := resp.Header.Get("Last-Modified"); lm != "" {
			if t, err := time.Parse(time.RFC1123, lm); err == nil {
				sourceVersion = "as-org-" + t.UTC().Format("20060102150405")
			} else {
				sourceVersion = "as-org-" + time.Now().UTC().Format("20060102150405")
			}
		} else {
			sourceVersion = "as-org-" + time.Now().UTC().Format("20060102150405")
		}
	}
	return &ASOrgFetchResult{
		Body:          resp.Body,
		SourceVersion: sourceVersion,
		ArtifactName:  artifactName,
	}, nil
}

// ValidatePfx2asGzip reads the first few lines of the gzip stream to ensure it looks like tab-separated prefix, prefix_len, AS.
func ValidatePfx2asGzip(r io.Reader) error {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return err
	}
	defer gz.Close()
	sc := bufio.NewScanner(gz)
	sc.Buffer(nil, 256*1024)
	var line string
	for sc.Scan() {
		line = sc.Text()
		if strings.TrimSpace(line) == "" || strings.HasPrefix(line, "#") {
			continue
		}
		break
	}
	if line == "" {
		return fmt.Errorf("no data line in pfx2as gzip")
	}
	parts := strings.Split(line, "\t")
	if len(parts) < 3 {
		return fmt.Errorf("pfx2as line expected at least 3 tab-separated fields, got %d", len(parts))
	}
	return nil
}
