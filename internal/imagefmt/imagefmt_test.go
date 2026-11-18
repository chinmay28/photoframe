package imagefmt_test

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"testing"
	"time"

	"github.com/chinmay28/photoframe/internal/imagefmt"
	"github.com/chinmay28/photoframe/internal/imagefmt/heictest"
	"github.com/rwcarlsen/goexif/exif"
)

func TestIsImageName(t *testing.T) {
	for name, want := range map[string]bool{
		"a.jpg": true, "b.JPEG": true, "c.png": true, "d.gif": true, "e.webp": true,
		"IMG_0042.HEIC": true, "f.heic": true, "g.heif": true,
		"notes.txt": false, "h.avif": false, "heic": false, "": false, "dir.heic/x": false,
	} {
		if got := imagefmt.IsImageName(name); got != want {
			t.Errorf("IsImageName(%q) = %v, want %v", name, got, want)
		}
	}
}

func TestIsHEIF(t *testing.T) {
	for _, tc := range []struct {
		head string
		want bool
	}{
		{"\x00\x00\x00\x18ftypheic", true},
		{"\x00\x00\x00\x18ftypheix", true},
		{"\x00\x00\x00\x18ftypmif1", true},
		{"\x00\x00\x00\x18ftypavif", false},
		{"\x00\x00\x00\x18ftypisom", false}, // an MP4
		{"\x00\x00\x00\x18ftyphei", false},  // too short
		{"\xff\xd8\xff\xe0\x00\x10JFIF\x00\x01", false},
		{"", false},
	} {
		if got := imagefmt.IsHEIF([]byte(tc.head)); got != tc.want {
			t.Errorf("IsHEIF(%q) = %v, want %v", tc.head, got, tc.want)
		}
	}
}

// near reports whether two colours differ by at most tol in every channel.
func near(a, b color.Color, tol int) bool {
	ar, ag, ab, _ := a.RGBA()
	br, bg, bb, _ := b.RGBA()
	for _, d := range []int{int(ar>>8) - int(br>>8), int(ag>>8) - int(bg>>8), int(ab>>8) - int(bb>>8)} {
		if d < -tol || d > tol {
			return false
		}
	}
	return true
}

func TestDecodeHEIC(t *testing.T) {
	src := heictest.Gradient(64, 32)
	b := heictest.Encode(t, src, nil)

	img, format, err := imagefmt.Decode(b)
	if err != nil {
		t.Fatal(err)
	}
	if format != imagefmt.HEIF || img.Bounds() != src.Bounds() {
		t.Fatalf("Decode = %s %v", format, img.Bounds())
	}
	for _, p := range []image.Point{{0, 0}, {63, 0}, {0, 31}, {63, 31}, {32, 16}} {
		if !near(img.At(p.X, p.Y), src.At(p.X, p.Y), 12) {
			t.Errorf("pixel %v = %v, want about %v", p, img.At(p.X, p.Y), src.At(p.X, p.Y))
		}
	}

	// image.Decode and DecodeConfig know the format too, for the code that
	// only checks a file is an image.
	cfg, format, err := image.DecodeConfig(bytes.NewReader(b))
	if err != nil || format != imagefmt.HEIF || cfg.Width != 64 || cfg.Height != 32 {
		t.Fatalf("DecodeConfig = %s %dx%d, %v", format, cfg.Width, cfg.Height, err)
	}
}

// rotated.heic, from github.com/gen2brain/heic's test data, is stored as
// 640×480 with an irot box turning it a quarter, and says the same with EXIF
// orientation 6, as phones write portrait photos.
func TestDecodeHEICAppliesRotation(t *testing.T) {
	b, err := os.ReadFile("testdata/rotated.heic")
	if err != nil {
		t.Fatal(err)
	}
	img, _, err := imagefmt.Decode(b)
	if err != nil {
		t.Fatal(err)
	}
	if img.Bounds() != image.Rect(0, 0, 480, 640) {
		t.Fatalf("bounds %v, want the 480×640 portrait", img.Bounds())
	}
	if stored, _, err := image.Decode(bytes.NewReader(b)); err != nil || stored.Bounds().Dx() != 640 {
		t.Fatalf("fixture no longer needs rotating: %v", err)
	}
}

func TestDecodeOtherFormats(t *testing.T) {
	var buf bytes.Buffer
	if err := png.Encode(&buf, heictest.Gradient(4, 2)); err != nil {
		t.Fatal(err)
	}
	if img, format, err := imagefmt.Decode(buf.Bytes()); err != nil || format != "png" || img.Bounds().Dx() != 4 {
		t.Fatalf("Decode png = %s, %v", format, err)
	}
	if _, _, err := imagefmt.Decode([]byte("not an image")); err == nil {
		t.Fatal("decoded garbage")
	}
}

func TestDecodeTruncatedHEICFails(t *testing.T) {
	b := heictest.Encode(t, heictest.Gradient(64, 32), nil)
	if _, _, err := imagefmt.Decode(b[:len(b)/2]); err == nil {
		t.Fatal("decoded half a file")
	}
}

func TestHEIFExif(t *testing.T) {
	b := heictest.Encode(t, heictest.Gradient(16, 16), heictest.DateTime("2023:08:14 10:00:00"))
	tiff, err := imagefmt.HEIFExif(bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	x, err := exif.Decode(bytes.NewReader(tiff))
	if err != nil {
		t.Fatal(err)
	}
	got, err := x.DateTime()
	if err != nil {
		t.Fatal(err)
	}
	if want := time.Date(2023, 8, 14, 10, 0, 0, 0, time.Local); !got.Equal(want) {
		t.Fatalf("date %v, want %v", got, want)
	}

	if _, err := imagefmt.HEIFExif(bytes.NewReader(heictest.Encode(t, heictest.Gradient(16, 16), nil))); err == nil {
		t.Error("found EXIF in a file without any")
	}
	if _, err := imagefmt.HEIFExif(bytes.NewReader([]byte("\xff\xd8\xff\xe0 a JPEG"))); err == nil {
		t.Error("found HEIF EXIF in a JPEG")
	}
}
