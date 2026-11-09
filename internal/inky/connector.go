package inky

import (
	"context"
	"fmt"
	"image"
	"image/color"
	"sync"
	"time"
)

// NotConnectedError reports that the panel's hardware could not be opened,
// usually because no panel is attached yet or SPI is not enabled.
type NotConnectedError struct{ Err error }

func (e *NotConnectedError) Error() string { return "panel not connected: " + e.Err.Error() }
func (e *NotConnectedError) Unwrap() error { return e.Err }

// Disconnected marks the error as "no panel" rather than a failed refresh,
// so callers can tell the two apart without importing this package.
func (e *NotConnectedError) Disconnected() bool { return true }

// Opener opens the panel hardware, returning a function that releases it.
type Opener func() (Hardware, func() error, error)

// Connector is a panel that may not be attached yet. It opens the hardware
// on first use and, while that fails, tries again at most once per Retry.
// Until it succeeds, Show returns a *NotConnectedError straight away.
type Connector struct {
	open  Opener
	retry time.Duration
	now   func() time.Time

	mu        sync.Mutex
	model     *Model
	dev       *Device
	closeHW   func() error
	lastTry   time.Time
	lastErr   error
	newDevice func(Hardware, *Model) *Device
}

// NewConnector returns a connector that has not tried to open the panel yet.
func NewConnector(open Opener, m *Model, retry time.Duration) *Connector {
	return &Connector{open: open, retry: retry, now: time.Now, model: m, newDevice: New}
}

// Connect tries to open the panel now if it is not open, and reports the
// result. It respects the retry interval.
func (c *Connector) Connect() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	_, err := c.deviceLocked()
	return err
}

func (c *Connector) deviceLocked() (*Device, error) {
	if c.dev != nil {
		return c.dev, nil
	}
	if !c.lastTry.IsZero() && c.now().Sub(c.lastTry) < c.retry {
		return nil, &NotConnectedError{Err: c.lastErr}
	}
	c.lastTry = c.now()
	hw, closeHW, err := c.open()
	if err != nil {
		c.lastErr = err
		return nil, &NotConnectedError{Err: err}
	}
	c.dev, c.closeHW, c.lastErr = c.newDevice(hw, c.model), closeHW, nil
	return c.dev, nil
}

// Connected reports whether the hardware is open.
func (c *Connector) Connected() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.dev != nil
}

// SetModel switches the panel variant.
func (c *Connector) SetModel(m *Model) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.model = m
	if c.dev != nil {
		c.dev.SetModel(m)
	}
}

// Size implements player.Display.
func (c *Connector) Size() image.Point { return image.Pt(Width, Height) }

// Palette implements player.Display.
func (c *Connector) Palette() color.Palette {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.model.Palette
}

// Show implements player.Display. Connecting is attempted first; the refresh
// itself runs without holding the connector's lock.
func (c *Connector) Show(ctx context.Context, img *image.Paletted) error {
	c.mu.Lock()
	dev, err := c.deviceLocked()
	c.mu.Unlock()
	if err != nil {
		return err
	}
	return dev.Show(ctx, img)
}

// Close releases the hardware if it was opened.
func (c *Connector) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closeHW == nil {
		return nil
	}
	err := c.closeHW()
	c.dev, c.closeHW = nil, nil
	if err != nil {
		return fmt.Errorf("inky: close: %w", err)
	}
	return nil
}
