//go:build !linux

package inky

import "errors"

// OpenLinux is only available on Linux; use the PNG display elsewhere.
func OpenLinux(spiPath, gpioChip string) (Hardware, func() error, error) {
	return Hardware{}, nil, errors.New("inky: hardware access requires Linux")
}
