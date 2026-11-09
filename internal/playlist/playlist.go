// Package playlist decides which photo comes next. It keeps a short history
// so the frame can step back, and for shuffle a deck so every photo is shown
// once before any repeats. It holds no locks; callers serialise access.
package playlist

import (
	"math/rand"
	"sort"
	"time"

	"github.com/chinmay28/photoframe/internal/config"
	"github.com/chinmay28/photoframe/internal/library"
)

const (
	maxHistory = 100
	// OnThisDayWindow is how many days either side of today count as "this day".
	OnThisDayWindow = 3
)

// Playlist tracks the shown history and the upcoming shuffle deck.
type Playlist struct {
	rng     *rand.Rand
	history []string // IDs shown, oldest first
	pos     int      // index of the current photo in history, -1 if none
	deck    []string // shuffle order of photos not yet shown this cycle
	shown   map[string]bool
}

// New returns an empty playlist. seed makes shuffling reproducible in tests.
func New(seed int64) *Playlist {
	return &Playlist{rng: rand.New(rand.NewSource(seed)), pos: -1, shown: map[string]bool{}}
}

// Current returns the ID of the photo on screen, or "" if none.
func (p *Playlist) Current() string {
	if p.pos < 0 {
		return ""
	}
	return p.history[p.pos]
}

// SetCurrent records id as on screen without choosing it, for example when
// restoring state after a restart.
func (p *Playlist) SetCurrent(id string) {
	p.push(id)
}

// Upcoming returns up to n photos that Next would show, in order, without
// changing state.
func (p *Playlist) Upcoming(photos []library.Photo, order config.Order, now time.Time, n int) []library.Photo {
	byID := index(photos)
	var out []library.Photo
	seen := map[string]bool{p.Current(): true}
	add := func(id string) bool {
		if ph, ok := byID[id]; ok && !seen[id] {
			seen[id] = true
			out = append(out, ph)
		}
		return len(out) >= n
	}

	// Photos stepped back past come first.
	for i := p.pos + 1; i < len(p.history); i++ {
		if add(p.history[i]) {
			return out
		}
	}
	for _, id := range p.sequence(photos, byID, order, now) {
		if add(id) {
			break
		}
	}
	return out
}

// Next advances to the next photo and returns it. ok is false when there are
// no photos.
func (p *Playlist) Next(photos []library.Photo, order config.Order, now time.Time) (library.Photo, bool) {
	byID := index(photos)
	if len(byID) == 0 {
		return library.Photo{}, false
	}

	// Walk forward through history first (after Prev).
	for p.pos+1 < len(p.history) {
		p.pos++
		if ph, ok := byID[p.history[p.pos]]; ok {
			p.shown[ph.ID] = true
			return ph, true
		}
	}

	seq := p.sequence(photos, byID, order, now)
	var id string
	if len(seq) > 0 {
		id = seq[0]
	} else {
		id = p.Current() // the only photo is the one on screen
	}
	if order == config.OrderShuffle || (order == config.OrderOnThisDay && len(onThisDay(photos, now)) == 0) {
		p.removeFromDeck(id)
	}
	p.shown[id] = true
	p.push(id)
	return byID[id], true
}

// Prev steps back to the previously shown photo still in the library.
func (p *Playlist) Prev(photos []library.Photo) (library.Photo, bool) {
	byID := index(photos)
	for i := p.pos - 1; i >= 0; i-- {
		if ph, ok := byID[p.history[i]]; ok {
			p.pos = i
			return ph, true
		}
	}
	return library.Photo{}, false
}

// sequence returns candidate IDs in the order they would be shown next,
// excluding the current photo.
func (p *Playlist) sequence(photos []library.Photo, byID map[string]library.Photo, order config.Order, now time.Time) []string {
	switch order {
	case config.OrderNewest:
		return after(sorted(photos, false), p.Current())
	case config.OrderOldest:
		return after(sorted(photos, true), p.Current())
	case config.OrderOnThisDay:
		if c := onThisDay(photos, now); len(c) > 0 {
			return after(sorted(c, true), p.Current())
		}
	}
	p.syncDeck(byID)
	return p.deck
}

