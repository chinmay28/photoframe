package render

import (
	"bytes"
	"fmt"
	"image"
	"image/draw"
	_ "image/gif" // register decoders
	_ "image/jpeg"
	_ "image/png"
	"os"

	"github.com/rwcarlsen/goexif/exif"
	xdraw "golang.org/x/image/draw"
	_ "golang.org/x/image/webp"
)

// Load reads an image file, applies its EXIF orientation, and downsamples it
// so neither side exceeds maxSide (0 means no limit). Downsampling early
// keeps memory low on small boards such as the Pi Zero.
func Load(path string, maxSide int) (*image.RGBA, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return Decode(b, maxSide)
}

// Decode is Load for an in-memory encoded image.
func Decode(b []byte, maxSide int) (*image.RGBA, error) {
	src, _, err := image.Decode(bytes.NewReader(b))
	if err != nil {
		return nil, fmt.Errorf("decode image: %w", err)
	}
	img := downscale(src, maxSide)
	return orient(img, exifOrientation(b)), nil
}

// exifOrientation returns the EXIF orientation tag (1-8), or 1 when absent.
func exifOrientation(b []byte) int {
	x, err := exif.Decode(bytes.NewReader(b))
	if err != nil {
		return 1
	}
	tag, err := x.Get(exif.Orientation)
	if err != nil {
		return 1
	}
	v, err := tag.Int(0)
	if err != nil || v < 1 || v > 8 {
		return 1
	}
	return v
}

// downscale converts src to RGBA, shrinking it to fit within maxSide.
func downscale(src image.Image, maxSide int) *image.RGBA {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	if maxSide > 0 && (w > maxSide || h > maxSide) {
		if w >= h {
			h = max(1, h*maxSide/w)
			w = maxSide
		} else {
			w = max(1, w*maxSide/h)
			h = maxSide
		}
		dst := image.NewRGBA(image.Rect(0, 0, w, h))
		xdraw.BiLinear.Scale(dst, dst.Bounds(), src, b, xdraw.Src, nil)
		return dst
	}
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(dst, dst.Bounds(), src, b.Min, draw.Src)
	return dst
}

// orient applies an EXIF orientation so the image appears upright.
func orient(img *image.RGBA, o int) *image.RGBA {
	if o <= 1 || o > 8 {
		return img
	}
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	swap := o >= 5
	dw, dh := w, h
	if swap {
		dw, dh = h, w
	}
	dst := image.NewRGBA(image.Rect(0, 0, dw, dh))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			var dx, dy int
			switch o {
			case 2: // mirror horizontal
				dx, dy = w-1-x, y
			case 3: // rotate 180
				dx, dy = w-1-x, h-1-y
			case 4: // mirror vertical
				dx, dy = x, h-1-y
			case 5: // transpose
				dx, dy = y, x
			case 6: // rotate 90 CW
				dx, dy = h-1-y, x
			case 7: // transverse
				dx, dy = h-1-y, w-1-x
			case 8: // rotate 90 CCW
				dx, dy = y, w-1-x
			}
			si := img.PixOffset(b.Min.X+x, b.Min.Y+y)
			di := dst.PixOffset(dx, dy)
			copy(dst.Pix[di:di+4], img.Pix[si:si+4])
		}
	}
	return dst
}
