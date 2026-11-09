package render

import (
	"image"
	"image/color"

	"github.com/chinmay28/photoframe/internal/config"
)

// Adjust changes saturation and contrast in place. Both are in [-1, 1];
// 0 leaves the image unchanged.
func Adjust(img *image.RGBA, saturation, contrast float64) {
	if saturation == 0 && contrast == 0 {
		return
	}
	cf := 1 + contrast
	sf := 1 + saturation
	for i := 0; i+3 < len(img.Pix); i += 4 {
		r, g, b := float64(img.Pix[i]), float64(img.Pix[i+1]), float64(img.Pix[i+2])
		r, g, b = (r-128)*cf+128, (g-128)*cf+128, (b-128)*cf+128
		l := 0.299*r + 0.587*g + 0.114*b
		img.Pix[i] = clamp8(l + (r-l)*sf)
		img.Pix[i+1] = clamp8(l + (g-l)*sf)
		img.Pix[i+2] = clamp8(l + (b-l)*sf)
	}
}

// Dither maps img onto pal using the given method.
func Dither(img *image.RGBA, pal color.Palette, method config.Dither) *image.Paletted {
	q := newQuantizer(pal)
	b := img.Bounds()
	out := image.NewPaletted(image.Rect(0, 0, b.Dx(), b.Dy()), pal)
	if method == config.DitherOrdered {
		ordered(img, out, q)
		return out
	}
	k, ok := kernels[method]
	if !ok {
		k = kernels[config.DitherFloydSteinberg]
	}
	diffuse(img, out, q, k)
	return out
}

type tap struct {
	dx, dy int
	w      float32
}

var kernels = map[config.Dither][]tap{
	config.DitherFloydSteinberg: {{1, 0, 7.0 / 16}, {-1, 1, 3.0 / 16}, {0, 1, 5.0 / 16}, {1, 1, 1.0 / 16}},
	// Atkinson spreads only 6/8 of the error, which keeps highlights clean.
	config.DitherAtkinson: {{1, 0, 1.0 / 8}, {2, 0, 1.0 / 8}, {-1, 1, 1.0 / 8}, {0, 1, 1.0 / 8}, {1, 1, 1.0 / 8}, {0, 2, 1.0 / 8}},
}

func diffuse(img *image.RGBA, out *image.Paletted, q quantizer, k []tap) {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	// Error buffers for the current row and the two below it.
	const rows = 3
	errs := make([][]float32, rows)
	for i := range errs {
		errs[i] = make([]float32, (w+4)*3)
	}
	for y := 0; y < h; y++ {
		cur := errs[0]
		for x := 0; x < w; x++ {
			si := img.PixOffset(b.Min.X+x, b.Min.Y+y)
			ei := (x + 2) * 3
			// Clamping stops error from piling up in saturated regions.
			r := clampf(float32(img.Pix[si]) + cur[ei])
			g := clampf(float32(img.Pix[si+1]) + cur[ei+1])
			bl := clampf(float32(img.Pix[si+2]) + cur[ei+2])
			idx := q.nearest(r, g, bl)
			out.Pix[y*out.Stride+x] = uint8(idx)
			pc := q.rgb[idx]
			er, eg, eb := r-pc[0], g-pc[1], bl-pc[2]
			for _, t := range k {
				nx := x + t.dx
				if nx < 0 || nx >= w || y+t.dy >= h {
					continue
				}
				row := errs[t.dy]
				j := (nx + 2) * 3
				row[j] += er * t.w
				row[j+1] += eg * t.w
				row[j+2] += eb * t.w
			}
		}
		// Rotate rows: the next row becomes current, clear the recycled one.
		first := errs[0]
		copy(errs, errs[1:])
		for i := range first {
			first[i] = 0
		}
		errs[rows-1] = first
	}
}

// bayer8 is the standard 8×8 ordered-dither threshold matrix.
var bayer8 = [8][8]float32{
	{0, 32, 8, 40, 2, 34, 10, 42},
	{48, 16, 56, 24, 50, 18, 58, 26},
	{12, 44, 4, 36, 14, 46, 6, 38},
	{60, 28, 52, 20, 62, 30, 54, 22},
	{3, 35, 11, 43, 1, 33, 9, 41},
	{51, 19, 59, 27, 49, 17, 57, 25},
	{15, 47, 7, 39, 13, 45, 5, 37},
	{63, 31, 55, 23, 61, 29, 53, 21},
}

func ordered(img *image.RGBA, out *image.Paletted, q quantizer) {
	const spread = 96 // roughly the gap between neighbouring inks
	b := img.Bounds()
	for y := 0; y < b.Dy(); y++ {
		for x := 0; x < b.Dx(); x++ {
			t := (bayer8[y&7][x&7]/64 - 0.5) * spread
			si := img.PixOffset(b.Min.X+x, b.Min.Y+y)
			idx := q.nearest(float32(img.Pix[si])+t, float32(img.Pix[si+1])+t, float32(img.Pix[si+2])+t)
			out.Pix[y*out.Stride+x] = uint8(idx)
		}
	}
}

type quantizer struct{ rgb [][3]float32 }

func newQuantizer(pal color.Palette) quantizer {
	q := quantizer{rgb: make([][3]float32, len(pal))}
	for i, c := range pal {
		r, g, b, _ := c.RGBA()
		q.rgb[i] = [3]float32{float32(r >> 8), float32(g >> 8), float32(b >> 8)}
	}
	return q
}

// nearest returns the palette index closest to (r, g, b), using a weighted
// distance that roughly tracks perceived difference.
func (q quantizer) nearest(r, g, b float32) int {
	best, bestD := 0, float32(1e30)
	for i, c := range q.rgb {
		dr, dg, db := r-c[0], g-c[1], b-c[2]
		d := 2*dr*dr + 4*dg*dg + 3*db*db
		if d < bestD {
			best, bestD = i, d
		}
	}
	return best
}

// NearestIndex returns the index in pal closest to c.
func NearestIndex(pal color.Palette, c color.Color) uint8 {
	r, g, b, _ := c.RGBA()
	return uint8(newQuantizer(pal).nearest(float32(r>>8), float32(g>>8), float32(b>>8)))
}

func clamp8(v float64) uint8 {
	switch {
	case v < 0:
		return 0
	case v > 255:
		return 255
	}
	return uint8(v + 0.5)
}

func clampf(v float32) float32 {
	switch {
	case v < 0:
		return 0
	case v > 255:
		return 255
	}
	return v
}
