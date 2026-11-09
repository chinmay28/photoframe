package render

import (
	"image"
	"image/color"
	"math"
	"testing"

	"github.com/chinmay28/photoframe/internal/adjust"
	"github.com/chinmay28/photoframe/internal/config"
)

func near(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

func TestViewFillCentresLargestWindow(t *testing.T) {
	// 400×100 source into a 100×100 output: a 100×100 window in the middle.
	w := View(nil, 400, 100, 100, 100, config.FitFill, 1, nil)
	if !near(w.W, 100) || !near(w.H, 100) || !near(w.X, 150) || !near(w.Y, 0) {
		t.Fatalf("window %+v", w)
	}
}

func TestViewFitHoldsWholeSource(t *testing.T) {
	// 100×100 source into 200×100: a 200×100 window with the source centred.
	w := View(nil, 100, 100, 200, 100, config.FitFit, 1, nil)
	if !near(w.W, 200) || !near(w.H, 100) || !near(w.X, -50) || !near(w.Y, 0) {
		t.Fatalf("window %+v", w)
	}
}

func TestViewZoomShrinksWindow(t *testing.T) {
	w := View(nil, 400, 200, 200, 100, config.FitFill, 2, nil)
	if !near(w.W, 200) || !near(w.H, 100) || !near(w.X, 100) || !near(w.Y, 50) {
		t.Fatalf("window %+v", w)
	}
}

func TestViewPositionSlidesWindowAcrossSource(t *testing.T) {
	for _, tc := range []struct {
		pos adjust.Point
		x   float64
	}{
		{adjust.Point{X: 0, Y: 0.5}, 0},     // left edges aligned
		{adjust.Point{X: 1, Y: 0.5}, 300},   // right edges aligned
		{adjust.Point{X: 0.25, Y: 0.5}, 75}, // a quarter of the way
		{adjust.Point{X: 0.5, Y: 0.5}, 150}, // centre
		{adjust.Point{X: 0.5, Y: 0.0}, 150}, // y has no travel here
	} {
		p := tc.pos
		w := View(nil, 400, 100, 100, 100, config.FitFill, 1, &p)
		if !near(w.X, tc.x) || !near(w.Y, 0) {
			t.Errorf("position %+v: window %+v, want x=%v", p, w, tc.x)
		}
	}
}

func TestViewZoomedPositionUsesBothAxes(t *testing.T) {
	// A 200×100 window over 400×200, zoomed 2×: travel of 200 by 100.
	w := View(nil, 400, 200, 200, 100, config.FitFill, 2, &adjust.Point{X: 1, Y: 0})
	if !near(w.X, 200) || !near(w.Y, 0) {
		t.Fatalf("window %+v", w)
	}
}

func TestViewFitPositionAlignsSourceToAnEdge(t *testing.T) {
	// The window is bigger than the source: position 0 puts the source's
	// left edge at the window's left edge.
	w := View(nil, 100, 100, 200, 100, config.FitFit, 1, &adjust.Point{X: 0, Y: 0.5})
	if !near(w.X, 0) {
		t.Fatalf("left-aligned window %+v, want x=0", w)
	}
	w = View(nil, 100, 100, 200, 100, config.FitFit, 1, &adjust.Point{X: 1, Y: 0.5})
	if !near(w.X, -100) {
		t.Fatalf("right-aligned window %+v, want x=-100", w)
	}
}

func TestViewSmartWithoutFocusFollowsDetail(t *testing.T) {
	src := uniform(400, 100, color.RGBA{200, 200, 200, 255})
	for y := 0; y < 100; y++ {
		for x := 320; x < 400; x++ {
			if (x/4+y/4)%2 == 0 {
				src.SetRGBA(x, y, color.RGBA{0, 0, 0, 255})
			}
		}
	}
	w := View(src, 400, 100, 100, 100, config.FitSmart, 1, nil)
	if w.X < 250 {
		t.Fatalf("smart window %+v ignored the detail at x>=320", w)
	}
	// An explicit position wins over smart crop.
	w = View(src, 400, 100, 100, 100, config.FitSmart, 1, &adjust.Point{X: 0, Y: 0.5})
	if !near(w.X, 0) {
		t.Fatalf("positioned window %+v, want x=0", w)
	}
}

func TestViewDegenerateInputs(t *testing.T) {
	w := View(nil, 0, 0, 100, 100, config.FitFill, math.NaN(), nil)
	if w.W <= 0 || w.H <= 0 {
		t.Fatalf("window %+v must have a positive size", w)
	}
}

// halves returns a w×h image: left half red, right half blue.
func halves(w, h int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			c := color.RGBA{255, 0, 0, 255}
			if x >= w/2 {
				c = color.RGBA{0, 0, 255, 255}
			}
			img.SetRGBA(x, y, c)
		}
	}
	return img
}

