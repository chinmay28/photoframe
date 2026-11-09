package schedule

import (
	"testing"
	"time"

	"github.com/chinmay28/photoframe/internal/config"
)

func at(h, m int) time.Time { return time.Date(2026, 3, 10, h, m, 0, 0, time.UTC) }

var overnight = config.Quiet{Enabled: true, From: config.MustClock("22:00"), Until: config.MustClock("07:00")}
var midday = config.Quiet{Enabled: true, From: config.MustClock("12:00"), Until: config.MustClock("13:30")}

func TestInQuiet(t *testing.T) {
	cases := []struct {
		q    config.Quiet
		t    time.Time
		want bool
	}{
		{overnight, at(21, 59), false},
		{overnight, at(22, 0), true},
		{overnight, at(3, 0), true},
		{overnight, at(7, 0), false},
		{midday, at(12, 30), true},
		{midday, at(13, 30), false},
		{midday, at(11, 0), false},
		{config.Quiet{Enabled: false, From: 0, Until: 600}, at(3, 0), false},
	}
	for _, c := range cases {
		if got := InQuiet(c.t, c.q); got != c.want {
			t.Errorf("InQuiet(%s, %v-%v) = %v, want %v", c.t.Format("15:04"), c.q.From, c.q.Until, got, c.want)
		}
	}
}

func TestQuietEnd(t *testing.T) {
	if got, want := QuietEnd(at(23, 0), overnight), time.Date(2026, 3, 11, 7, 0, 0, 0, time.UTC); !got.Equal(want) {
		t.Errorf("before midnight: got %v want %v", got, want)
	}
	if got, want := QuietEnd(at(2, 0), overnight), at(7, 0); !got.Equal(want) {
		t.Errorf("after midnight: got %v want %v", got, want)
	}
	if got := QuietEnd(at(9, 0), overnight); !got.Equal(at(9, 0)) {
		t.Errorf("outside quiet should be unchanged, got %v", got)
	}
}

func TestNext(t *testing.T) {
	if got := Next(at(10, 0), 30*time.Minute, overnight); !got.Equal(at(10, 30)) {
		t.Errorf("plain interval: got %v", got)
	}
	if got, want := Next(at(21, 45), 30*time.Minute, overnight), time.Date(2026, 3, 11, 7, 0, 0, 0, time.UTC); !got.Equal(want) {
		t.Errorf("into quiet: got %v want %v", got, want)
	}
}
