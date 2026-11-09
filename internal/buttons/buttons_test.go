package buttons

import (
	"testing"
	"time"
)

func TestDebouncer(t *testing.T) {
	now := time.Unix(0, 0)
	d := &Debouncer{Window: 300 * time.Millisecond, Now: func() time.Time { return now }}
	if !d.Accept(0) {
		t.Fatal("first press rejected")
	}
	now = now.Add(10 * time.Millisecond)
	if d.Accept(0) {
		t.Fatal("bounce accepted")
	}
	if !d.Accept(1) {
		t.Fatal("other button affected by debounce")
	}
	now = now.Add(time.Second)
	if !d.Accept(0) {
		t.Fatal("later press rejected")
	}
}
