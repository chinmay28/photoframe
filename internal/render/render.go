// Package render turns photos into panel-ready paletted images: it fits them
// to the panel, tunes colour, dithers to the panel's inks and draws captions
// and info screens. It knows nothing about hardware.
package render

import (
	"image"
	"image/color"
	"image/draw"
	"sync"

	"github.com/chinmay28/photoframe/internal/adjust"
	"github.com/chinmay28/photoframe/internal/config"
	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

// Options controls Render.
type Options struct {
	Width, Height int // output size in logical (wall) orientation
	Fit           config.Fit
	Border        color.Color // background for config.FitFit
	Saturation    float64
	Contrast      float64
	Dither        config.Dither
	Palette       color.Palette
	Caption       string // drawn bottom-left when non-empty
	// Adjust is the photo's own adjustment; its Fit, when set, overrides Fit.
	Adjust adjust.Adjustment
}

// OptionsFrom builds Options from settings for a panel of the given native
// (landscape) size and palette.
func OptionsFrom(s config.Settings, panelW, panelH int, pal color.Palette) Options {
	w, h := panelW, panelH
	if s.Display.Orientation == config.Portrait {
		w, h = h, w
	}
	border := color.Color(color.White)
	if s.Border == config.BorderBlack {
		border = color.Black
	}
	return Options{
		Width: w, Height: h,
		Fit:        s.Fit,
		Border:     border,
		Saturation: s.Display.Saturation,
		Contrast:   s.Display.Contrast,
		Dither:     s.Display.Dither,
		Palette:    pal,
	}
}

// MaxSourceSide is how large Load should keep a source image for a target of
// the given size and zoom: enough headroom for cropping without wasting
// memory. Zooms beyond 2× get no extra detail, to bound memory on a Pi Zero.
func MaxSourceSide(w, h int, zoom float64) int {
	return int(2 * float64(max(w, h)) * min(max(zoom, 1), 2))
}

// Render produces the final paletted image in logical orientation.
func Render(src *image.RGBA, o Options) *image.Paletted {
	a := o.Adjust
	src = Rotate(src, a.Rotate)
	mode := o.Fit
	if a.Fit != "" {
		mode = a.Fit
	}
	sb := src.Bounds()
	win := View(src, sb.Dx(), sb.Dy(), o.Width, o.Height, mode, a.ZoomOrOne(), a.Position)
	fitted := Frame(src, win, o.Width, o.Height, o.Border)
	Brighten(fitted, a.Brightness)
	Adjust(fitted, o.Saturation, o.Contrast)
	out := Dither(fitted, o.Palette, o.Dither)
	if o.Caption != "" {
		DrawCaption(out, o.Caption)
	}
	return out
}

// Rotate90 rotates p clockwise by 90°, turning a portrait image into the
// panel's native landscape layout.
func Rotate90(p *image.Paletted) *image.Paletted {
	b := p.Bounds()
	w, h := b.Dx(), b.Dy()
	out := image.NewPaletted(image.Rect(0, 0, h, w), p.Palette)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			out.Pix[x*out.Stride+(h-1-y)] = p.Pix[(b.Min.Y+y-p.Rect.Min.Y)*p.Stride+(b.Min.X+x-p.Rect.Min.X)]
		}
	}
	return out
}

// Solid returns a w×h image filled with the palette colour nearest c.
func Solid(w, h int, pal color.Palette, c color.Color) *image.Paletted {
	out := image.NewPaletted(image.Rect(0, 0, w, h), pal)
	idx := NearestIndex(pal, c)
	for i := range out.Pix {
		out.Pix[i] = idx
	}
	return out
}

var (
	fontsOnce sync.Once
	regular   *opentype.Font
	bold      *opentype.Font
	faceMu    sync.Mutex
	faceByKey = map[faceKey]font.Face{}
)

type faceKey struct {
	bold bool
	size float64
}

