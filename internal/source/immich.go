package source

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/chinmay28/photoframe/internal/config"
	"github.com/chinmay28/photoframe/internal/library"
)

// maxDownload bounds a single image download.
const maxDownload = 40 << 20

// Immich syncs the images of one Immich album into CacheDir. It downloads
// Immich's "preview" rendition (about 1440 px), which is plenty for an
// 800×480 panel and far lighter than originals.
type Immich struct {
	SourceID string
	BaseURL  string
	APIKey   string
	AlbumID  string
	CacheDir string
	Client   *http.Client
}

type immichAlbum struct {
	Assets []immichAsset `json:"assets"`
}

type immichAsset struct {
	ID               string    `json:"id"`
	Type             string    `json:"type"`
	OriginalFileName string    `json:"originalFileName"`
	FileCreatedAt    time.Time `json:"fileCreatedAt"`
	ExifInfo         *struct {
		DateTimeOriginal *time.Time `json:"dateTimeOriginal"`
		City             string     `json:"city"`
		Country          string     `json:"country"`
	} `json:"exifInfo"`
}

// cachedAsset is what the index file records per downloaded asset.
type cachedAsset struct {
	ID      string    `json:"id"`
	TakenAt time.Time `json:"takenAt"`
	Title   string    `json:"title,omitempty"`
	Name    string    `json:"name,omitempty"`
}

var assetIDPattern = regexp.MustCompile(`^[A-Za-z0-9-]{1,64}$`)

// Sync implements library.Source. When Immich is unreachable it returns the
// previously downloaded photos along with the error.
func (im *Immich) Sync(ctx context.Context) ([]library.Photo, error) {
	if err := os.MkdirAll(im.CacheDir, 0o755); err != nil {
		return nil, err
	}
	album, err := im.album(ctx)
	if err != nil {
		cached, _ := im.loadIndex()
		return im.photos(cached), err
	}

	var assets []cachedAsset
	var errs []error
	keep := map[string]bool{indexFile: true}
	for _, a := range album.Assets {
		if a.Type != "IMAGE" || !assetIDPattern.MatchString(a.ID) {
			continue
		}
		path := im.path(a.ID)
		keep[filepath.Base(path)] = true
		if _, err := os.Stat(path); err != nil {
			if err := im.download(ctx, a.ID, path); err != nil {
				errs = append(errs, err)
				continue
			}
		}
		assets = append(assets, cachedAsset{ID: a.ID, TakenAt: a.takenAt(), Title: a.place(), Name: a.OriginalFileName})
	}
	pruneDir(im.CacheDir, keep)
	if err := im.saveIndex(assets); err != nil {
		errs = append(errs, err)
	}
	if len(errs) > 3 {
		errs = append(errs[:3], fmt.Errorf("and %d more errors", len(errs)-3))
	}
	return im.photos(assets), errors.Join(errs...)
}

func (im *Immich) album(ctx context.Context) (*immichAlbum, error) {
	u := strings.TrimRight(im.BaseURL, "/") + "/api/albums/" + url.PathEscape(im.AlbumID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("x-api-key", im.APIKey)
	req.Header.Set("Accept", "application/json")
	resp, err := im.client().Do(req)
	if err != nil {
		return nil, unreachable(im.BaseURL, err)
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusUnauthorized, http.StatusForbidden:
		return nil, fmt.Errorf("Immich rejected the API key (%s)", resp.Status)
	case http.StatusNotFound, http.StatusBadRequest:
		return nil, fmt.Errorf("album %q not found on this Immich server (%s)", im.AlbumID, resp.Status)
	default:
		return nil, fmt.Errorf("Immich returned %s", resp.Status)
	}
	var a immichAlbum
	if err := json.NewDecoder(io.LimitReader(resp.Body, 64<<20)).Decode(&a); err != nil {
		return nil, fmt.Errorf("immich: decode album: %w", err)
	}
	return &a, nil
}

func (im *Immich) download(ctx context.Context, id, path string) error {
	u := strings.TrimRight(im.BaseURL, "/") + "/api/assets/" + id + "/thumbnail?size=preview"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	req.Header.Set("x-api-key", im.APIKey)
	resp, err := im.client().Do(req)
	if err != nil {
		return unreachable(im.BaseURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("immich: asset %s returned %s", id, resp.Status)
	}
	return saveBody(resp.Body, path)
}

func (im *Immich) photos(assets []cachedAsset) []library.Photo {
	var out []library.Photo
	for _, a := range assets {
		p := im.path(a.ID)
		if _, err := os.Stat(p); err != nil {
			continue
		}
		out = append(out, library.Photo{
			ID: library.PhotoID(im.SourceID, a.ID), SourceID: im.SourceID,
			Path: p, TakenAt: a.TakenAt, Title: a.Title, Name: a.Name,
		})
	}
	return out
}

func (im *Immich) path(id string) string { return filepath.Join(im.CacheDir, id+".jpg") }

const indexFile = "index.json"

func (im *Immich) saveIndex(a []cachedAsset) error {
	b, err := json.Marshal(a)
	if err != nil {
		return err
	}
	return config.WriteFileAtomic(filepath.Join(im.CacheDir, indexFile), b, 0o644)
}

func (im *Immich) loadIndex() ([]cachedAsset, error) {
	b, err := os.ReadFile(filepath.Join(im.CacheDir, indexFile))
	if err != nil {
		return nil, err
	}
	var a []cachedAsset
	return a, json.Unmarshal(b, &a)
}

func (im *Immich) client() *http.Client {
	if im.Client != nil {
		return im.Client
	}
	return http.DefaultClient
}

func (a immichAsset) takenAt() time.Time {
	if a.ExifInfo != nil && a.ExifInfo.DateTimeOriginal != nil {
		return *a.ExifInfo.DateTimeOriginal
	}
	return a.FileCreatedAt
}

func (a immichAsset) place() string {
	if a.ExifInfo == nil {
		return ""
	}
	switch {
	case a.ExifInfo.City != "" && a.ExifInfo.Country != "":
		return a.ExifInfo.City + ", " + a.ExifInfo.Country
	case a.ExifInfo.City != "":
		return a.ExifInfo.City
	}
	return a.ExifInfo.Country
}

// unreachable turns a transport error into a short message naming the host.
func unreachable(rawURL string, err error) error {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	host := rawURL
	if u, perr := url.Parse(rawURL); perr == nil && u.Host != "" {
		host = u.Host
	}
	return &netError{host: host, err: err}
}

type netError struct {
	host string
	err  error
}

func (e *netError) Error() string { return "cannot reach " + e.host }
func (e *netError) Unwrap() error { return e.err }

// saveBody writes r to path atomically, refusing oversized or empty bodies.
func saveBody(r io.Reader, path string) error {
	b, err := io.ReadAll(io.LimitReader(r, maxDownload+1))
	if err != nil {
		return err
	}
	if len(b) == 0 || len(b) > maxDownload {
		return fmt.Errorf("download for %s: %d bytes is not a usable image", filepath.Base(path), len(b))
	}
	return config.WriteFileAtomic(path, b, 0o644)
}

// pruneDir removes files in dir whose names are not in keep.
func pruneDir(dir string, keep map[string]bool) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if !e.IsDir() && !keep[e.Name()] && !strings.HasPrefix(e.Name(), ".tmp-") {
			os.Remove(filepath.Join(dir, e.Name()))
		}
	}
}
