package source

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/chinmay28/photoframe/internal/config"
	"github.com/chinmay28/photoframe/internal/library"
)

// Factory builds sources, storing uploads and downloads under DataDir.
type Factory struct {
	DataDir string
	Client  *http.Client
}

// UploadDir is where photos uploaded to an upload source are stored.
func (f Factory) UploadDir(sourceID string) string {
	return filepath.Join(f.DataDir, "uploads", sourceID)
}

// CacheDir is where a remote source keeps its downloads.
func (f Factory) CacheDir(sourceID string) string {
	return filepath.Join(f.DataDir, "cache", sourceID)
}

// New implements library.Factory.
func (f Factory) New(c config.Source) (library.Source, error) {
	client := f.Client
	if client == nil {
		client = &http.Client{Timeout: 60 * time.Second}
	}
	switch c.Type {
	case config.SourceUpload:
		dir := f.UploadDir(c.ID)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, err
		}
		return &Dir{SourceID: c.ID, Root: dir}, nil
	case config.SourceFolder:
		return &Dir{SourceID: c.ID, Root: c.Path}, nil
	case config.SourceImmich:
		return &Immich{SourceID: c.ID, BaseURL: c.URL, APIKey: c.APIKey, AlbumID: c.AlbumID,
			CacheDir: f.CacheDir(c.ID), Client: client}, nil
	case config.SourceURL:
		return &URLImage{SourceID: c.ID, URL: c.URL, CacheDir: f.CacheDir(c.ID), Client: client}, nil
	}
	return nil, fmt.Errorf("unknown source type %q", c.Type)
}
