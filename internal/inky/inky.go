// Package inky drives Pimoroni Inky Impression 7.3" e-ink panels over SPI.
//
// The command sequences are ported from Pimoroni's MIT-licensed Python
// library (github.com/pimoroni/inky: inky_e673.py and inky_ac073tc1a.py).
// Hardware access goes through the small Pin/Input/io.Writer interfaces so
// the protocol can be tested without a Raspberry Pi.
package inky

import (
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	"io"
	"sync"
	"time"

	"github.com/chinmay28/photoframe/internal/config"
)

// Width and Height are the native panel resolution.
const (
	Width  = 800
	Height = 480
)

// spiChunk is the largest single SPI write; spidev's default buffer is 4 KiB.
const spiChunk = 4096

// Pin is a GPIO output.
type Pin interface{ Set(high bool) error }

// Input is a GPIO input.
type Input interface{ High() (bool, error) }

// Hardware bundles the connections to the panel.
type Hardware struct {
	SPI   io.Writer // chip-select is driven manually via CS
	CS    Pin       // active low
	DC    Pin       // low = command, high = data
	Reset Pin       // active low
	Busy  Input     // low while the panel is busy
}

// Model describes one panel variant.
type Model struct {
	Name config.Panel
	// Palette holds the colours the panel actually shows, used for dithering
	// and previews. Index i is sent to the panel as Native[i].
	Palette color.Palette
	Native  []byte

	init    func(d *Device) error
	refresh func(ctx context.Context, d *Device) error
}

// ModelFor returns the model for a configured panel.
func ModelFor(p config.Panel) (*Model, error) {
	switch p {
	case config.PanelSpectra6:
		return Spectra6, nil
	case config.PanelACeP7:
		return ACeP7, nil
	}
	return nil, fmt.Errorf("inky: unknown panel %q", p)
}

// Device is an Inky panel. Show calls are serialised internally.
type Device struct {
	hw    Hardware
	sleep func(context.Context, time.Duration) error
	poll  time.Duration

	mu    sync.Mutex
	model *Model
}

// New returns a device driving the given hardware.
func New(hw Hardware, m *Model) *Device {
	return &Device{hw: hw, model: m, sleep: sleepCtx, poll: 10 * time.Millisecond}
}

// SetModel switches the panel variant used by subsequent Show calls.
func (d *Device) SetModel(m *Model) {
	d.mu.Lock()
	d.model = m
	d.mu.Unlock()
}

// Size returns the native panel size.
func (d *Device) Size() image.Point { return image.Pt(Width, Height) }

// Palette returns the current model's palette.
func (d *Device) Palette() color.Palette {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.model.Palette
}

// Show sends img to the panel and blocks until the refresh completes
// (roughly 30-40 s). img must be Width×Height and use Palette().
func (d *Device) Show(ctx context.Context, img *image.Paletted) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	buf, err := pack(img, d.model)
	if err != nil {
		return err
	}
	if err := d.model.init(d); err != nil {
		return fmt.Errorf("inky: init: %w", err)
	}
	if err := d.command(0x10, buf); err != nil { // DTM: data start transmission
		return fmt.Errorf("inky: send image: %w", err)
	}
	if err := d.model.refresh(ctx, d); err != nil {
		// Best effort: never leave the panel's high voltage on.
		_ = d.command(0x02, []byte{0x00})
		return fmt.Errorf("inky: refresh: %w", err)
	}
	return nil
}

// pack converts img to 4 bits per pixel in panel colour codes, two pixels
// per byte, high nibble first.
func pack(img *image.Paletted, m *Model) ([]byte, error) {
	b := img.Bounds()
	if b.Dx() != Width || b.Dy() != Height {
		return nil, fmt.Errorf("inky: image is %dx%d, want %dx%d", b.Dx(), b.Dy(), Width, Height)
	}
	if len(img.Palette) != len(m.Palette) {
		return nil, fmt.Errorf("inky: image has %d colours, panel %s has %d", len(img.Palette), m.Name, len(m.Palette))
	}
	buf := make([]byte, Width*Height/2)
	for y := 0; y < Height; y++ {
		row := img.Pix[y*img.Stride : y*img.Stride+Width]
		for x := 0; x < Width; x += 2 {
			a, c := row[x], row[x+1]
			if int(a) >= len(m.Native) || int(c) >= len(m.Native) {
				return nil, errors.New("inky: pixel index outside palette")
			}
			buf[(y*Width+x)/2] = m.Native[a]<<4 | m.Native[c]&0x0F
		}
	}
	return buf, nil
}

func (d *Device) reset(low, high time.Duration, pulses int) error {
	for i := 0; i < pulses; i++ {
		if err := d.hw.Reset.Set(false); err != nil {
			return err
		}
		if err := d.sleep(context.Background(), low); err != nil {
			return err
		}
		if err := d.hw.Reset.Set(true); err != nil {
			return err
		}
		if err := d.sleep(context.Background(), high); err != nil {
			return err
		}
	}
	return nil
}

// command sends a command byte followed by optional data, framed by CS.
func (d *Device) command(cmd byte, data []byte) error {
	if err := d.hw.CS.Set(false); err != nil {
		return err
	}
	defer d.hw.CS.Set(true)
	if err := d.hw.DC.Set(false); err != nil {
		return err
	}
	if _, err := d.hw.SPI.Write([]byte{cmd}); err != nil {
		return err
	}
	if len(data) > 0 {
		if err := d.hw.DC.Set(true); err != nil {
			return err
		}
		for off := 0; off < len(data); off += spiChunk {
			end := min(off+spiChunk, len(data))
			if _, err := d.hw.SPI.Write(data[off:end]); err != nil {
				return err
			}
		}
	}
	return d.hw.DC.Set(false)
}

// busyWait waits for the busy line to go high (idle). As in Pimoroni's
// driver, a line that is already high is treated as "no busy signal wired"
// and the full timeout is slept instead.
func (d *Device) busyWait(ctx context.Context, timeout time.Duration) error {
	high, err := d.hw.Busy.High()
	if err != nil {
		return err
	}
	if high {
		return d.sleep(ctx, timeout)
	}
	deadline := time.Now().Add(timeout)
	for {
		high, err := d.hw.Busy.High()
		if err != nil {
			return err
		}
		if high {
			return nil
		}
		if time.Now().After(deadline) {
			return nil // the panel usually finishes regardless; match upstream and carry on
		}
		if err := d.sleep(ctx, d.poll); err != nil {
			return err
		}
	}
}

func sleepCtx(ctx context.Context, dur time.Duration) error {
	t := time.NewTimer(dur)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// seq runs commands in order, stopping at the first error.
func (d *Device) seq(cmds ...[]byte) error {
	for _, c := range cmds {
		if err := d.command(c[0], c[1:]); err != nil {
			return err
		}
	}
	return nil
}

func rgb(r, g, b uint8) color.RGBA { return color.RGBA{r, g, b, 255} }

// blend mixes the panel's measured ("saturated") and ideal colours half and
// half, matching Pimoroni's default saturation of 0.5. Dithering against the
// measured colours alone makes photos look flat.
func blend(measured, ideal color.RGBA) color.RGBA {
	return rgb(uint8((int(measured.R)+int(ideal.R))/2), uint8((int(measured.G)+int(ideal.G))/2), uint8((int(measured.B)+int(ideal.B))/2))
}
