// Package buttons watches the Inky Impression's four buttons (A-D).
package buttons

import (
	"sync"
	"time"
)

// Pins are the BCM GPIO numbers of buttons A, B, C and D.
var Pins = [4]int{5, 6, 16, 24}

// Debouncer drops presses that arrive within Window of the previous press
// of the same button. Contacts on the HAT bounce for a few milliseconds and
// a single press must not trigger two 30-second refreshes.
type Debouncer struct {
	Window time.Duration
	Now    func() time.Time

	mu   sync.Mutex
	last map[int]time.Time
}

// Accept reports whether a press of button i should be acted on.
func (d *Debouncer) Accept(i int) bool {
	now := time.Now
	if d.Now != nil {
		now = d.Now
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.last == nil {
		d.last = map[int]time.Time{}
	}
	t := now()
	if prev, ok := d.last[i]; ok && t.Sub(prev) < d.Window {
		return false
	}
	d.last[i] = t
	return true
}