func TestFrameShowsWindowAndBorder(t *testing.T) {
	src := halves(200, 100)
	// The left half only: all red.
	out := Frame(src, Window{X: 0, Y: 0, W: 100, H: 100}, 50, 50, color.White)
	if c := out.RGBAAt(45, 25); c.R != 255 || c.B != 0 {
		t.Fatalf("left window right edge = %v, want red", c)
	}
	// A window twice the source's height: border above and below.
	out = Frame(src, Window{X: 0, Y: -50, W: 200, H: 200}, 100, 100, color.White)
	if c := out.RGBAAt(50, 5); c != (color.RGBA{255, 255, 255, 255}) {
		t.Fatalf("top border = %v, want white", c)
	}
	if c := out.RGBAAt(75, 50); c.B != 255 {
		t.Fatalf("centre right = %v, want blue", c)
	}
}

func TestRotate(t *testing.T) {
	src := halves(4, 2) // red left, blue right
	if got := Rotate(src, 0); got != src {
		t.Fatal("0° should return the source")
	}
	r := Rotate(src, 90) // left goes to the top
	if r.Bounds().Dx() != 2 || r.Bounds().Dy() != 4 || r.RGBAAt(0, 0).R != 255 || r.RGBAAt(0, 3).B != 255 {
		t.Fatalf("90°: %v top=%v bottom=%v", r.Bounds(), r.RGBAAt(0, 0), r.RGBAAt(0, 3))
	}
	r = Rotate(src, 180)
	if r.RGBAAt(0, 0).B != 255 || r.RGBAAt(3, 0).R != 255 {
		t.Fatal("180° did not swap left and right")
	}
	r = Rotate(src, 270) // left goes to the bottom
	if r.RGBAAt(0, 0).B != 255 || r.RGBAAt(0, 3).R != 255 {
		t.Fatal("270° did not put the left edge at the bottom")
	}
}

func TestBrighten(t *testing.T) {
	img := uniform(1, 1, color.RGBA{100, 100, 100, 255})
	Brighten(img, 0.5)
	if c := img.RGBAAt(0, 0); c.R != 228 || c.G != 228 || c.B != 228 {
		t.Fatalf("brightened = %v, want 228", c)
	}
	Brighten(img, -1)
	if c := img.RGBAAt(0, 0); c.R != 0 || c.A != 255 {
		t.Fatalf("darkened = %v", c)
	}
}

func TestRenderAppliesAdjustment(t *testing.T) {
	src := halves(200, 100)
	o := Options{Width: 40, Height: 40, Fit: config.FitFill, Border: color.White, Palette: testPalette, Dither: config.DitherOrdered}
	centre := func(p *image.Paletted) color.Color { return p.Palette[p.ColorIndexAt(20, 20)] }

	o.Adjust = adjust.Adjustment{Position: &adjust.Point{X: 0, Y: 0.5}}
	if c := centre(Render(src, o)); c != testPalette[3] {
		t.Fatalf("left position centre = %v, want red", c)
	}
	o.Adjust = adjust.Adjustment{Position: &adjust.Point{X: 1, Y: 0.5}}
	if c := centre(Render(src, o)); c != testPalette[4] {
		t.Fatalf("right position centre = %v, want blue", c)
	}
	// Rotating 180° swaps the halves, so the left position now shows blue.
	o.Adjust = adjust.Adjustment{Rotate: 180, Position: &adjust.Point{X: 0, Y: 0.5}}
	if c := centre(Render(src, o)); c != testPalette[4] {
		t.Fatalf("rotated left position centre = %v, want blue", c)
	}
	// A per-photo Fit overrides the slideshow's: the whole photo with border.
	o.Adjust = adjust.Adjustment{Fit: config.FitFit}
	if c := Render(src, o).Palette[Render(src, o).ColorIndexAt(20, 2)]; c != testPalette[1] {
		t.Fatalf("fit override top = %v, want white border", c)
	}
}

func TestMaxSourceSideGrowsWithZoom(t *testing.T) {
	if MaxSourceSide(800, 480, 1) != 1600 || MaxSourceSide(800, 480, 0) != 1600 {
		t.Fatal("no zoom should keep 2× the panel")
	}
	if MaxSourceSide(800, 480, 1.5) != 2400 || MaxSourceSide(800, 480, 4) != 3200 {
		t.Fatal("zoom should add detail up to 2×")
	}
}
