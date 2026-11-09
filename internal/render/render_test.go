package render

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"testing"

	"github.com/chinmay28/photoframe/internal/config"
)

var testPalette = color.Palette{
	color.RGBA{0, 0, 0, 255},
	color.RGBA{255, 255, 255, 255},
	color.RGBA{255, 255, 0, 255},
	color.RGBA{255, 0, 0, 255},
	color.RGBA{0, 0, 255, 255},
	color.RGBA{0, 255, 0, 255},
}

func uniform(w, h int, c color.RGBA) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = c.R, c.G, c.B, 255
	}
	return img
}

func TestDitherPureColoursMapExactly(t *testing.T) {
	for _, m := range []config.Dither{config.DitherFloydSteinberg, config.DitherAtkinson, config.DitherOrdered} {
		for i, c := range testPalette {
			out := Dither(uniform(16, 8, c.(color.RGBA)), testPalette, m)
			for _, px := range out.Pix {
				if int(px) != i {
					t.Fatalf("%s: pure colour %d dithered to %d", m, i, px)
				}
			}
		}
	}
}

func TestDitherGreyMixesBlackAndWhite(t *testing.T) {
	out := Dither(uniform(32, 32, color.RGBA{128, 128, 128, 255}), testPalette, config.DitherFloydSteinberg)
	counts := map[uint8]int{}
	for _, px := range out.Pix {
		counts[px]++
	}
	black, white := counts[0], counts[1]
	if black+white < len(out.Pix)*9/10 {
		t.Fatalf("grey used colours: %v", counts)
	}
	if ratio := float64(black) / float64(black+white); ratio < 0.35 || ratio > 0.65 {
		t.Fatalf("black ratio %.2f, want about half", ratio)
	}
}

func TestFitModesProduceTargetSize(t *testing.T) {
	src := uniform(300, 100, color.RGBA{10, 20, 30, 255})
	for _, m := range []config.Fit{config.FitSmart, config.FitFill, config.FitFit} {
		out := Fit(src, 80, 48, m, color.White)
		if out.Bounds().Dx() != 80 || out.Bounds().Dy() != 48 {
			t.Fatalf("%s: size %v", m, out.Bounds())
		}
	}
}

func TestFitLetterboxesWithBorder(t *testing.T) {
	src := uniform(100, 100, color.RGBA{0, 0, 0, 255})
	out := Fit(src, 200, 100, config.FitFit, color.White)
	if c := out.RGBAAt(5, 50); c.R != 255 {
		t.Fatalf("border pixel = %v, want white", c)
	}
	if c := out.RGBAAt(100, 50); c.R != 0 {
		t.Fatalf("centre pixel = %v, want black", c)
	}
}

func TestSmartCropFindsDetail(t *testing.T) {
	// Plain wide image with a checkerboard near the right edge.
	src := uniform(400, 100, color.RGBA{200, 200, 200, 255})
	for y := 0; y < 100; y++ {
		for x := 320; x < 400; x++ {
			if (x/4+y/4)%2 == 0 {
				src.SetRGBA(x, y, color.RGBA{0, 0, 0, 255})
			}
		}
	}
	r := SmartCrop(src, 100, 100)
	if r.Dx() != 100 || r.Dy() != 100 {
		t.Fatalf("crop size %v", r)
	}
	if r.Min.X < 250 {
		t.Fatalf("crop %v ignored the detailed region at x>=320", r)
	}
}

func TestAdjustSaturation(t *testing.T) {
	img := uniform(1, 1, color.RGBA{200, 100, 100, 255})
	Adjust(img, -1, 0)
	c := img.RGBAAt(0, 0)
	if c.R != c.G || c.G != c.B {
		t.Fatalf("saturation -1 should be grey, got %v", c)
	}
}

func TestOrientRotates(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 2, 1))
	img.SetRGBA(0, 0, color.RGBA{255, 0, 0, 255}) // red on the left
	out := orient(img, 6)                         // rotate 90° CW: red goes to the top
	if out.Bounds().Dx() != 1 || out.Bounds().Dy() != 2 {
		t.Fatalf("size %v", out.Bounds())
	}
	if out.RGBAAt(0, 0).R != 255 {
		t.Fatal("orientation 6 did not put the left edge on top")
	}
}

func TestDecodeDownscales(t *testing.T) {
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, uniform(1000, 500, color.RGBA{1, 2, 3, 255}), nil); err != nil {
		t.Fatal(err)
	}
	img, err := Decode(buf.Bytes(), 200)
	if err != nil {
		t.Fatal(err)
	}
	if img.Bounds().Dx() != 200 || img.Bounds().Dy() != 100 {
		t.Fatalf("size %v", img.Bounds())
	}
	if _, err := Decode([]byte("not an image"), 0); err == nil {
		t.Fatal("want error for garbage")
	}
}

func TestRotate90(t *testing.T) {
	p := image.NewPaletted(image.Rect(0, 0, 3, 2), testPalette)
	p.SetColorIndex(0, 0, 3) // top-left
	r := Rotate90(p)
	if r.Bounds().Dx() != 2 || r.Bounds().Dy() != 3 {
		t.Fatalf("size %v", r.Bounds())
	}
	if r.ColorIndexAt(1, 0) != 3 {
		t.Fatal("top-left should move to top-right")
	}
}

func TestRenderWithCaptionUsesOnlyPalette(t *testing.T) {
	src := uniform(400, 300, color.RGBA{90, 140, 200, 255})
	o := Options{Width: 200, Height: 120, Fit: config.FitFill, Border: color.White, Dither: config.DitherFloydSteinberg,
		Palette: testPalette, Caption: "Emerald Bay · Aug 2025"}
	out := Render(src, o)
	if out.Bounds() != image.Rect(0, 0, 200, 120) {
		t.Fatalf("bounds %v", out.Bounds())
	}
	whiteInLabel := false
	for y := 90; y < 115; y++ {
		for x := 0; x < 120; x++ {
			if out.ColorIndexAt(x, y) == 1 {
				whiteInLabel = true
			}
		}
	}
	if !whiteInLabel {
		t.Fatal("caption text not drawn")
	}
}

func TestMessageEncodes(t *testing.T) {
	p := Message(800, 480, testPalette, "Photoframe", "Open http://photoframe.local")
	if err := png.Encode(&bytes.Buffer{}, p); err != nil {
		t.Fatal(err)
	}
	// The bottom strip contains every ink.
	seen := map[uint8]bool{}
	for x := 0; x < 800; x++ {
		seen[p.ColorIndexAt(x, 480-60)] = true
	}
	if len(seen) != len(testPalette) {
		t.Fatalf("ink strip shows %d colours, want %d", len(seen), len(testPalette))
	}
}

func TestOptionsFromPortraitSwaps(t *testing.T) {
	s := config.Default()
	s.Display.Orientation = config.Portrait
	o := OptionsFrom(s, 800, 480, testPalette)
	if o.Width != 480 || o.Height != 800 {
		t.Fatalf("portrait size %dx%d", o.Width, o.Height)
	}
}
