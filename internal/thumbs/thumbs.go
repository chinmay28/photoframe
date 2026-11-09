// Package thumbs makes and caches small JPEG renditions of photos for the app.
package thumbs

import (
	"bytes"
	"context"
	"image/jpeg"
	"os"
	"path/filepath"
	"sync"

	"github.com/chinmay28/photoframe/internal/config"
	"github.com/chinmay28/photoframe/internal/library"
	"github.com/chinmay28/photoframe/internal/render"
)

// Limiter bounds how many renditions are made at once, across every Cache
// that shares it. Decoding a phone photo takes tens of megabytes and a core
// for a second or more on a Pi Zero, so the bound is what keeps the board
// answering the app (and out of the OOM killer) while a screenful of
// thumbnails is requested.
type Limiter struct{ slots chan struct{} }

// NewLimiter allows n renditions at once; n < 1 means 1.
func NewLimiter(n int) *Limiter { return &Limiter{slots: make(chan struct{}, max(1, n))} }

func (l *Limiter) acquire(ctx context.Context) error {
	select {
	case l.slots <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (l *Limiter) release() { <-l.slots }

// Cache stores renditions under Dir, remaking one when its photo file is
// newer. It is safe for concurrent use: simultaneous requests for the same
// photo share one piece of work.
type Cache struct {
	Dir  string
	Size int // longest side in pixels

	lim *Limiter

	mu       sync.Mutex
	inflight map[string]*call
}

// call is one rendition being made, shared by everyone who asked for it.
type call struct {
	done chan struct{}
	b    []byte
	err  error
}

// New returns a cache writing to dir. Caches given the same Limiter share
// its bound; a nil Limiter gives the cache its own, of two.
func New(dir string, size int, lim *Limiter) *Cache {
	if lim == nil {
		lim = NewLimiter(2)
	}
	return &Cache{Dir: dir, Size: size, lim: lim, inflight: map[string]*call{}}
}

func (c *Cache) path(id string) string { return filepath.Join(c.Dir, id+".jpg") }

// cached returns the stored rendition when it is at least as new as the
// photo. A missing photo file is an error.
func (c *Cache) cached(ph library.Photo, read bool) ([]byte, bool, error) {
	src, err := os.Stat(ph.Path)
	if err != nil {
		return nil, false, err
	}
	st, err := os.Stat(c.path(ph.ID))
	if err != nil || st.ModTime().Before(src.ModTime()) {
		return nil, false, nil
	}
	if !read {
		return nil, true, nil
	}
	b, err := os.ReadFile(c.path(ph.ID))
	return b, err == nil, nil
}

// Thumb returns a JPEG rendition of ph, from the cache when it has one. It
// gives up with ctx's error if ctx ends while waiting for a turn; work
// already started carries on for the others waiting on it and for the cache.
func (c *Cache) Thumb(ctx context.Context, ph library.Photo) ([]byte, error) {
	if b, ok, err := c.cached(ph, true); err != nil || ok {
		return b, err
	}

	c.mu.Lock()
	cl, running := c.inflight[ph.ID]
	if !running {
		cl = &call{done: make(chan struct{})}
		c.inflight[ph.ID] = cl
		go c.run(ph, cl)
	}
	c.mu.Unlock()

	select {
	case <-cl.done:
		return cl.b, cl.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// run makes one rendition and hands it to everyone waiting on cl. It is not
// tied to a request: by the time it gets a turn the app has often moved on
// and asked again, and that request should find the work done.
func (c *Cache) run(ph library.Photo, cl *call) {
	defer func() {
		c.mu.Lock()
		delete(c.inflight, ph.ID)
		c.mu.Unlock()
		close(cl.done)
	}()
	if cl.err = c.lim.acquire(context.Background()); cl.err != nil {
		return
	}
	defer c.lim.release()
	cl.b, cl.err = c.make(ph)
}

func (c *Cache) make(ph library.Photo) ([]byte, error) {
	img, err := render.Load(ph.Path, c.Size)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 80}); err != nil {
		return nil, err
	}
	if err := config.WriteFileAtomic(c.path(ph.ID), buf.Bytes(), 0o644); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// Warm makes the missing renditions of photos one at a time, so the app
// finds them ready instead of waiting for each as it scrolls. It returns
// how many it made. Photos that cannot be read are skipped; Warm stops
// early only when ctx ends.
func (c *Cache) Warm(ctx context.Context, photos []library.Photo) int {
	made := 0
	for _, ph := range photos {
		if ctx.Err() != nil {
			break
		}
		if _, ok, err := c.cached(ph, false); err != nil || ok {
			continue
		}
		if _, err := c.Thumb(ctx, ph); err == nil {
			made++
		}
	}
	return made
}

// Remove deletes the cached rendition for a photo ID.
func (c *Cache) Remove(id string) { os.Remove(c.path(id)) }
