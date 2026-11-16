package main

import (
	"bytes"
	"image/png"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
)

const repoRoot = "../.."

var planned = sync.OnceValues(func() ([]output, error) { return plan(repoRoot) })

// TestCommittedIconsMatchTheArtwork fails when web/icons/icon.svg (or a
// developer badge) changes without a `make icons`, or when someone
// hand-edits a derived icon.
func TestCommittedIconsMatchTheArtwork(t *testing.T) {
	files, err := planned()
	if err != nil {
		t.Fatal(err)
	}
	stale, err := staleFiles(repoRoot, files)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range stale {
		t.Errorf("%s does not match %s — run: make icons", f, artPath)
	}
}

func TestLauncherIconsAreTheirDensitysSize(t *testing.T) {
	files, err := planned()
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, f := range files {
		for _, d := range densities {
			if !strings.Contains(f.path, "/mipmap-"+d.name+"/") {
				continue
			}
			n++
			cfg, err := png.DecodeConfig(bytes.NewReader(f.data))
			if err != nil {
				t.Fatalf("%s: %v", f.path, err)
			}
			dp := 48
			if strings.HasSuffix(f.path, "_foreground.png") || strings.HasSuffix(f.path, "_monochrome.png") {
				dp = 108
			}
			if want := dp * d.scale / 4; cfg.Width != want || cfg.Height != want {
				t.Errorf("%s is %dx%d, want %d", f.path, cfg.Width, cfg.Height, want)
			}
		}
	}
	if n != 3*len(densities) {
		t.Errorf("%d launcher icons, want %d", n, 3*len(densities))
	}
}

// The tile's corners are transparent on the round icon, and the adaptive
// foreground leaves the tile out entirely (the launcher draws the
// background colour and its own mask).
func TestShapes(t *testing.T) {
	svg, err := os.ReadFile(filepath.Join(repoRoot, artPath))
	if err != nil {
		t.Fatal(err)
	}
	art, err := parseSVG(svg)
	if err != nil {
		t.Fatal(err)
	}
	icon := art.render(192, 192, 0, false)
	if a := icon.NRGBAAt(0, 0).A; a != 0 {
		t.Errorf("icon corner alpha %d, want 0", a)
	}
	if got, want := icon.NRGBAAt(96, 4), art.plate(); got != want {
		t.Errorf("icon top edge %v, want the plate %v", got, want)
	}
	fg := art.render(432, 288, 72, true)
	if a := fg.NRGBAAt(216, 100).A; a != 0 {
		t.Errorf("foreground above the frame has alpha %d, want transparent", a)
	}
	if a := fg.NRGBAAt(216, 216).A; a != 0xff {
		t.Errorf("foreground centre has alpha %d, want the scene", a)
	}
	if p := art.plate(); p.R != 0x2A || p.G != 0x27 || p.B != 0x24 {
		t.Errorf("plate %v, want #2A2724 from icon.svg", p)
	}
}

func TestParseRefusesWhatItCannotDraw(t *testing.T) {
	for _, bad := range []string{
		`<svg viewBox="0 0 512 256"><rect width="1" height="1" fill="#000000"/></svg>`,
		`<svg viewBox="0 0 512 512"><ellipse fill="#000000"/></svg>`,
		`<svg viewBox="0 0 512 512"><path d="M0 0 a1 1 0 0 0 2 2" fill="#000000"/></svg>`,
		`<svg viewBox="0 0 512 512"><rect width="1" height="1" fill="red"/></svg>`,
		`<svg viewBox="0 0 512 512"></svg>`,
	} {
		if _, err := parseSVG([]byte(bad)); err == nil {
			t.Errorf("parsed %s", bad)
		}
	}
}

// Every @drawable/, @mipmap/, R.drawable. and R.mipmap. reference in the
// app names a file that exists, so retiring an icon can't leave a reference
// only a Gradle build would notice.
func TestEveryIconReferenceExists(t *testing.T) {
	resDir := filepath.Join(repoRoot, res)
	have := map[string]bool{}
	filepath.WalkDir(resDir, func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			dir := filepath.Base(filepath.Dir(p))
			kind := strings.SplitN(dir, "-", 2)[0]
			have[kind+"/"+strings.TrimSuffix(d.Name(), filepath.Ext(d.Name()))] = true
		}
		return nil
	})
	ref := regexp.MustCompile(`(?:@|R\.)(drawable|mipmap)[/.]([a-z0-9_]+)`)
	filepath.WalkDir(filepath.Join(repoRoot, "apps/android/app/src/main"), func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !(strings.HasSuffix(p, ".xml") || strings.HasSuffix(p, ".kt")) {
			return nil
		}
		b, _ := os.ReadFile(p)
		for _, m := range ref.FindAllStringSubmatch(string(b), -1) {
			if !have[m[1]+"/"+m[2]] {
				t.Errorf("%s refers to %s/%s, which does not exist", p, m[1], m[2])
			}
		}
		return nil
	})
}
