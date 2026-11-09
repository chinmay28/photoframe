package source

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image"
	_ "image/gif" // decoders for validating downloads
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/chinmay28/photoframe/internal/config"
	"github.com/chinmay28/photoframe/internal/library"
	_ "golang.org/x/image/webp"
)

// URLImage fetches a single image URL (for example an "art of the day" feed
// that always serves the current picture). Each new image replaces the
// previous one; conditional requests avoid re-downloading unchanged images.
type URLImage struct {
	SourceID string
	URL      string
	CacheDir string
	Client   *http.Client
	Now      func() time.Time
}

type urlMeta struct {
	File         string    `json:"file"`
	ETag         string    `json:"etag,omitempty"`
	LastModified string    `json:"lastModified,omitempty"`
	FetchedAt    time.Time `json:"fetchedAt"`
}

const urlMetaFile = "meta.json"

// Sync implements library.Source.
func (u *URLImage) Sync(ctx context.Context) ([]library.Photo, error) {
	if err := os.MkdirAll(u.CacheDir, 0o755); err != nil {
		return nil, err
	}
	meta := u.loadMeta()
	next, err := u.fetch(ctx, meta)
	if err != nil {
		return u.photos(meta), err
	}
	return u.photos(next), nil
}

func (u *URLImage) fetch(ctx context.Context, meta *urlMeta) (*urlMeta, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.URL, nil)
	if err != nil {
		return nil, err
	}
	if meta != nil {
		if meta.ETag != "" {
			req.Header.Set("If-None-Match", meta.ETag)
		}
		if meta.LastModified != "" {
			req.Header.Set("If-Modified-Since", meta.LastModified)
		}
	}
	client := u.Client
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, unreachable(u.URL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotModified && meta != nil {
		return meta, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch %s: %s", u.URL, resp.Status)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, maxDownload+1))
	if err != nil {
		return nil, err
	}
	if len(b) > maxDownload {
		return nil, fmt.Errorf("fetch %s: image larger than %d MB", u.URL, maxDownload>>20)
	}
	if _, _, err := image.DecodeConfig(bytes.NewReader(b)); err != nil {
		return nil, fmt.Errorf("fetch %s: not a supported image: %w", u.URL, err)
	}

	sum := sha1.Sum(b)
	next := &urlMeta{
		File:         hex.EncodeToString(sum[:8]) + ".img",
		ETag:         resp.Header.Get("ETag"),
		LastModified: resp.Header.Get("Last-Modified"),
		FetchedAt:    u.now(),
	}
	if meta != nil && meta.File == next.File {
		next.FetchedAt = meta.FetchedAt // same picture; keep its date
	}
	if err := config.WriteFileAtomic(filepath.Join(u.CacheDir, next.File), b, 0o644); err != nil {
		return nil, err
	}
	mb, _ := json.Marshal(next)
	if err := config.WriteFileAtomic(filepath.Join(u.CacheDir, urlMetaFile), mb, 0o644); err != nil {
		return nil, err
	}
	pruneDir(u.CacheDir, map[string]bool{next.File: true, urlMetaFile: true})
	return next, nil
}

func (u *URLImage) photos(m *urlMeta) []library.Photo {
	if m == nil {
		return nil
	}
	p := filepath.Join(u.CacheDir, m.File)
	if _, err := os.Stat(p); err != nil {
		return nil
	}
	return []library.Photo{{
		ID: library.PhotoID(u.SourceID, m.File), SourceID: u.SourceID,
		Path: p, TakenAt: m.FetchedAt,
	}}
}

func (u *URLImage) loadMeta() *urlMeta {
	b, err := os.ReadFile(filepath.Join(u.CacheDir, urlMetaFile))
	if err != nil {
		return nil
	}
	var m urlMeta
	if json.Unmarshal(b, &m) != nil || m.File == "" {
		return nil
	}
	return &m
}

func (u *URLImage) now() time.Time {
	if u.Now != nil {
		return u.Now()
	}
	return time.Now()
}
