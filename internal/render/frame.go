package render

import (
	"image"
	"image/color"
	"image/draw"
	"math"

	"github.com/chinmay28/photoframe/internal/adjust"
	"github.com/chinmay28/photoframe/internal/config"
	xdraw "golang.org/x/image/draw"
)

// Window is the region of a source image, in source pixels, that maps onto
// the whole output. It may extend past the source (config.FitFit), in
// which case the uncovered output is border.
type Window struct {
	X, Y, W, H float64 // top-left corner and size
}

// View computes the window of a sw×sh source shown in a w×h output for the
// given framing mode, zoom (≥ 1) and position. The window keeps the
// output's aspect ratio.
//
// Position works like CSS object-position: on each axis 0 lines up the
// source's left (top) edge with the window's, 1 the right (bottom) edges,
// and values between slide the window across; this holds both when the
// window crops the source and when it is larger than the source (a border).
// A nil position centres the window, except that FitSmart centres it on the
// most detailed region of src (src may be nil; FitSmart then centres too).
func View(src *image.RGBA, sw, sh, w, h int, mode config.Fit, zoom float64, pos *adjust.Point) Window {
	if sw <= 0 || sh <= 0 || w <= 0 || h <= 0 {
		return Window{W: float64(max(sw, 1)), H: float64(max(sh, 1))}
	}
	if !(zoom >= 1) { // also catches NaN
		zoom = 1
	}
	fw, fh, tw, th := float64(sw), float64(sh), float64(w), float64(h)
	wider := fw*th > fh*tw // source is wider than the output

	var vw, vh float64
	if mode == config.FitFit {
		// The smallest output-shaped window that holds the whole source.
		if wider {
			vw, vh = fw, fw*th/tw
		} else {
			vw, vh = fh*tw/th, fh
		}
	} else {
		// The largest output-shaped window inside the source.
		if wider {
			vw, vh = fh*tw/th, fh
		} else {
			vw, vh = fw, fw*th/tw
		}
	}
	vw, vh = vw/zoom, vh/zoom

	px, py := 0.5, 0.5
	switch {
	case pos != nil:
		px, py = unit(pos.X), unit(pos.Y)
	case mode == config.FitSmart && src != nil:
		r := SmartCrop(src, w, h)
		cx := float64(r.Min.X+r.Max.X)/2 - float64(src.Rect.Min.X)
		cy := float64(r.Min.Y+r.Max.Y)/2 - float64(src.Rect.Min.Y)
		px, py = positionOf(cx, vw, fw), positionOf(cy, vh, fh)
	}
	// The window's left edge travels from 0 (edges aligned on the left) to
	// fw-vw (aligned on the right); negative travel means a border.
	return Window{X: px * (fw - vw), Y: py * (fh - vh), W: vw, H: vh}
}

// positionOf is the position that centres a window of length v on c over a
// source of length s, clamped to [0, 1].
func positionOf(c, v, s float64) float64 {
	if s == v {
		return 0.5
	}
	return unit((c - v/2) / (s - v))
}

func unit(v float64) float64 {
	if math.IsNaN(v) {
		return 0.5
	}
	return math.Min(math.Max(v, 0), 1)
}

// Frame draws the part of src inside win onto a new w×h image, filling any
// area the source does not cover with bg.
func Frame(src *image.RGBA, win Window, w, h int, bg color.Color) *image.RGBA {
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	sb := src.Bounds()
	if sb.Empty() || win.W <= 0 || win.H <= 0 {
		draw.Draw(dst, dst.Bounds(), image.NewUniform(bg), image.Point{}, draw.Src)
		return dst
	}
	// Visible part of the source, in source-relative coordinates.
	x0, y0 := math.Max(win.X, 0), math.Max(win.Y, 0)
	x1, y1 := math.Min(win.X+win.W, float64(sb.Dx())), math.Min(win.Y+win.H, float64(sb.Dy()))
	kx, ky := float64(w)/win.W, float64(h)/win.H
	dr := image.Rect(
		int(math.Round((x0-win.X)*kx)), int(math.Round((y0-win.Y)*ky)),
		int(math.Round((x1-win.X)*kx)), int(math.Round((y1-win.Y)*ky)),
	).Intersect(dst.Bounds())
	if dr != dst.Bounds() {
		draw.Draw(dst, dst.Bounds(), image.NewUniform(bg), image.Point{}, draw.Src)
	}
	sr := image.Rect(
		sb.Min.X+int(math.Floor(x0)), sb.Min.Y+int(math.Floor(y0)),
		sb.Min.X+int(math.Ceil(x1)), sb.Min.Y+int(math.Ceil(y1)),
	).Intersect(sb)
	if !dr.Empty() && !sr.Empty() {
		xdraw.CatmullRom.Scale(dst, dr, src, sr, xdraw.Src, nil)
	}
	return dst
}

// Rotate returns src turned clockwise by deg (0, 90, 180 or 270). Other
// values return src unchanged.
func Rotate(src *image.RGBA, deg int) *image.RGBA {
	// EXIF orientations: 6 is 90° clockwise, 3 is 180°, 8 is 90° anticlockwise.
	switch deg {
	case 90:
		return orient(src, 6)
	case 180:
		return orient(src, 3)
	case 270:
		return orient(src, 8)
	}
	return src
}

// Brighten shifts every channel by amount of the full range; amount is in
// [-1, 1], so 1 turns everything white and -1 black.
func Brighten(img *image.RGBA, amount float64) {
	if amount == 0 {
		return
	}
	d := amount * 255
	for i := 0; i+3 < len(img.Pix); i += 4 {
		img.Pix[i] = clamp8(float64(img.Pix[i]) + d)
		img.Pix[i+1] = clamp8(float64(img.Pix[i+1]) + d)
		img.Pix[i+2] = clamp8(float64(img.Pix[i+2]) + d)
	}
}
