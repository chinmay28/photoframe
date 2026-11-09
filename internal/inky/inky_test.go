package inky

import (
	"bytes"
	"context"
	"errors"
	"image"
	"testing"
	"time"
)

// fakePanel records traffic and decodes it into commands.
type fakePanel struct {
	cs, dc, reset bool
	busyHigh      []bool // successive Busy reads; last value repeats
	reads         int

	cmds []recorded
}

type recorded struct {
	cmd  byte
	data []byte
}

type pin struct {
	set func(bool)
}

func (p pin) Set(v bool) error { p.set(v); return nil }

func (f *fakePanel) Write(b []byte) (int, error) {
	if f.cs {
		return 0, errors.New("SPI write with CS high")
	}
	if !f.dc {
		for _, c := range b {
			f.cmds = append(f.cmds, recorded{cmd: c})
		}
		return len(b), nil
	}
	last := &f.cmds[len(f.cmds)-1]
	last.data = append(last.data, b...)
	return len(b), nil
}

func (f *fakePanel) High() (bool, error) {
	i := min(f.reads, len(f.busyHigh)-1)
	f.reads++
	return f.busyHigh[i], nil
}

func (f *fakePanel) hardware() Hardware {
	return Hardware{
		SPI:   f,
		CS:    pin{func(v bool) { f.cs = v }},
		DC:    pin{func(v bool) { f.dc = v }},
		Reset: pin{func(v bool) { f.reset = v }},
		Busy:  f,
	}
}

func newTestDevice(f *fakePanel, m *Model) (*Device, *[]time.Duration) {
	d := New(f.hardware(), m)
	var slept []time.Duration
	d.sleep = func(ctx context.Context, dur time.Duration) error {
		slept = append(slept, dur)
		return ctx.Err()
	}
	return d, &slept
}

func testImage(m *Model) *image.Paletted {
	img := image.NewPaletted(image.Rect(0, 0, Width, Height), m.Palette)
	img.Pix[0] = 4 // blue -> native 5 on Spectra 6
	img.Pix[1] = 5 // green -> native 6
	return img
}

func TestSpectra6ShowSequence(t *testing.T) {
	f := &fakePanel{busyHigh: []bool{true}}
	d, _ := newTestDevice(f, Spectra6)
	// Every wait sees "busy" once, then "idle".
	toggle := false
	d.hw.Busy = inputFunc(func() bool { toggle = !toggle; return !toggle })

	if err := d.Show(context.Background(), testImage(Spectra6)); err != nil {
		t.Fatal(err)
	}

	var got []byte
	for _, c := range f.cmds {
		got = append(got, c.cmd)
	}
	want := []byte{0xAA, 0x01, 0x00, 0x05, 0x08, 0x06, 0x03, 0x60, 0x30, 0x50, 0x61, 0xE3, 0x82, 0x10, 0x04, 0x06, 0x12, 0x02, 0x00}
	if !bytes.Equal(got, want) {
		t.Fatalf("command sequence\n got % X\nwant % X", got, want)
	}

	dtm := f.cmds[13]
	if len(dtm.data) != Width*Height/2 {
		t.Fatalf("DTM carried %d bytes, want %d", len(dtm.data), Width*Height/2)
	}
	if dtm.data[0] != 0x56 {
		t.Fatalf("first packed byte = %#x, want 0x56 (blue, green native codes)", dtm.data[0])
	}
	if !f.cs {
		t.Fatal("CS left asserted")
	}
	if !f.reset {
		t.Fatal("reset left low")
	}
}

func TestACeP7UsesAllSevenCodes(t *testing.T) {
	if len(ACeP7.Palette) != 7 || len(ACeP7.Native) != 7 {
		t.Fatal("ACeP7 should have 7 inks")
	}
	f := &fakePanel{busyHigh: []bool{false, true}}
	d, _ := newTestDevice(f, ACeP7)
	toggle := false
	d.hw.Busy = inputFunc(func() bool { toggle = !toggle; return !toggle })
	img := image.NewPaletted(image.Rect(0, 0, Width, Height), ACeP7.Palette)
	img.Pix[0], img.Pix[1] = 6, 1
	if err := d.Show(context.Background(), img); err != nil {
		t.Fatal(err)
	}
	for _, c := range f.cmds {
		if c.cmd == 0x10 && c.data[0] != 0x61 {
			t.Fatalf("packed byte %#x, want 0x61", c.data[0])
		}
	}
}

func TestBusyHeldHighSleepsFullTimeout(t *testing.T) {
	f := &fakePanel{busyHigh: []bool{true}}
	d, slept := newTestDevice(f, Spectra6)
	if err := d.busyWait(context.Background(), 32*time.Second); err != nil {
		t.Fatal(err)
	}
	if len(*slept) != 1 || (*slept)[0] != 32*time.Second {
		t.Fatalf("slept %v, want one 32s sleep", *slept)
	}
}

func TestShowRejectsWrongSize(t *testing.T) {
	f := &fakePanel{busyHigh: []bool{true}}
	d, _ := newTestDevice(f, Spectra6)
	img := image.NewPaletted(image.Rect(0, 0, 10, 10), Spectra6.Palette)
	if err := d.Show(context.Background(), img); err == nil {
		t.Fatal("want size error")
	}
	if len(f.cmds) != 0 {
		t.Fatal("nothing should be sent for an invalid image")
	}
}

func TestShowCancelledPowersOff(t *testing.T) {
	f := &fakePanel{busyHigh: []bool{false}} // stays busy
	d, _ := newTestDevice(f, Spectra6)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := d.Show(ctx, testImage(Spectra6))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if last := f.cmds[len(f.cmds)-1]; last.cmd != 0x02 {
		t.Fatalf("last command %#x, want POF (0x02)", last.cmd)
	}
}

func TestLargeDataIsChunked(t *testing.T) {
	var sizes []int
	f := &fakePanel{busyHigh: []bool{true}}
	d, _ := newTestDevice(f, Spectra6)
	d.hw.SPI = writerFunc(func(b []byte) { sizes = append(sizes, len(b)) })
	if err := d.command(0x10, make([]byte, 10000)); err != nil {
		t.Fatal(err)
	}
	if len(sizes) != 4 || sizes[1] != spiChunk || sizes[3] != 10000-2*spiChunk {
		t.Fatalf("write sizes %v", sizes)
	}
}

type inputFunc func() bool

func (f inputFunc) High() (bool, error) { return f(), nil }

type writerFunc func([]byte)

func (f writerFunc) Write(b []byte) (int, error) { f(b); return len(b), nil }
