package thumbs

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/chinmay28/photoframe/internal/library"
)

var ctx = context.Background()

func writePNG(t *testing.T, path string, w, h int) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, image.NewRGBA(image.Rect(0, 0, w, h))); err != nil {
		t.Fatal(err)
	}
}

func TestThumbIsCachedAndSized(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "p.png")
	writePNG(t, src, 800, 400)

	c := New(filepath.Join(dir, "thumbs"), 200, nil)
	ph := library.Photo{ID: "s-1", Path: src}
	b, err := c.Thumb(ctx, ph)
	if err != nil {
		t.Fatal(err)
	}
	img, err := jpeg.Decode(bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	if img.Bounds().Dx() != 200 || img.Bounds().Dy() != 100 {
		t.Fatalf("thumb size %v", img.Bounds())
	}
	if _, err := os.Stat(filepath.Join(dir, "thumbs", "s-1.jpg")); err != nil {
		t.Fatal("thumb not cached")
	}
	// Served from cache even if the source becomes unreadable as an image.
	os.WriteFile(filepath.Join(dir, "thumbs", "s-1.jpg"), []byte("cached"), 0o644)
	if b, _ := c.Thumb(ctx, ph); string(b) != "cached" {
		t.Fatal("cache not used")
	}
}

func TestThumbRemadeWhenPhotoIsNewer(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "p.png")
	writePNG(t, src, 400, 400)
	c := New(filepath.Join(dir, "thumbs"), 100, nil)
	ph := library.Photo{ID: "s-1", Path: src}
	if _, err := c.Thumb(ctx, ph); err != nil {
		t.Fatal(err)
	}
	writePNG(t, src, 400, 200)
	os.Chtimes(src, time.Time{}, time.Now().Add(time.Hour))
	b, err := c.Thumb(ctx, ph)
	if err != nil {
		t.Fatal(err)
	}
	if img, _ := jpeg.Decode(bytes.NewReader(b)); img.Bounds().Dy() != 50 {
		t.Fatalf("stale thumb served: %v", img.Bounds())
	}
}

func TestThumbMissingFile(t *testing.T) {
	c := New(t.TempDir(), 100, nil)
	if _, err := c.Thumb(ctx, library.Photo{ID: "x", Path: "/nonexistent.jpg"}); err == nil {
		t.Fatal("want error")
	}
}

// Many requests for one uncached photo must do the work once.
func TestThumbSharesWorkForTheSamePhoto(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "p.png")
	writePNG(t, src, 400, 300)
	lim := NewLimiter(1)
	c := New(filepath.Join(dir, "thumbs"), 100, lim)
	ph := library.Photo{ID: "s-1", Path: src}

	lim.acquire(ctx) // hold the only slot so every request has to queue
	var wg sync.WaitGroup
	results := make([][]byte, 8)
	for i := range results {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results[i], _ = c.Thumb(ctx, ph)
		}()
	}
	waitFor(t, func() bool {
		c.mu.Lock()
		defer c.mu.Unlock()
		return len(c.inflight) == 1
	})
	lim.release()
	wg.Wait()

	for i, b := range results {
		if len(b) == 0 || &b[0] != &results[0][0] {
			t.Fatalf("request %d did not share the one rendition", i)
		}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.inflight) != 0 {
		t.Fatal("finished work still listed as in flight")
	}
}

// A request that goes away while queued returns at once, and the work still
// lands in the cache for the next request.
func TestThumbGivesUpWhenRequestEnds(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "p.png")
	writePNG(t, src, 400, 300)
	lim := NewLimiter(1)
	c := New(filepath.Join(dir, "thumbs"), 100, lim)
	ph := library.Photo{ID: "s-1", Path: src}

	lim.acquire(ctx)
	gone, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := c.Thumb(gone, ph); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	lim.release()
	waitFor(t, func() bool {
		_, err := os.Stat(c.path(ph.ID))
		return err == nil
	})
}

// Caches sharing a limiter never work on more photos at once than it allows.
func TestLimiterIsSharedBetweenCaches(t *testing.T) {
	dir := t.TempDir()
	lim := NewLimiter(1)
	small := New(filepath.Join(dir, "small"), 50, lim)
	large := New(filepath.Join(dir, "large"), 100, lim)
	var photos []library.Photo
	for i := 0; i < 4; i++ {
		p := filepath.Join(dir, fmt.Sprintf("%d.png", i))
		writePNG(t, p, 300, 200)
		photos = append(photos, library.Photo{ID: fmt.Sprintf("s-%d", i), Path: p})
	}

	lim.acquire(ctx)
	var wg sync.WaitGroup
	for _, ph := range photos {
		for _, c := range []*Cache{small, large} {
			wg.Add(1)
			go func() {
				defer wg.Done()
				if _, err := c.Thumb(ctx, ph); err != nil {
					t.Error(err)
				}
			}()
		}
	}
	waitFor(t, func() bool {
		small.mu.Lock()
		large.mu.Lock()
		defer small.mu.Unlock()
		defer large.mu.Unlock()
		return len(small.inflight)+len(large.inflight) == 8
	})
	// Everything is queued behind the held slot: nothing was made yet.
	for _, c := range []*Cache{small, large} {
		if entries, _ := os.ReadDir(c.Dir); len(entries) != 0 {
			t.Fatalf("%d renditions made past the limiter", len(entries))
		}
	}
	lim.release()
	wg.Wait()
}

func TestWarmMakesOnlyWhatIsMissing(t *testing.T) {
	dir := t.TempDir()
	c := New(filepath.Join(dir, "thumbs"), 50, nil)
	var photos []library.Photo
	for i := 0; i < 3; i++ {
		p := filepath.Join(dir, fmt.Sprintf("%d.png", i))
		writePNG(t, p, 200, 100)
		photos = append(photos, library.Photo{ID: fmt.Sprintf("s-%d", i), Path: p})
	}
	photos = append(photos, library.Photo{ID: "gone", Path: filepath.Join(dir, "gone.png")})
	bad := filepath.Join(dir, "bad.png")
	os.WriteFile(bad, []byte("not an image"), 0o644)
	photos = append(photos, library.Photo{ID: "bad", Path: bad})

	if _, err := c.Thumb(ctx, photos[0]); err != nil {
		t.Fatal(err)
	}
	if n := c.Warm(ctx, photos); n != 2 {
		t.Fatalf("made %d, want 2", n)
	}
	if n := c.Warm(ctx, photos); n != 0 {
		t.Fatalf("second pass made %d, want 0", n)
	}

	stopped, cancel := context.WithCancel(ctx)
	cancel()
	c.Remove("s-1")
	if n := c.Warm(stopped, photos); n != 0 {
		t.Fatalf("made %d after cancel", n)
	}
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(time.Millisecond) {
		if cond() {
			return
		}
	}
	t.Fatal("condition not met in time")
}
