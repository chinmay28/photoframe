//go:build !linux

package buttons

import "errors"

// Watch is only supported on Linux.
func Watch(chip string, onPress func(index int)) (func() error, error) {
	return nil, errors.New("buttons: GPIO requires Linux")
}
