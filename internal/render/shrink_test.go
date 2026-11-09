package render

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"testing"

	xdraw "golang.org/x/image/draw"
)

func TestShrinkFactor(t *testing.T) {
	for _, c := range []struct{ w, h, maxSide, want int }{
		{4000, 3000, 640, 6},
		{3000, 4000, 640, 6},
		{4000, 3000, 1600, 2},
		{4000, 3000, 2560, 1},
		{640, 480, 640, 1},
		{100, 80, 640, 1},
		{4000, 3000, 0, 1}, // no limit
	} {
		if got := shrinkFactor(c.w, c.h, c.maxSide); got != c.want {
			t.Errorf("shrinkFactor(%d, %d, %d) = %d, want %d", c.w, c.h, c.maxSide, got, c.want)
		}
	}
}

// quadrants returns a w×h image whose four quarters are flat colours, so a
// correct shrink is easy to recognise whatever the chroma layout.
func quadrants(w, h int, ratio image.YCbCrSubsampleRatio) *image.YCbCr {
	img := image.NewYCbCr(image.Rect(0, 0, w, h), ratio)
	cols := [4]color.YCbCr{{200, 100, 160}, {60, 180, 90}, {120, 128, 128}, {250, 60, 200}}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			c := cols[2*(2*y/h)+2*x/w]
			img.Y[img.YOffset(x, y)] = c.Y
			img.Cb[img.COffset(x, y)] = c.Cb
			img.Cr[img.COffset(x, y)] = c.Cr
		}
	}
	return img
}

func TestShrinkYCbCrAveragesBlocks(t *testing.T) {
	ratios := map[string]image.YCbCrSubsampleRatio{
		"444": image.YCbCrSubsampleRatio444, "422": image.YCbCrSubsampleRatio422,
		"420": image.YCbCrSubsampleRatio420, "440": image.YCbCrSubsampleRatio440,
	}
	for name, ratio := range ratios {
		for _, f := range []int{2, 3, 4, 5, 8} {
			// 240 is divisible by every factor and by 2·factor, so quadrant
			// edges fall on block edges and each block is one flat colour.
			src := quadrants(240, 240, ratio)
			got, ok := shrinkYCbCr(src, f)
			if !ok {
				t.Fatalf("%s f=%d: not shrunk", name, f)
			}
			if got.Rect != image.Rect(0, 0, 240/f, 240/f) {
				t.Fatalf("%s f=%d: bounds %v", name, f, got.Rect)
			}
			for y := 0; y < got.Rect.Dy(); y++ {
				for x := 0; x < got.Rect.Dx(); x++ {
					if want, c := src.YCbCrAt(x*f, y*f), got.YCbCrAt(x, y); c != want {
						t.Fatalf("%s f=%d at (%d,%d): %v, want %v", name, f, x, y, c, want)
					}
				}
			}
		}
	}
}

func TestShrinkYCbCrMeanOfBlock(t *testing.T) {
	src := image.NewYCbCr(image.Rect(0, 0, 4, 2), image.YCbCrSubsampleRatio444)
	copy(src.Y, []uint8{0, 10, 100, 200, 20, 30, 50, 51})
	got, ok := shrinkYCbCr(src, 2)
	if !ok || got.Y[0] != 15 || got.Y[1] != 100 { // (0+10+20+30)/4, (100+200+50+51)/4 rounded
		t.Fatalf("means %v ok=%v", got.Y, ok)
	}
}

// Odd sizes and sub-images must not read outside the planes.
func TestShrinkYCbCrOddSizesAndSubImages(t *testing.T) {
	for _, ratio := range []image.YCbCrSubsampleRatio{image.YCbCrSubsampleRatio420, image.YCbCrSubsampleRatio422, image.YCbCrSubsampleRatio440} {
		for _, size := range []image.Point{{7, 5}, {33, 17}, {101, 3}, {3, 101}} {
			for f := 2; f <= 5; f++ {
				src := quadrants(size.X, size.Y, ratio)
				got, ok := shrinkYCbCr(src, f)
				if want := size.X/f >= 1 && size.Y/f >= 1; ok != want {
					t.Fatalf("%v f=%d: ok=%v, want %v", size, f, ok, want)
				}
				if ok && got.Rect != image.Rect(0, 0, size.X/f, size.Y/f) {
					t.Fatalf("%v f=%d: bounds %v", size, f, got.Rect)
				}
			}
		}
		full := quadrants(64, 64, ratio)
		sub := full.SubImage(image.Rect(33, 31, 64, 64)).(*image.YCbCr) // the bottom-right quadrant, off the chroma grid
		got, ok := shrinkYCbCr(sub, 3)
		if !ok {
			t.Fatal("sub-image not shrunk")
		}
		if c, want := got.YCbCrAt(5, 5), full.YCbCrAt(60, 60); c != want {
			t.Fatalf("sub-image pixel %v, want %v", c, want)
		}
	}
}

func TestShrinkYCbCrDeclines(t *testing.T) {
	if _, ok := shrinkYCbCr(quadrants(8, 8, image.YCbCrSubsampleRatio420), 1); ok {
		t.Error("factor 1 shrunk")
	}
	if _, ok := shrinkYCbCr(quadrants(8, 8, image.YCbCrSubsampleRatio411), 2); ok {
		t.Error("unsupported chroma layout shrunk")
	}
}

// The fast path must give the same picture as the quality scaler it stands
// in for, to within what JPEG and rounding do anyway.
func TestDecodeFastPathMatchesQualityScaler(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 1200, 900))
	for y := 0; y < 900; y++ {
		for x := 0; x < 1200; x++ {
			src.SetRGBA(x, y, color.RGBA{uint8(x * 255 / 1200), uint8(y * 255 / 900), uint8((x + y) / 9), 255})
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, src, &jpeg.Options{Quality: 92}); err != nil {
		t.Fatal(err)
	}
	for _, maxSide := range []int{160, 320, 500, 1000} {
		got, err := Decode(buf.Bytes(), maxSide)
		if err != nil {
			t.Fatal(err)
		}
		want := image.NewRGBA(got.Bounds())
		xdraw.CatmullRom.Scale(want, want.Bounds(), src, src.Bounds(), xdraw.Src, nil)
		if got.Bounds().Dx() != maxSide || got.Bounds().Dy() != maxSide*3/4 {
			t.Fatalf("maxSide %d: bounds %v", maxSide, got.Bounds())
		}
		var sum, n int
		for i := range got.Pix {
			d := int(got.Pix[i]) - int(want.Pix[i])
			sum += max(d, -d)
			n++
		}
		if mean := float64(sum) / float64(n); mean > 2 {
			t.Errorf("maxSide %d: mean difference %.2f levels", maxSide, mean)
		}
	}
}

func BenchmarkDecode12MP(b *testing.B) {
	src := image.NewRGBA(image.Rect(0, 0, 4000, 3000))
	for i := range src.Pix {
		src.Pix[i] = uint8(i*7 + i/4000)
	}
	var buf bytes.Buffer
	jpeg.Encode(&buf, src, &jpeg.Options{Quality: 90})
	for _, maxSide := range []int{320, 1280, 1600} {
		b.Run(string(rune('0'+maxSide/1000))+"k"+string(rune('0'+maxSide%1000/100)), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				if _, err := Decode(buf.Bytes(), maxSide); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