func face(isBold bool, size float64) font.Face {
	fontsOnce.Do(func() {
		regular, _ = opentype.Parse(goregular.TTF)
		bold, _ = opentype.Parse(gobold.TTF)
	})
	faceMu.Lock()
	defer faceMu.Unlock()
	k := faceKey{isBold, size}
	if f, ok := faceByKey[k]; ok {
		return f
	}
	src := regular
	if isBold {
		src = bold
	}
	f, err := opentype.NewFace(src, &opentype.FaceOptions{Size: size, DPI: 72, Hinting: font.HintingFull})
	if err != nil {
		panic(err) // the embedded Go fonts always parse
	}
	faceByKey[k] = f
	return f
}

// DrawCaption draws text in white on a black label at the bottom-left.
func DrawCaption(p *image.Paletted, text string) {
	b := p.Bounds()
	size := float64(b.Dx()) / 50 // 16px on an 800px-wide panel
	f := face(true, size)
	m := f.Metrics()
	padX, padY := int(size*0.9), int(size*0.45)
	textW := font.MeasureString(f, text).Ceil()
	textW = min(textW, b.Dx()-4*padX)
	lineH := (m.Ascent + m.Descent).Ceil()
	margin := int(size * 1.5)

	box := image.Rect(b.Min.X+margin, b.Max.Y-margin-lineH-2*padY, b.Min.X+margin+textW+2*padX, b.Max.Y-margin)
	black := NearestIndex(p.Palette, color.Black)
	white := NearestIndex(p.Palette, color.White)
	fill(p, box, black)
	drawText(p, f, text, box.Min.X+padX, box.Min.Y+padY+m.Ascent.Ceil(), white, box)
}

// Message renders an information screen: a title, body lines, and a strip of
// the panel's inks along the bottom.
func Message(w, h int, pal color.Palette, title string, lines ...string) *image.Paletted {
	p := Solid(w, h, pal, color.White)
	black := NearestIndex(pal, color.Black)
	scale := float64(w) / 800
	margin := int(56 * scale)

	tf := face(true, 40*scale)
	y := margin + tf.Metrics().Ascent.Ceil()
	drawText(p, tf, title, margin, y, black, p.Bounds())
	y += int(28 * scale)

	bf := face(false, 24*scale)
	for _, line := range lines {
		y += (bf.Metrics().Ascent + bf.Metrics().Descent).Ceil() + int(8*scale)
		drawText(p, bf, line, margin, y, black, p.Bounds())
	}

	// Ink strip: one block per palette colour.
	stripH := int(24 * scale)
	cell := (w - 2*margin) / len(pal)
	for i := range pal {
		r := image.Rect(margin+i*cell, h-margin-stripH, margin+(i+1)*cell, h-margin)
		fill(p, r, uint8(i))
	}
	return p
}

func fill(p *image.Paletted, r image.Rectangle, idx uint8) {
	r = r.Intersect(p.Bounds())
	for y := r.Min.Y; y < r.Max.Y; y++ {
		row := p.Pix[(y-p.Rect.Min.Y)*p.Stride:]
		for x := r.Min.X; x < r.Max.X; x++ {
			row[x-p.Rect.Min.X] = idx
		}
	}
}

// drawText renders text with its baseline at (x, y), thresholding the
// anti-aliased glyphs so only pure palette colours are used.
func drawText(p *image.Paletted, f font.Face, text string, x, y int, idx uint8, clip image.Rectangle) {
	mask := image.NewAlpha(p.Bounds())
	d := font.Drawer{Dst: mask, Src: image.Opaque, Face: f, Dot: fixed.P(x, y)}
	d.DrawString(text)
	clip = clip.Intersect(p.Bounds())
	for yy := clip.Min.Y; yy < clip.Max.Y; yy++ {
		for xx := clip.Min.X; xx < clip.Max.X; xx++ {
			if mask.AlphaAt(xx, yy).A >= 128 {
				p.SetColorIndex(xx, yy, idx)
			}
		}
	}
}

// ToRGBA converts a paletted image for encoding previews.
func ToRGBA(p *image.Paletted) *image.RGBA {
	out := image.NewRGBA(p.Bounds())
	draw.Draw(out, out.Bounds(), p, p.Bounds().Min, draw.Src)
	return out
}
