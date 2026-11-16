// Command photoframe-icons derives the Android app's icons from the frame's
// own artwork, so there is one icon to draw and every size is made from it:
//
//	go run ./cmd/photoframe-icons          # write them (make icons)
//	go run ./cmd/photoframe-icons -check   # list the stale ones, exit 1
//
// The source is web/icons/icon.svg, the web app's icon, rendered fresh at
// each size (it is vector art, so nothing is scaled from a bitmap). The
// developer badge is copied from web/icons as it is, plus a round cut of it.
// `go test ./cmd/photoframe-icons` fails when a committed copy has drifted.
// See apps/android/README.md.
package main

import (
	"bytes"
	"flag"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
)

const (
	artPath = "web/icons/icon.svg"
	res     = "apps/android/app/src/main/res"
)

// densities are Android's, with the scale against mdpi in quarters.
var densities = []struct {
	name  string
	scale int
}{{"mdpi", 4}, {"hdpi", 6}, {"xhdpi", 8}, {"xxhdpi", 12}, {"xxxhdpi", 16}}

// output is one file the generator owns.
type output struct {
	path string // relative to the repository root
	data []byte
	png  bool
}

func main() {
	check := flag.Bool("check", false, "report stale files instead of writing them")
	root := flag.String("root", ".", "the repository root")
	flag.Parse()

	files, err := plan(*root)
	if err != nil {
		fmt.Fprintln(os.Stderr, "photoframe-icons:", err)
		os.Exit(1)
	}
	if *check {
		stale, err := staleFiles(*root, files)
		if err != nil {
			fmt.Fprintln(os.Stderr, "photoframe-icons:", err)
			os.Exit(1)
		}
		for _, f := range stale {
			fmt.Println("stale:", f)
		}
		if len(stale) > 0 {
			os.Exit(1)
		}
		return
	}
	for _, f := range files {
		p := filepath.Join(*root, f.path)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			fmt.Fprintln(os.Stderr, "photoframe-icons:", err)
			os.Exit(1)
		}
		if err := os.WriteFile(p, f.data, 0o644); err != nil {
			fmt.Fprintln(os.Stderr, "photoframe-icons:", err)
			os.Exit(1)
		}
		fmt.Println("wrote", f.path)
	}
}

// plan renders every derived file in memory.
func plan(root string) ([]output, error) {
	svg, err := os.ReadFile(filepath.Join(root, artPath))
	if err != nil {
		return nil, err
	}
	art, err := parseSVG(svg)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", artPath, err)
	}
	var out []output
	add := func(path string, img image.Image) error {
		var b bytes.Buffer
		if err := png.Encode(&b, img); err != nil {
			return err
		}
		out = append(out, output{path: path, data: b.Bytes(), png: true})
		return nil
	}

	for _, d := range densities {
		// The legacy launcher icon: the whole rounded tile, 48dp.
		px := 48 * d.scale / 4
		if err := add(res+"/mipmap-"+d.name+"/ic_launcher.png", art.render(px, float32(px), 0, false)); err != nil {
			return nil, err
		}
		// The adaptive foreground: the scene without its tile, on a 108dp
		// canvas, with the artwork's square in the 72dp a launcher shows —
		// the picture frame then sits well inside the 66dp safe circle.
		fg := 108 * d.scale / 4
		inner := 72 * d.scale / 4
		if err := add(res+"/mipmap-"+d.name+"/ic_launcher_foreground.png", art.render(fg, float32(inner), float32(fg-inner)/2, true)); err != nil {
			return nil, err
		}
	}
	// The themed-icon silhouette (Android 13+ tints it with the wallpaper's
	// colours): the light parts of the scene — the mount and the sun — cut
	// out of the foreground, which is what still reads as a picture frame in
	// one colour.
	for _, d := range densities {
		fg := 108 * d.scale / 4
		inner := 72 * d.scale / 4
		if err := add(res+"/mipmap-"+d.name+"/ic_launcher_monochrome.png", silhouette(art.render(fg, float32(inner), float32(fg-inner)/2, true))); err != nil {
			return nil, err
		}
	}
	// The mark on the app's own screens (setup, connecting), larger than any
	// launcher size so it is never a duplicate of one.
	if err := add(res+"/drawable-nodpi/photoframe_logo.png", art.render(256, 256, 0, false)); err != nil {
		return nil, err
	}

	p := art.plate()
	out = append(out,
		output{path: res + "/mipmap-anydpi-v26/ic_launcher.xml", data: []byte(adaptiveXML)},
		output{path: res + "/values/ic_launcher_background.xml", data: fmt.Appendf(nil, backgroundXML, p.R, p.G, p.B)},
	)

	// The developer badge: the web app's two PNGs as they are, and a round
	// cut of the small one for the settings row.
	for _, b := range []struct{ from, to string }{
		{"web/icons/dev-badge.png", res + "/drawable-nodpi/dev_badge.png"},
		{"web/icons/dev-badge-full.png", res + "/drawable-nodpi/dev_badge_full.png"},
	} {
		data, err := os.ReadFile(filepath.Join(root, b.from))
		if err != nil {
			return nil, err
		}
		out = append(out, output{path: b.to, data: data, png: true})
	}
	badge, err := decodePNG(filepath.Join(root, "web/icons/dev-badge.png"))
	if err != nil {
		return nil, err
	}
	if err := add(res+"/drawable-nodpi/dev_badge_round.png", roundMask(badge)); err != nil {
		return nil, err
	}
	return out, nil
}

