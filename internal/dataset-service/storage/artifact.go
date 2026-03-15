package storage

import (
	"fmt"
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

// Save writes the artifact for the given version ID and returns the storage path.
func (a *ArtifactStore) Save(versionID uuid.UUID, registry string, serial int64, r io.Reader) (path string, written int64, err error) {
	// e.g. baseDir/ripencc/serial-12345-<uuid>.txt
	sub := filepath.Join(a.BaseDir, registry)
	if err := os.MkdirAll(sub, 0755); err != nil {
		return "", 0, err
	}
	name := fmt.Sprintf("delegated-%s-serial-%d-%s.txt", registry, serial, versionID.String()[:8])
	fullPath := filepath.Join(sub, name)
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
		rel = filepath.Join(registry, name)
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
