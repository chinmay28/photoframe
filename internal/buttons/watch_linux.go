//go:build linux

package buttons

import (
	"time"

	"github.com/warthog618/go-gpiocdev"
)

// Watch calls onPress with the button index (0 = A ... 3 = D) on each
// debounced press. Buttons are active-low with pull-ups. Close the returned
// function to stop.
func Watch(chip string, onPress func(index int)) (func() error, error) {
	d := &Debouncer{Window: 300 * time.Millisecond}
	index := map[int]int{}
	offsets := make([]int, len(Pins))
	for i, p := range Pins {
		index[p] = i
		offsets[i] = p
	}
	lines, err := gpiocdev.RequestLines(chip, offsets,
		gpiocdev.AsInput,
		gpiocdev.WithPullUp,
		gpiocdev.WithFallingEdge,
		gpiocdev.WithDebounce(20*time.Millisecond),
		gpiocdev.WithConsumer("photoframe-buttons"),
		gpiocdev.WithEventHandler(func(e gpiocdev.LineEvent) {
			if i, ok := index[e.Offset]; ok && d.Accept(i) {
				onPress(i)
			}
		}),
	)
	if err != nil {
		return nil, err
	}
	return lines.Close, nil
}