const adaptiveXML = `<?xml version="1.0" encoding="utf-8"?>
<!-- Generated from web/icons/icon.svg by ` + "`make icons`" + `; edit that, not this. -->
<adaptive-icon xmlns:android="http://schemas.android.com/apk/res/android">
    <background android:drawable="@color/ic_launcher_background" />
    <foreground android:drawable="@mipmap/ic_launcher_foreground" />
    <monochrome android:drawable="@mipmap/ic_launcher_monochrome" />
</adaptive-icon>
`

const backgroundXML = `<?xml version="1.0" encoding="utf-8"?>
<!-- The icon's tile colour. Generated from web/icons/icon.svg by ` + "`make icons`" + `. -->
<resources>
    <color name="ic_launcher_background">#%02X%02X%02X</color>
</resources>
`

// silhouette keeps the light parts of img, as white at their own coverage,
// and drops the dark ones: alpha follows luma, ramping over 0.6–0.7 (above
// the sky band, below the sun).
func silhouette(img *image.NRGBA) *image.NRGBA {
	out := image.NewNRGBA(img.Bounds())
	for i := 0; i < len(img.Pix); i += 4 {
		r, g, b, a := float64(img.Pix[i]), float64(img.Pix[i+1]), float64(img.Pix[i+2]), float64(img.Pix[i+3])
		luma := (0.2126*r + 0.7152*g + 0.0722*b) / 255
		k := min(max((luma-0.6)/0.1, 0), 1)
		out.Pix[i], out.Pix[i+1], out.Pix[i+2] = 0xff, 0xff, 0xff
		out.Pix[i+3] = uint8(k*a + 0.5)
	}
	return out
}

func decodePNG(path string) (image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return png.Decode(f)
}

// staleFiles lists the outputs whose committed copy is missing or differs.
// PNGs are compared by pixel, within a step of rounding, not by bytes: the
// encoder's compression may differ between Go releases, and the rasteriser's
// last bit between CPUs.
func staleFiles(root string, files []output) ([]string, error) {
	var stale []string
	for _, f := range files {
		have, err := os.ReadFile(filepath.Join(root, f.path))
		if os.IsNotExist(err) {
			stale = append(stale, f.path)
			continue
		}
		if err != nil {
			return nil, err
		}
		if bytes.Equal(have, f.data) {
			continue
		}
		if !f.png || !samePixels(have, f.data) {
			stale = append(stale, f.path)
		}
	}
	return stale, nil
}

func samePixels(a, b []byte) bool {
	ia, err1 := png.Decode(bytes.NewReader(a))
	ib, err2 := png.Decode(bytes.NewReader(b))
	if err1 != nil || err2 != nil || ia.Bounds() != ib.Bounds() {
		return false
	}
	for y := ia.Bounds().Min.Y; y < ia.Bounds().Max.Y; y++ {
		for x := ia.Bounds().Min.X; x < ia.Bounds().Max.X; x++ {
			r1, g1, b1, a1 := ia.At(x, y).RGBA()
			r2, g2, b2, a2 := ib.At(x, y).RGBA()
			for _, d := range []int{int(r1) - int(r2), int(g1) - int(g2), int(b1) - int(b2), int(a1) - int(a2)} {
				if d > 0x0202 || d < -0x0202 {
					return false
				}
			}
		}
	}
	return true
}
