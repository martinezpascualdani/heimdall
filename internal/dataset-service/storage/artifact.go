package storage

import (
	"io"
	"os"
	"path/filepath"

	"github.com/google/uuid"
)

// ArtifactStore saves and reads artifact files by version ID.
type ArtifactStore struct {
	BaseDir string
}

// NewArtifactStore creates a store under baseDir (e.g. /data/datasets).
func NewArtifactStore(baseDir string) (*ArtifactStore, error) {
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		return nil, err
	}
	return &ArtifactStore{BaseDir: baseDir}, nil
}

// Save writes the artifact for the given version ID, source, and artifact name. Returns storage path and bytes written.
// source is used as subdirectory (e.g. ripencc, caida_pfx2as_ipv4); artifactName is the file name from the adapter.
func (a *ArtifactStore) Save(versionID uuid.UUID, source string, artifactName string, r io.Reader) (path string, written int64, err error) {
	sub := filepath.Join(a.BaseDir, source)
	if err := os.MkdirAll(sub, 0755); err != nil {
		return "", 0, err
	}
	fullPath := filepath.Join(sub, artifactName)
	f, err := os.Create(fullPath)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()
	written, err = io.Copy(f, r)
	if err != nil {
		os.Remove(fullPath)
		return "", 0, err
	}
	rel, _ := filepath.Rel(a.BaseDir, fullPath)
	if rel == "" || rel == "." {
		rel = filepath.Join(source, artifactName)
	}
	return rel, written, nil
}

// Open returns a ReadCloser for the artifact at storagePath (relative to BaseDir or absolute).
func (a *ArtifactStore) Open(storagePath string) (io.ReadCloser, error) {
	p := storagePath
	if !filepath.IsAbs(p) {
		p = filepath.Join(a.BaseDir, p)
	}
	return os.Open(p)
}