// syncDeck drops deleted photos from the deck, deals in new ones at random
// positions, and starts a new cycle when everything has been shown.
func (p *Playlist) syncDeck(byID map[string]library.Photo) {
	cur := p.Current()
	kept := p.deck[:0]
	inDeck := map[string]bool{}
	for _, id := range p.deck {
		if _, ok := byID[id]; ok && id != cur {
			kept = append(kept, id)
			inDeck[id] = true
		}
	}
	p.deck = kept

	var fresh []string
	for id := range byID {
		if !inDeck[id] && !p.shown[id] && id != cur {
			fresh = append(fresh, id)
		}
	}
	sort.Strings(fresh) // map order is random; keep the rng the only source of randomness
	for _, id := range fresh {
		i := p.rng.Intn(len(p.deck) + 1)
		p.deck = append(p.deck, "")
		copy(p.deck[i+1:], p.deck[i:])
		p.deck[i] = id
	}

	if len(p.deck) == 0 && len(byID) > 1 {
		p.shown = map[string]bool{}
		for id := range byID {
			if id != cur {
				p.deck = append(p.deck, id)
			}
		}
		sort.Strings(p.deck)
		p.rng.Shuffle(len(p.deck), func(i, j int) { p.deck[i], p.deck[j] = p.deck[j], p.deck[i] })
	}
}

func (p *Playlist) removeFromDeck(id string) {
	for i, d := range p.deck {
		if d == id {
			p.deck = append(p.deck[:i], p.deck[i+1:]...)
			return
		}
	}
}

func (p *Playlist) push(id string) {
	// Showing something new discards any "forward" history.
	p.history = append(p.history[:p.pos+1], id)
	if len(p.history) > maxHistory {
		p.history = p.history[len(p.history)-maxHistory:]
	}
	p.pos = len(p.history) - 1
}

func index(photos []library.Photo) map[string]library.Photo {
	m := make(map[string]library.Photo, len(photos))
	for _, ph := range photos {
		m[ph.ID] = ph
	}
	return m
}

func sorted(photos []library.Photo, oldestFirst bool) []library.Photo {
	s := append([]library.Photo(nil), photos...)
	sort.SliceStable(s, func(i, j int) bool {
		a, b := s[i], s[j]
		if !a.TakenAt.Equal(b.TakenAt) {
			return a.TakenAt.Before(b.TakenAt) == oldestFirst
		}
		return a.ID < b.ID
	})
	return s
}

// after returns the IDs following cur in s, wrapping around, excluding cur.
func after(s []library.Photo, cur string) []string {
	start := 0
	for i, ph := range s {
		if ph.ID == cur {
			start = i + 1
			break
		}
	}
	out := make([]string, 0, len(s))
	for i := 0; i < len(s); i++ {
		if id := s[(start+i)%len(s)].ID; id != cur {
			out = append(out, id)
		}
	}
	return out
}

// onThisDay returns photos taken within OnThisDayWindow days of now's date
// in any year.
func onThisDay(photos []library.Photo, now time.Time) []library.Photo {
	var out []library.Photo
	for _, ph := range photos {
		if ph.TakenAt.IsZero() {
			continue
		}
		if dayDistance(ph.TakenAt, now) <= OnThisDayWindow {
			out = append(out, ph)
		}
	}
	return out
}

// dayDistance is the number of days between the calendar dates of a and b
// ignoring the year, so 30 Dec and 2 Jan are 3 days apart.
func dayDistance(a, b time.Time) int {
	ref := time.Date(b.Year(), b.Month(), b.Day(), 0, 0, 0, 0, time.UTC)
	best := 400
	for _, y := range []int{b.Year() - 1, b.Year(), b.Year() + 1} {
		d := time.Date(y, a.Month(), a.Day(), 0, 0, 0, 0, time.UTC).Sub(ref)
		days := int(d.Hours() / 24)
		if days < 0 {
			days = -days
		}
		if days < best {
			best = days
		}
	}
	return best
}
