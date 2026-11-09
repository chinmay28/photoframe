// Package thumbs makes and caches small JPEG thumbnails for the app.
package thumbs

import (
	"bytes"
	"image"
	"image/jpeg"
	"os"
	"path/filepath"

	"github.com/chinmay28/photoframe/internal/config"
	"github.com/chinmay28/photoframe/internal/library"
	"github.com/chinmay28/photoframe/internal/render"
	xdraw "golang.org/x/image/draw"
)

// Cache stores thumbnails under Dir, regenerating one when its photo file is
// newer. At most two thumbnails are generated at once to keep a small board
// responsive.
type Cache struct {
	Dir  string
	Size int // longest side in pixels

	sem chan struct{}
}

// New returns a cache writing to dir.
func New(dir string, size int) *Cache {
	return &Cache{Dir: dir, Size: size, sem: make(chan struct{}, 2)}
}

// Thumb returns a JPEG thumbnail for ph.
func (c *Cache) Thumb(ph library.Photo) ([]byte, error) {
	path := filepath.Join(c.Dir, ph.ID+".jpg")
	src, err := os.Stat(ph.Path)
	if err != nil {
		return nil, err
	}
	if st, err := os.Stat(path); err == nil && !st.ModTime().Before(src.ModTime()) {
		if b, err := os.ReadFile(path); err == nil {
			return b, nil
		}
	}

	c.sem <- struct{}{}
	defer func() { <-c.sem }()

	img, err := render.Load(ph.Path, c.Size*2)
	if err != nil {
		return nil, err
	}
	b := img.Bounds()
	w, h := c.Size, c.Size
	if b.Dx() > b.Dy() {
		h = max(1, b.Dy()*c.Size/b.Dx())
	} else {
		w = max(1, b.Dx()*c.Size/b.Dy())
	}
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	xdraw.CatmullRom.Scale(dst, dst.Bounds(), img, b, xdraw.Src, nil)

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, dst, &jpeg.Options{Quality: 80}); err != nil {
		return nil, err
	}
	if err := config.WriteFileAtomic(path, buf.Bytes(), 0o644); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// Remove deletes the cached thumbnail for a photo ID.
func (c *Cache) Remove(id string) { os.Remove(filepath.Join(c.Dir, id+".jpg")) }
