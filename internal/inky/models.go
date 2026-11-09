package inky

import (
	"context"
	"image/color"
	"time"

	"github.com/chinmay28/photoframe/internal/config"
)

// Spectra6 is the Inky Impression 7.3" (2025 edition) with an E Ink
// Spectra 6 panel. Port of inky_e673.py.
var Spectra6 = &Model{
	Name: config.PanelSpectra6,
	Palette: color.Palette{
		blend(rgb(0, 0, 0), rgb(0, 0, 0)),             // black
		blend(rgb(161, 164, 165), rgb(255, 255, 255)), // white
		blend(rgb(208, 190, 71), rgb(255, 255, 0)),    // yellow
		blend(rgb(156, 72, 75), rgb(255, 0, 0)),       // red
		blend(rgb(61, 59, 94), rgb(0, 0, 255)),        // blue
		blend(rgb(58, 91, 70), rgb(0, 255, 0)),        // green
	},
	// Panel colour codes; code 4 is unused by this controller.
	Native: []byte{0, 1, 2, 3, 5, 6},

	init: func(d *Device) error {
		if err := d.reset(30*time.Millisecond, 30*time.Millisecond, 1); err != nil {
			return err
		}
		if err := d.busyWait(context.Background(), 300*time.Millisecond); err != nil {
			return err
		}
		return d.seq(
			[]byte{0xAA, 0x49, 0x55, 0x20, 0x08, 0x09, 0x18},
			[]byte{0x01, 0x3F},                   // PWR
			[]byte{0x00, 0x5F, 0x69},             // PSR
			[]byte{0x05, 0x40, 0x1F, 0x1F, 0x2C}, // BTST1
			[]byte{0x08, 0x6F, 0x1F, 0x1F, 0x22}, // BTST3
			[]byte{0x06, 0x6F, 0x1F, 0x17, 0x17}, // BTST2
			[]byte{0x03, 0x00, 0x54, 0x00, 0x44}, // POFS
			[]byte{0x60, 0x02, 0x00},             // TCON
			[]byte{0x30, 0x08},                   // PLL: dynamic frame rate (best colour)
			[]byte{0x50, 0x3F},                   // CDI
			[]byte{0x61, 0x03, 0x20, 0x01, 0xE0}, // TRES 800x480
			[]byte{0xE3, 0x2F},                   // PWS
			[]byte{0x82, 0x01},                   // VDCS
		)
	},
	refresh: func(ctx context.Context, d *Device) error {
		if err := d.command(0x04, nil); err != nil { // PON
			return err
		}
		if err := d.busyWait(ctx, 300*time.Millisecond); err != nil {
			return err
		}
		if err := d.command(0x06, []byte{0x6F, 0x1F, 0x17, 0x49}); err != nil { // BTST2, second setting
			return err
		}
		if err := d.command(0x12, []byte{0x00}); err != nil { // DRF
			return err
		}
		if err := d.busyWait(ctx, 32*time.Second); err != nil {
			return err
		}
		if err := d.command(0x02, []byte{0x00}); err != nil { // POF
			return err
		}
		if err := d.busyWait(ctx, 300*time.Millisecond); err != nil {
			return err
		}
		if err := d.command(0x00, []byte{0x4F, 0x6E}); err != nil { // PSR
			return err
		}
		return d.busyWait(ctx, 300*time.Millisecond)
	},
}

// ACeP7 is the original Inky Impression 7.3" with a 7-colour ACeP panel.
// Port of inky_ac073tc1a.py.
var ACeP7 = &Model{
	Name: config.PanelACeP7,
	Palette: color.Palette{
		blend(rgb(0, 0, 0), rgb(0, 0, 0)),             // black
		blend(rgb(217, 242, 255), rgb(255, 255, 255)), // white
		blend(rgb(3, 124, 76), rgb(0, 255, 0)),        // green
		blend(rgb(27, 46, 198), rgb(0, 0, 255)),       // blue
		blend(rgb(245, 80, 34), rgb(255, 0, 0)),       // red
		blend(rgb(255, 255, 68), rgb(255, 255, 0)),    // yellow
		blend(rgb(239, 121, 44), rgb(255, 140, 0)),    // orange
	},
	Native: []byte{0, 1, 2, 3, 4, 5, 6},

	init: func(d *Device) error {
		if err := d.reset(100*time.Millisecond, 100*time.Millisecond, 2); err != nil {
			return err
		}
		if err := d.busyWait(context.Background(), time.Second); err != nil {
			return err
		}
		const border = 1 // white
		return d.seq(
			[]byte{0xAA, 0x49, 0x55, 0x20, 0x08, 0x09, 0x18}, // CMDH
			[]byte{0x01, 0x3F, 0x00, 0x32, 0x2A, 0x0E, 0x2A}, // PWR
			[]byte{0x00, 0x5F, 0x69},                         // PSR
			[]byte{0x03, 0x00, 0x54, 0x00, 0x44},             // POFS
			[]byte{0x05, 0x40, 0x1F, 0x1F, 0x2C},             // BTST1
			[]byte{0x06, 0x6F, 0x1F, 0x16, 0x25},             // BTST2
			[]byte{0x08, 0x6F, 0x1F, 0x1F, 0x22},             // BTST3
			[]byte{0x13, 0x00, 0x04},                         // IPC
			[]byte{0x30, 0x02},                               // PLL
			[]byte{0x41, 0x00},                               // TSE
			[]byte{0x50, border<<5 | 0x17},                   // CDI
			[]byte{0x60, 0x02, 0x00},                         // TCON
			[]byte{0x61, 0x03, 0x20, 0x01, 0xE0},             // TRES 800x480
			[]byte{0x82, 0x1E},                               // VDCS
			[]byte{0x84, 0x00},                               // T_VDCS
			[]byte{0x86, 0x00},                               // AGID
			[]byte{0xE3, 0x2F},                               // PWS
			[]byte{0xE0, 0x00},                               // CCSET
			[]byte{0xE6, 0x00},                               // TSSET
		)
	},
	refresh: func(ctx context.Context, d *Device) error {
		if err := d.command(0x04, nil); err != nil { // PON
			return err
		}
		if err := d.busyWait(ctx, 400*time.Millisecond); err != nil {
			return err
		}
		if err := d.command(0x12, []byte{0x00}); err != nil { // DRF
			return err
		}
		if err := d.busyWait(ctx, 45*time.Second); err != nil {
			return err
		}
		if err := d.command(0x02, []byte{0x00}); err != nil { // POF
			return err
		}
		return d.busyWait(ctx, 400*time.Millisecond)
	},
}
