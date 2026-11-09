//go:build linux

package inky

import (
	"errors"
	"fmt"
	"os"
	"unsafe"

	"github.com/warthog618/go-gpiocdev"
	"golang.org/x/sys/unix"
)

// Default BCM pin numbers on the Inky Impression HAT.
const (
	PinReset = 27
	PinBusy  = 17
	PinDC    = 22
	PinCS    = 8 // CE0, driven as a GPIO; requires dtoverlay=spi0-0cs
)

// spidev ioctls: _IOW('k', nr, size).
const (
	spiIocWrMode       = 0x40016b01
	spiIocWrMaxSpeedHz = 0x40046b04
	spiNoCS            = 0x40
	spiSpeedHz         = 5_000_000
)

// OpenLinux opens the SPI device (e.g. /dev/spidev0.0) and GPIO chip (e.g.
// gpiochip0) and returns the hardware handle plus a function to release it.
func OpenLinux(spiPath, gpioChip string) (Hardware, func() error, error) {
	spi, err := openSPI(spiPath)
	if err != nil {
		return Hardware{}, nil, err
	}
	closers := []func() error{spi.Close}
	closeAll := func() error {
		var errs []error
		for i := len(closers) - 1; i >= 0; i-- {
			errs = append(errs, closers[i]())
		}
		return errors.Join(errs...)
	}

	out := func(offset, initial int) (*gpiocdev.Line, error) {
		l, err := gpiocdev.RequestLine(gpioChip, offset, gpiocdev.AsOutput(initial), gpiocdev.WithConsumer("photoframe"))
		if err != nil {
			return nil, fmt.Errorf("gpio %d: %w", offset, err)
		}
		closers = append(closers, l.Close)
		return l, nil
	}
	cs, err := out(PinCS, 1)
	if err != nil {
		closeAll()
		return Hardware{}, nil, fmt.Errorf("%w (is dtoverlay=spi0-0cs set in config.txt?)", err)
	}
	dc, err := out(PinDC, 0)
	if err != nil {
		closeAll()
		return Hardware{}, nil, err
	}
	reset, err := out(PinReset, 1)
	if err != nil {
		closeAll()
		return Hardware{}, nil, err
	}
	busy, err := gpiocdev.RequestLine(gpioChip, PinBusy, gpiocdev.AsInput, gpiocdev.WithPullUp, gpiocdev.WithConsumer("photoframe"))
	if err != nil {
		closeAll()
		return Hardware{}, nil, fmt.Errorf("gpio %d: %w", PinBusy, err)
	}
	closers = append(closers, busy.Close)

	return Hardware{
		SPI:   spi,
		CS:    outPin{cs},
		DC:    outPin{dc},
		Reset: outPin{reset},
		Busy:  inPin{busy},
	}, closeAll, nil
}

func openSPI(path string) (*os.File, error) {
	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return nil, fmt.Errorf("%w (is SPI enabled?)", err)
	}
	mode := uint8(spiNoCS) // SPI mode 0, chip-select handled by us
	if err := ioctl(f, spiIocWrMode, unsafe.Pointer(&mode)); err != nil {
		mode = 0 // some kernels refuse NO_CS; with spi0-0cs there is no CS pin anyway
		if err := ioctl(f, spiIocWrMode, unsafe.Pointer(&mode)); err != nil {
			f.Close()
			return nil, fmt.Errorf("spi mode: %w", err)
		}
	}
	speed := uint32(spiSpeedHz)
	if err := ioctl(f, spiIocWrMaxSpeedHz, unsafe.Pointer(&speed)); err != nil {
		f.Close()
		return nil, fmt.Errorf("spi speed: %w", err)
	}
	return f, nil
}

func ioctl(f *os.File, req uintptr, arg unsafe.Pointer) error {
	_, _, errno := unix.Syscall(unix.SYS_IOCTL, f.Fd(), req, uintptr(arg))
	if errno != 0 {
		return errno
	}
	return nil
}

type outPin struct{ l *gpiocdev.Line }

func (p outPin) Set(high bool) error {
	v := 0
	if high {
		v = 1
	}
	return p.l.SetValue(v)
}

type inPin struct{ l *gpiocdev.Line }

func (p inPin) High() (bool, error) {
	v, err := p.l.Value()
	return v == 1, err
}
