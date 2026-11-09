package source

import (
	"bytes"
	"context"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/chinmay28/photoframe/internal/library"
)

func pngBytes(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 4, 4))); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func write(t *testing.T, path string, b []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestDirListsImagesRecursively(t *testing.T) {
	root := t.TempDir()
	img := pngBytes(t)
	write(t, filepath.Join(root, "a.jpg"), img)
	write(t, filepath.Join(root, "Emerald Bay", "b.PNG"), img)
	write(t, filepath.Join(root, "notes.txt"), []byte("x"))
	write(t, filepath.Join(root, ".hidden", "c.jpg"), img)
	write(t, filepath.Join(root, ".d.jpg"), img)

	d := &Dir{SourceID: "up", Root: root}
	photos, err := d.Sync(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(photos) != 2 {
		t.Fatalf("got %d photos: %+v", len(photos), photos)
	}
	titles, names := map[string]bool{}, map[string]bool{}
	for _, p := range photos {
		titles[p.Title] = true
		names[p.Name] = true
		if p.TakenAt.IsZero() {
			t.Error("TakenAt should fall back to mtime")
		}
		if p.SourceID != "up" || !strings.HasPrefix(p.ID, "up-") {
			t.Errorf("bad ids: %+v", p)
		}
	}
	if !titles["Emerald Bay"] || !titles[""] {
		t.Fatalf("titles = %v", titles)
	}
	if !names["a.jpg"] || !names["b.PNG"] {
		t.Fatalf("names = %v", names)
	}
}

func TestDirMissingRootFails(t *testing.T) {
	d := &Dir{SourceID: "nas", Root: filepath.Join(t.TempDir(), "not-mounted")}
	if _, err := d.Sync(context.Background()); err == nil {
		t.Fatal("want error for missing folder")
	}
}

func immichServer(t *testing.T, album string, downloads *int32) *httptest.Server {
	img := pngBytes(t)
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-api-key") != "key" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		switch {
		case r.URL.Path == "/api/albums/alb":
			w.Write([]byte(album))
		case strings.HasPrefix(r.URL.Path, "/api/assets/") && strings.HasSuffix(r.URL.Path, "/thumbnail"):
			if r.URL.Query().Get("size") != "preview" {
				t.Errorf("thumbnail size = %q", r.URL.Query().Get("size"))
			}
			atomic.AddInt32(downloads, 1)
			w.Write(img)
		default:
			http.NotFound(w, r)
		}
	}))
}

const albumJSON = `{"assets":[
 {"id":"a1","type":"IMAGE","originalFileName":"IMG_0042.HEIC","fileCreatedAt":"2024-01-01T00:00:00Z","exifInfo":{"dateTimeOriginal":"2023-08-14T10:00:00Z","city":"South Lake Tahoe","country":"United States of America"}},
 {"id":"v1","type":"VIDEO","fileCreatedAt":"2024-01-01T00:00:00Z"},
 {"id":"a2","type":"IMAGE","fileCreatedAt":"2024-02-02T00:00:00Z"}
]}`

