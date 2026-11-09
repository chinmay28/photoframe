package inky

import (
	"context"
	"errors"
	"image"
	"testing"
	"time"
)

func TestConnectorRetriesUntilPanelAppears(t *testing.T) {
	now := time.Unix(0, 0)
	attempts := 0
	present := false
	f := &fakePanel{busyHigh: []bool{true}}
	c := NewConnector(func() (Hardware, func() error, error) {
		attempts++
		if !present {
			return Hardware{}, nil, errors.New("open /dev/spidev0.0: no such file or directory")
		}
		return f.hardware(), func() error { return nil }, nil
	}, Spectra6, time.Minute)
	c.now = func() time.Time { return now }
	c.newDevice = func(hw Hardware, m *Model) *Device {
		d := New(hw, m)
		d.sleep = func(context.Context, time.Duration) error { return nil }
		return d
	}
	img := image.NewPaletted(image.Rect(0, 0, Width, Height), Spectra6.Palette)

	err := c.Show(context.Background(), img)
	var nc *NotConnectedError
	if !errors.As(err, &nc) || !nc.Disconnected() {
		t.Fatalf("Show without panel = %v, want NotConnectedError", err)
	}
	// Within the retry interval the hardware is not probed again.
	c.Show(context.Background(), img)
	if attempts != 1 {
		t.Fatalf("attempts = %d, want 1 within the retry interval", attempts)
	}

	present = true
	now = now.Add(time.Minute)
	if err := c.Show(context.Background(), img); err != nil {
		t.Fatalf("Show after panel appeared: %v", err)
	}
	if !c.Connected() || attempts != 2 {
		t.Fatalf("connected=%v attempts=%d", c.Connected(), attempts)
	}
	if len(f.cmds) == 0 {
		t.Fatal("nothing was sent to the panel")
	}
	if err := c.Close(); err != nil || c.Connected() {
		t.Fatalf("Close: %v connected=%v", err, c.Connected())
	}
}

func TestConnectorSetModelBeforeConnect(t *testing.T) {
	c := NewConnector(func() (Hardware, func() error, error) {
		return Hardware{}, nil, errors.New("absent")
	}, Spectra6, time.Minute)
	c.SetModel(ACeP7)
	if len(c.Palette()) != 7 {
		t.Fatal("palette did not follow SetModel")
	}
}
