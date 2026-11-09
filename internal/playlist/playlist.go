// Package playlist decides which photo comes next. It keeps a short history
// so the frame can step back, and for shuffle a deck so every photo is shown
// once before any repeats. It holds no locks; callers serialise access.
package playlist

import (
	"math/rand"
	"slices"
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
	// pinned are photos to show next, first first (PlayNext).
	pinned []string
	// deferred maps photos sent to the back (PlayLast) to how many other
	// photos must be shown before them.
	deferred map[string]int
	// anchor is the last photo the order itself chose. Sorted orders carry
	// on after it, so showing a queued or jumped-to photo does not lose the
	// place.
	anchor string
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
	p.anchor = id
}

// Upcoming returns up to n photos that Next would show, in order, without
// changing state. n < 0 returns every photo that is coming up.
func (p *Playlist) Upcoming(photos []library.Photo, order config.Order, now time.Time, n int) []library.Photo {
	byID := index(photos)
	ids := p.upcoming(byID, photos, order, now)
	if n >= 0 && len(ids) > n {
		ids = ids[:n]
	}
	out := make([]library.Photo, 0, len(ids))
	for _, id := range ids {
		out = append(out, byID[id])
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
	id := p.Current() // the only photo is the one on screen
	if ids := p.upcoming(byID, photos, order, now); len(ids) > 0 {
		id = ids[0]
	}
	if _, ok := byID[id]; !ok { // the current photo is gone and nothing else is left
		return library.Photo{}, false
	}
	// Walking forward through history (after Prev) keeps that history.
	moved := false
	for j := p.pos + 1; j < len(p.history); j++ {
		if p.history[j] == id {
			p.pos, moved = j, true
			break
		}
	}
	_, wasDeferred := p.deferred[id]
	if !moved && !wasDeferred && !slices.Contains(p.pinned, id) {
		p.anchor = id // chosen by the order itself
	}
	if !moved {
		p.push(id)
	}
	p.consume(id)
	return byID[id], true
}

// Jump makes id the current photo, as if Next had chosen it: sorted orders
// carry on from it. It reports false, changing nothing, when id is not
// among photos.
func (p *Playlist) Jump(photos []library.Photo, id string) bool {
	if _, ok := index(photos)[id]; !ok {
		return false
	}
	p.push(id)
	p.anchor = id
	p.consume(id)
	return true
}

// PlayNext puts id at the front of the queue: it is shown next, ahead of
// photos queued earlier. It reports false for the photo on screen.
func (p *Playlist) PlayNext(id string) bool {
	if id == "" || id == p.Current() {
		return false
	}
	p.unqueue(id)
	p.pinned = append([]string{id}, p.pinned...)
	return true
}

// PlayLast sends id to the back of the queue: every other photo is shown
// once before it. It reports false for the photo on screen or a photo that
// is not among photos.
func (p *Playlist) PlayLast(photos []library.Photo, id string) bool {
	byID := index(photos)
	if _, ok := byID[id]; !ok || id == p.Current() {
		return false
	}
	p.unqueue(id)
	others := len(byID) - 1
	if _, ok := byID[p.Current()]; ok {
		others-- // the photo on screen has had its turn
	}
	if p.deferred == nil {
		p.deferred = map[string]int{}
	}
	p.deferred[id] = others
	return true
}

// upcoming lists the IDs Next would show, in order: photos queued with
// PlayNext, photos stepped back past, then the order's sequence with photos
// sent back by PlayLast placed after the number of others they wait for.
func (p *Playlist) upcoming(byID map[string]library.Photo, photos []library.Photo, order config.Order, now time.Time) []string {
	cur := p.Current()
	seen := map[string]bool{cur: true}
	var out []string
	add := func(id string) {
		if _, ok := byID[id]; ok && !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	for _, id := range p.pinned {
		add(id)
	}
	for i := p.pos + 1; i < len(p.history); i++ {
		if _, wait := p.deferred[p.history[i]]; !wait {
			add(p.history[i])
		}
	}

	type held struct {
		id   string
		wait int
	}
	var back []held
	for id, wait := range p.deferred {
		if _, ok := byID[id]; ok && !seen[id] {
			back = append(back, held{id, wait})
		}
	}
	sort.Slice(back, func(i, j int) bool {
		if back[i].wait != back[j].wait {
			return back[i].wait < back[j].wait
		}
		return back[i].id < back[j].id
	})
	release := func() {
		for len(back) > 0 && back[0].wait <= len(out) {
			add(back[0].id)
			back = back[1:]
		}
	}
	for _, id := range p.sequence(photos, byID, order, now) {
		if _, wait := p.deferred[id]; wait {
			continue
		}
		release()
		add(id)
	}
	for _, h := range back {
		add(h.id)
	}
	return out
}

// consume records that id is now on screen.
func (p *Playlist) consume(id string) {
	p.shown[id] = true
	p.removeFromDeck(id)
	p.unqueue(id)
	for other, wait := range p.deferred {
		if wait > 0 {
			p.deferred[other] = wait - 1
		}
	}
}

// unqueue drops id from the PlayNext and PlayLast queues.
func (p *Playlist) unqueue(id string) {
	for i, q := range p.pinned {
		if q == id {
			p.pinned = append(p.pinned[:i], p.pinned[i+1:]...)
			break
		}
	}
	delete(p.deferred, id)
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

// sequence returns candidate IDs in the order they would be shown next.
// Callers skip the photo on screen.
func (p *Playlist) sequence(photos []library.Photo, byID map[string]library.Photo, order config.Order, now time.Time) []string {
	switch order {
	case config.OrderNewest:
		return after(sorted(photos, false), p.anchor)
	case config.OrderOldest:
		return after(sorted(photos, true), p.anchor)
	case config.OrderOnThisDay:
		if c := onThisDay(photos, now); len(c) > 0 {
			return after(sorted(c, true), p.anchor)
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
