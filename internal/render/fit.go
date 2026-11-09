package render

import (
	"image"
	"image/color"

	"github.com/chinmay28/photoframe/internal/config"
)

// Fit scales src into a w×h image according to mode. For config.FitFit the
// uncovered area is filled with bg. It is Frame with View's automatic window.
func Fit(src *image.RGBA, w, h int, mode config.Fit, bg color.Color) *image.RGBA {
	sb := src.Bounds()
	return Frame(src, View(src, sb.Dx(), sb.Dy(), w, h, mode, 1, nil), w, h, bg)
}

// cropSize returns the largest w:h window that fits in b.
func cropSize(b image.Rectangle, w, h int) (int, int) {
	sw, sh := b.Dx(), b.Dy()
	if sw*h > sh*w { // source is wider than target
		return max(1, sh*w/h), sh
	}
	return sw, max(1, sw*h/w)
}

func centerCrop(b image.Rectangle, w, h int) image.Rectangle {
	cw, ch := cropSize(b, w, h)
	x0 := b.Min.X + (b.Dx()-cw)/2
	y0 := b.Min.Y + (b.Dy()-ch)/2
	return image.Rect(x0, y0, x0+cw, y0+ch)
}

// SmartCrop picks the w:h window of src with the most edge detail, with a
// mild preference for the centre. Detail is a cheap stand-in for "where the
// subject is" that works well enough for landscapes and portraits alike.
func SmartCrop(src *image.RGBA, w, h int) image.Rectangle {
	b := src.Bounds()
	cw, ch := cropSize(b, w, h)
	horizontal := cw < b.Dx() // the window slides along x
	span := b.Dx() - cw
	if !horizontal {
		span = b.Dy() - ch
	}
	if span <= 0 {
		return centerCrop(b, w, h)
	}

	// Sample energy on a coarse grid along the sliding axis.
	const bins = 64
	length := b.Dx()
	if !horizontal {
		length = b.Dy()
	}
	nb := min(bins, length)
	energy := make([]float64, nb)
	step := max(1, max(b.Dx(), b.Dy())/256) // subsample for speed
	for y := b.Min.Y + step; y < b.Max.Y-step; y += step {
		for x := b.Min.X + step; x < b.Max.X-step; x += step {
			e := absDiff(lum(src, x+step, y), lum(src, x-step, y)) + absDiff(lum(src, x, y+step), lum(src, x, y-step))
			pos := x - b.Min.X
			if !horizontal {
				pos = y - b.Min.Y
			}
			energy[pos*nb/length] += e
		}
	}

	prefix := make([]float64, nb+1)
	for i, e := range energy {
		prefix[i+1] = prefix[i] + e
	}
	win := cw
	if !horizontal {
		win = ch
	}
	winBins := max(1, win*nb/length)

	bestStart, bestScore := (nb-winBins)/2, -1.0
	for s := 0; s+winBins <= nb; s++ {
		score := prefix[s+winBins] - prefix[s]
		// Up to 15% penalty for distance from centre.
		mid := float64(nb-winBins) / 2
		if mid > 0 {
			d := (float64(s) - mid) / mid
			if d < 0 {
				d = -d
			}
			score *= 1 - 0.15*d
		}
		if score > bestScore {
			bestStart, bestScore = s, score
		}
	}

	offset := min(span, bestStart*length/nb)
	if horizontal {
		return image.Rect(b.Min.X+offset, b.Min.Y, b.Min.X+offset+cw, b.Min.Y+ch)
	}
	return image.Rect(b.Min.X, b.Min.Y+offset, b.Min.X+cw, b.Min.Y+offset+ch)
}

func lum(img *image.RGBA, x, y int) int {
	i := img.PixOffset(x, y)
	p := img.Pix[i : i+3 : i+3]
	return (299*int(p[0]) + 587*int(p[1]) + 114*int(p[2])) / 1000
}

func absDiff(a, b int) float64 {
	if a > b {
		return float64(a - b)
	}
	return float64(b - a)
}
