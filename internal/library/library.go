package library

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/chinmay28/photoframe/internal/config"
)

// Source provides photos. Sync makes the source's photos available locally
// (downloading if needed) and lists them. On failure it may still return the
// photos it has cached, together with the error. Sync is never called
// concurrently for the same Source.
type Source interface {
	Sync(ctx context.Context) ([]Photo, error)
}

// Factory builds a Source from its configuration.
type Factory func(config.Source) (Source, error)

// Status reports a source's state for the app.
type Status struct {
	Count    int       `json:"count"`
	LastSync time.Time `json:"lastSync,omitzero"`
	Error    string    `json:"error,omitempty"`
	Syncing  bool      `json:"syncing,omitempty"`
}

// Library aggregates photos from the enabled sources. It is safe for
// concurrent use.
type Library struct {
	factory Factory
	now     func() time.Time

	mu      sync.RWMutex
	entries map[string]*entry
	changed chan struct{}
	subs    []chan struct{}
	kick    chan struct{}
}

type entry struct {
	cfg    config.Source
	src    Source
	syncMu sync.Mutex // serialises Sync calls on src

	photos []Photo
	status Status
}

// New returns an empty library.
func New(f Factory) *Library {
	return &Library{
		factory: f,
		now:     time.Now,
		entries: map[string]*entry{},
		changed: make(chan struct{}, 1),
		kick:    make(chan struct{}, 1),
	}
}

// Configure replaces the set of sources. Enabled sources whose configuration
// is unchanged keep their photos; others are rebuilt and need a sync.
func (l *Library) Configure(srcs []config.Source) {
	l.mu.Lock()
	next := map[string]*entry{}
	for _, cfg := range srcs {
		if !cfg.Enabled {
			continue
		}
		if e, ok := l.entries[cfg.ID]; ok && e.cfg == cfg {
			next[cfg.ID] = e
			continue
		}
		e := &entry{cfg: cfg}
		src, err := l.factory(cfg)
		if err != nil {
			e.status.Error = err.Error()
		}
		e.src = src
		next[cfg.ID] = e
	}
	l.entries = next
	l.mu.Unlock()
	l.notify()
}

// SyncAll syncs every source in turn.
func (l *Library) SyncAll(ctx context.Context) {
	l.mu.RLock()
	ids := make([]string, 0, len(l.entries))
	for id := range l.entries {
		ids = append(ids, id)
	}
	l.mu.RUnlock()
	sort.Strings(ids)
	for _, id := range ids {
		if ctx.Err() != nil {
			return
		}
		_ = l.SyncSource(ctx, id)
	}
}

// SyncSource syncs one source. The error is also recorded in Status.
func (l *Library) SyncSource(ctx context.Context, id string) error {
	l.mu.RLock()
	e, ok := l.entries[id]
	l.mu.RUnlock()
	if !ok {
		return fmt.Errorf("source %q is not enabled", id)
	}
	if e.src == nil {
		return fmt.Errorf("source %q: %s", id, e.status.Error)
	}

	e.syncMu.Lock()
	defer e.syncMu.Unlock()

	l.mu.Lock()
	e.status.Syncing = true
	l.mu.Unlock()

	photos, err := e.src.Sync(ctx)

	l.mu.Lock()
	e.status.Syncing = false
	if l.entries[id] != e { // reconfigured while syncing; drop the result
		l.mu.Unlock()
		return err
	}
	if photos != nil || err == nil {
		e.photos = photos
		e.status.Count = len(photos)
	}
	e.status.LastSync = l.now()
	e.status.Error = ""
	if err != nil {
		e.status.Error = err.Error()
	}
	l.mu.Unlock()
	l.notify()
	return err
}

// Photos returns all photos from enabled sources, ordered by ID.
func (l *Library) Photos() []Photo {
	l.mu.RLock()
	defer l.mu.RUnlock()
	var out []Photo
	for _, e := range l.entries {
		out = append(out, e.photos...)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// Photo looks up one photo by ID.
func (l *Library) Photo(id string) (Photo, bool) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	for _, e := range l.entries {
		for _, p := range e.photos {
			if p.ID == id {
				return p, true
			}
		}
	}
	return Photo{}, false
}

// Status returns the state of every enabled source, keyed by source ID.
func (l *Library) Status() map[string]Status {
	l.mu.RLock()
	defer l.mu.RUnlock()
	out := make(map[string]Status, len(l.entries))
	for id, e := range l.entries {
		out[id] = e.status
	}
	return out
}

// Changed receives a value whenever the photo set may have changed.
func (l *Library) Changed() <-chan struct{} { return l.changed }

// Kick asks Run to sync all sources soon.
func (l *Library) Kick() {
	select {
	case l.kick <- struct{}{}:
	default:
	}
}

// Run syncs all sources every interval and whenever kicked, until ctx is
// cancelled. Callers do the initial SyncAll themselves so they can tell when
// the library is first populated.
func (l *Library) Run(ctx context.Context, every time.Duration) {
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		case <-l.kick:
		}
		l.SyncAll(ctx)
	}
}

// Subscribe returns a further channel that, like Changed, receives a value
// whenever the photo set may have changed. Each subscriber has its own
// channel; notifications are coalesced, so a slow reader sees at most one
// pending value.
func (l *Library) Subscribe() <-chan struct{} {
	c := make(chan struct{}, 1)
	l.mu.Lock()
	l.subs = append(l.subs, c)
	l.mu.Unlock()
	return c
}

func (l *Library) notify() {
	l.mu.RLock()
	chans := append([]chan struct{}{l.changed}, l.subs...)
	l.mu.RUnlock()
	for _, c := range chans {
		select {
		case c <- struct{}{}:
		default:
		}
	}
}