func TestImmichSyncDownloadsAndCaches(t *testing.T) {
	var downloads int32
	srv := immichServer(t, albumJSON, &downloads)
	defer srv.Close()

	im := &Immich{SourceID: "fam", BaseURL: srv.URL + "/", APIKey: "key", AlbumID: "alb", CacheDir: t.TempDir()}
	photos, err := im.Sync(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(photos) != 2 || downloads != 2 {
		t.Fatalf("photos=%d downloads=%d", len(photos), downloads)
	}
	var tahoe bool
	for _, p := range photos {
		if p.Title == "South Lake Tahoe, United States of America" && p.TakenAt.Year() == 2023 && p.Name == "IMG_0042.HEIC" {
			tahoe = true
		}
	}
	if !tahoe {
		t.Fatalf("exif place/date or file name not used: %+v", photos)
	}

	// Second sync: nothing new to download.
	if _, err := im.Sync(context.Background()); err != nil {
		t.Fatal(err)
	}
	if downloads != 2 {
		t.Fatalf("re-downloaded cached assets: %d", downloads)
	}

	// Server goes away: cached photos are still returned with an error.
	srv.Close()
	photos, err = im.Sync(context.Background())
	if err == nil || len(photos) != 2 {
		t.Fatalf("offline sync = %d photos, err %v", len(photos), err)
	}
	if !slices.ContainsFunc(photos, func(p library.Photo) bool { return p.Name == "IMG_0042.HEIC" }) {
		t.Fatalf("cached photos lost their file names: %+v", photos)
	}
	if !strings.HasPrefix(err.Error(), "cannot reach 127.0.0.1:") {
		t.Fatalf("offline error = %q, want a short host message", err)
	}
}

func TestImmichPrunesRemovedAssets(t *testing.T) {
	var downloads int32
	srv := immichServer(t, albumJSON, &downloads)
	im := &Immich{SourceID: "fam", BaseURL: srv.URL, APIKey: "key", AlbumID: "alb", CacheDir: t.TempDir()}
	if _, err := im.Sync(context.Background()); err != nil {
		t.Fatal(err)
	}
	srv.Close()

	srv = immichServer(t, `{"assets":[{"id":"a2","type":"IMAGE","fileCreatedAt":"2024-02-02T00:00:00Z"}]}`, &downloads)
	defer srv.Close()
	im.BaseURL = srv.URL
	photos, err := im.Sync(context.Background())
	if err != nil || len(photos) != 1 {
		t.Fatalf("photos=%d err=%v", len(photos), err)
	}
	if _, err := os.Stat(filepath.Join(im.CacheDir, "a1.jpg")); !os.IsNotExist(err) {
		t.Fatal("removed asset still cached")
	}
}

func TestImmichBadKey(t *testing.T) {
	var downloads int32
	srv := immichServer(t, albumJSON, &downloads)
	defer srv.Close()
	im := &Immich{SourceID: "fam", BaseURL: srv.URL, APIKey: "wrong", AlbumID: "alb", CacheDir: t.TempDir()}
	if _, err := im.Sync(context.Background()); err == nil || !strings.Contains(err.Error(), "API key") {
		t.Fatalf("err = %v, want 401", err)
	}
}

func TestURLImageConditionalFetch(t *testing.T) {
	img := pngBytes(t)
	var hits, notModified int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		if r.Header.Get("If-None-Match") == `"v1"` {
			atomic.AddInt32(&notModified, 1)
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("ETag", `"v1"`)
		w.Write(img)
	}))
	defer srv.Close()

	u := &URLImage{SourceID: "art", URL: srv.URL, CacheDir: t.TempDir(),
		Now: func() time.Time { return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC) }}
	first, err := u.Sync(context.Background())
	if err != nil || len(first) != 1 {
		t.Fatalf("first sync: %v %v", first, err)
	}
	second, err := u.Sync(context.Background())
	if err != nil || len(second) != 1 || second[0].ID != first[0].ID {
		t.Fatalf("second sync: %v %v", second, err)
	}
	if notModified != 1 {
		t.Fatalf("expected a conditional request, hits=%d 304s=%d", hits, notModified)
	}
}

func TestURLImageRejectsNonImage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("<html>not an image</html>"))
	}))
	defer srv.Close()
	u := &URLImage{SourceID: "art", URL: srv.URL, CacheDir: t.TempDir()}
	if photos, err := u.Sync(context.Background()); err == nil || len(photos) != 0 {
		t.Fatalf("got %v, %v", photos, err)
	}
}

func TestURLFileName(t *testing.T) {
	for raw, want := range map[string]string{
		"https://example.com/art/today%20big.jpg?x=1": "today big.jpg",
		"https://example.com/":                        "",
		"https://example.com":                         "",
		"https://example.com/feed/":                   "feed",
	} {
		if got := urlFileName(raw); got != want {
			t.Errorf("urlFileName(%q) = %q, want %q", raw, got, want)
		}
	}
}
