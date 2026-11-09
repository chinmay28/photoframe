package thumbs

import (
	"bytes"
	"image"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/chinmay28/photoframe/internal/library"
)

func TestThumbIsCachedAndSized(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "p.png")
	f, _ := os.Create(src)
	png.Encode(f, image.NewRGBA(image.Rect(0, 0, 800, 400)))
	f.Close()

	c := New(filepath.Join(dir, "thumbs"), 200)
	ph := library.Photo{ID: "s-1", Path: src}
	b, err := c.Thumb(ph)
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
	if b, _ := c.Thumb(ph); string(b) != "cached" {
		t.Fatal("cache not used")
	}
}

func TestThumbMissingFile(t *testing.T) {
	c := New(t.TempDir(), 100)
	if _, err := c.Thumb(library.Photo{ID: "x", Path: "/nonexistent.jpg"}); err == nil {
		t.Fatal("want error")
	}
}
