package playlist

import (
	"fmt"
	"testing"
	"time"

	"github.com/chinmay28/photoframe/internal/config"
	"github.com/chinmay28/photoframe/internal/library"
)

var now = time.Date(2026, 8, 15, 12, 0, 0, 0, time.UTC)

func photos(n int) []library.Photo {
	var out []library.Photo
	for i := 0; i < n; i++ {
		out = append(out, library.Photo{
			ID:      fmt.Sprintf("p%02d", i),
			TakenAt: time.Date(2020+i, 1, 1, 0, 0, 0, 0, time.UTC),
		})
	}
	return out
}

func TestShuffleShowsEveryPhotoOncePerCycle(t *testing.T) {
	ps := photos(10)
	pl := New(1)
	for cycle := 0; cycle < 3; cycle++ {
		seen := map[string]bool{}
		for i := 0; i < len(ps); i++ {
			ph, ok := pl.Next(ps, config.OrderShuffle, now)
			if !ok {
				t.Fatal("Next returned !ok")
			}
			if seen[ph.ID] {
				t.Fatalf("cycle %d: %s repeated before all were shown", cycle, ph.ID)
			}
			seen[ph.ID] = true
		}
	}
}

func TestShuffleNeverRepeatsBackToBack(t *testing.T) {
	ps := photos(3)
	pl := New(7)
	prev := ""
	for i := 0; i < 50; i++ {
		ph, _ := pl.Next(ps, config.OrderShuffle, now)
		if ph.ID == prev {
			t.Fatalf("step %d: %s shown twice in a row", i, ph.ID)
		}
		prev = ph.ID
	}
}

func TestUpcomingMatchesNext(t *testing.T) {
	ps := photos(8)
	for _, order := range []config.Order{config.OrderShuffle, config.OrderNewest, config.OrderOldest} {
		pl := New(3)
		pl.Next(ps, order, now)
		up := pl.Upcoming(ps, order, now, 4)
		if len(up) != 4 {
			t.Fatalf("%s: Upcoming returned %d", order, len(up))
		}
		for i, want := range up {
			got, _ := pl.Next(ps, order, now)
			if got.ID != want.ID {
				t.Fatalf("%s step %d: Next=%s, Upcoming said %s", order, i, got.ID, want.ID)
			}
		}
	}
}

func TestSortedOrdersWrap(t *testing.T) {
	ps := photos(3) // p00 oldest .. p02 newest
	pl := New(0)
	var got []string
	for i := 0; i < 4; i++ {
		ph, _ := pl.Next(ps, config.OrderNewest, now)
		got = append(got, ph.ID)
	}
	want := "[p02 p01 p00 p02]" // from an empty start, the newest comes first
	if fmt.Sprint(got) != want {
		t.Fatalf("newest order = %v, want %v", got, want)
	}
}

func TestPrevAndForwardHistory(t *testing.T) {
	ps := photos(5)
	pl := New(2)
	a, _ := pl.Next(ps, config.OrderOldest, now)
	b, _ := pl.Next(ps, config.OrderOldest, now)
	back, ok := pl.Prev(ps)
	if !ok || back.ID != a.ID {
		t.Fatalf("Prev = %v %v, want %s", back.ID, ok, a.ID)
	}
	if _, ok := pl.Prev(ps); ok {
		t.Fatal("Prev past the start should fail")
	}
	fwd, _ := pl.Next(ps, config.OrderOldest, now)
	if fwd.ID != b.ID {
		t.Fatalf("Next after Prev = %s, want %s", fwd.ID, b.ID)
	}
}

func TestRemovedPhotosAreSkipped(t *testing.T) {
	ps := photos(4)
	pl := New(5)
	pl.Next(ps, config.OrderShuffle, now)
	up := pl.Upcoming(ps, config.OrderShuffle, now, 1)[0]
	var remaining []library.Photo
	for _, p := range ps {
		if p.ID != up.ID {
			remaining = append(remaining, p)
		}
	}
	for i := 0; i < 10; i++ {
		ph, _ := pl.Next(remaining, config.OrderShuffle, now)
		if ph.ID == up.ID {
			t.Fatal("deleted photo was shown")
		}
	}
}

func TestSinglePhotoRepeats(t *testing.T) {
	ps := photos(1)
	pl := New(0)
	for i := 0; i < 3; i++ {
		ph, ok := pl.Next(ps, config.OrderShuffle, now)
		if !ok || ph.ID != "p00" {
			t.Fatalf("got %v %v", ph.ID, ok)
		}
	}
	if _, ok := pl.Next(nil, config.OrderShuffle, now); ok {
		t.Fatal("Next with no photos should fail")
	}
}

func TestOnThisDay(t *testing.T) {
	ps := []library.Photo{
		{ID: "match-2019", TakenAt: time.Date(2019, 8, 14, 9, 0, 0, 0, time.UTC)},
		{ID: "match-2022", TakenAt: time.Date(2022, 8, 17, 9, 0, 0, 0, time.UTC)},
		{ID: "other", TakenAt: time.Date(2022, 3, 1, 9, 0, 0, 0, time.UTC)},
	}
	pl := New(0)
	for i := 0; i < 6; i++ {
		ph, _ := pl.Next(ps, config.OrderOnThisDay, now)
		if ph.ID == "other" {
			t.Fatal("on-this-day showed a photo from another date")
		}
	}
	// No matches: falls back to shuffle over everything.
	ph, ok := pl.Next(ps[2:], config.OrderOnThisDay, now)
	if !ok || ph.ID != "other" {
		t.Fatalf("fallback = %v %v", ph.ID, ok)
	}
}

func TestDayDistanceWrapsYear(t *testing.T) {
	a := time.Date(2010, 12, 30, 0, 0, 0, 0, time.UTC)
	b := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	if d := dayDistance(a, b); d != 3 {
		t.Fatalf("dayDistance = %d, want 3", d)
	}
}
