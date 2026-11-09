// Package devdisplay is a stand-in for the e-ink panel that writes each frame
// to a PNG file, for developing without the hardware.
package devdisplay

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"sync"
	"time"

	"github.com/chinmay28/photoframe/internal/config"
	"github.com/chinmay28/photoframe/internal/inky"
)

// PNG writes frames to Path. Delay simulates the panel's refresh time.
type PNG struct {
	Path  string
	Delay time.Duration

	mu    sync.Mutex
	model *inky.Model
}

// New returns a PNG display using the given panel model's palette.
func New(path string, m *inky.Model, delay time.Duration) *PNG {
	return &PNG{Path: path, Delay: delay, model: m}
}

// SetModel switches palette.
func (p *PNG) SetModel(m *inky.Model) {
	p.mu.Lock()
	p.model = m
	p.mu.Unlock()
}

// Size implements player.Display.
func (p *PNG) Size() image.Point { return image.Pt(inky.Width, inky.Height) }

// Palette implements player.Display.
func (p *PNG) Palette() color.Palette {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.model.Palette
}

// Show implements player.Display.
func (p *PNG) Show(ctx context.Context, img *image.Paletted) error {
	if p.Delay > 0 {
		t := time.NewTimer(p.Delay)
		defer t.Stop()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-t.C:
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return err
	}
	return config.WriteFileAtomic(p.Path, buf.Bytes(), 0o644)
}
