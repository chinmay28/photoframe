// Package player runs the slideshow: it decides when to change photo, renders
// and sends images to the display, handles commands from the app and the
// frame's buttons, and persists what is on screen across restarts.
package player

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"log"
	"os"
	"sync"
	"time"

	"github.com/chinmay28/photoframe/internal/config"
	"github.com/chinmay28/photoframe/internal/library"
	"github.com/chinmay28/photoframe/internal/playlist"
	"github.com/chinmay28/photoframe/internal/render"
	"github.com/chinmay28/photoframe/internal/schedule"
)

// Display is the panel the player draws on.
type Display interface {
	Size() image.Point // native size
	Palette() color.Palette
	Show(ctx context.Context, img *image.Paletted) error
}

// Photos is the catalogue the player draws from.
type Photos interface {
	Photos() []library.Photo
	Changed() <-chan struct{}
}

// Command is an action requested by the app or a button.
type Command string

const (
	Next    Command = "next"
	Prev    Command = "prev"
	Pause   Command = "pause"
	Resume  Command = "resume"
	Refresh Command = "refresh" // re-render the current photo with current settings
	Clear   Command = "clear"   // blank the panel and pause
	Info    Command = "info"    // show how to reach the app
)

// Valid reports whether c is a known command.
func (c Command) Valid() bool {
	switch c {
	case Next, Prev, Pause, Resume, Refresh, Clear, Info:
		return true
	}
	return false
}

// Screen is what the panel currently shows.
type Screen string

const (
	ScreenNone    Screen = ""
	ScreenPhoto   Screen = "photo"
	ScreenWelcome Screen = "welcome"
	ScreenInfo    Screen = "info"
	ScreenClear   Screen = "clear"
)

// Status is a snapshot for the app.
type Status struct {
	Screen     Screen         `json:"screen"`
	Current    *library.Photo `json:"current,omitempty"`
	Paused     bool           `json:"paused"`
	Refreshing bool           `json:"refreshing"`
	ShownAt    time.Time      `json:"shownAt,omitzero"`
	NextAt     time.Time      `json:"nextAt,omitzero"`
	Total      int            `json:"total"`
	Error      string         `json:"error,omitempty"`
}

// Deps are the player's collaborators.
type Deps struct {
	Display         Display
	Library         Photos
	Settings        func() config.Settings
	SettingsChanged <-chan struct{}
	// Ready is closed once the library has finished its first sync. Until
	// then the player leaves whatever is on the panel alone.
	Ready <-chan struct{}
	// StatePath persists the on-screen photo across restarts. Optional.
	StatePath string
	// InfoLines returns the text for the welcome and info screens.
	InfoLines func() []string
	Now       func() time.Time
	Logger    *log.Logger
	// RetryDelay is how long to wait after a display error. Default 1 min.
	RetryDelay time.Duration
	// Seed seeds shuffling; 0 uses the clock.
	Seed int64
}

// Player runs the slideshow. Create with New and start with Run.
type Player struct {
	d    Deps
	cmds chan Command

	mu         sync.Mutex
	pl         *playlist.Playlist
	current    *library.Photo
	screen     Screen
	shownAt    time.Time
	paused     bool
	refreshing bool
	nextAt     time.Time
	retryAt    time.Time
	pending    *library.Photo // photo whose display failed, re-sent at retryAt
	lastErr    string
	preview    []byte
}

// New returns a player; it does nothing until Run.
func New(d Deps) *Player {
	if d.Now == nil {
		d.Now = time.Now
	}
	if d.Logger == nil {
		d.Logger = log.Default()
	}
	if d.RetryDelay == 0 {
		d.RetryDelay = time.Minute
	}
	if d.InfoLines == nil {
		d.InfoLines = func() []string { return nil }
	}
	if d.Seed == 0 {
		d.Seed = time.Now().UnixNano()
	}
	return &Player{d: d, cmds: make(chan Command, 8), pl: playlist.New(d.Seed)}
}

// Do queues a command. It never blocks; if the queue is full the command is
// dropped, which only happens when someone hammers a button mid-refresh.
func (p *Player) Do(c Command) bool {
	select {
	case p.cmds <- c:
		return true
	default:
		return false
	}
}

// Status returns the current state.
func (p *Player) Status() Status {
	p.mu.Lock()
	defer p.mu.Unlock()
	st := Status{
		Screen: p.screen, Paused: p.paused, Refreshing: p.refreshing,
		ShownAt: p.shownAt, NextAt: p.nextAt, Error: p.lastErr,
		Total: len(p.d.Library.Photos()),
	}
	if p.current != nil {
		c := *p.current
		st.Current = &c
	}
	return st
}

// Preview returns the last image sent to the panel as PNG, in wall
// orientation and the panel's colours, or nil if nothing was shown yet.
func (p *Player) Preview() []byte {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.preview
}

// UpNext returns the next n photos in order.
func (p *Player) UpNext(n int) []library.Photo {
	photos := p.d.Library.Photos()
	s := p.d.Settings()
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.pl.Upcoming(photos, s.Order, p.d.Now(), n)
}

// Run drives the slideshow until ctx is cancelled.
func (p *Player) Run(ctx context.Context) error {
	p.restore()
	if p.d.Ready != nil {
		select {
		case <-p.d.Ready:
		case <-ctx.Done():
			return nil
		}
	}

	for {
		s := p.d.Settings()
		photos := p.d.Library.Photos()

		if len(photos) == 0 {
			p.setNextAt(time.Time{})
			if p.needsWelcome() {
				p.showMessage(ctx, s, ScreenWelcome, "Photoframe")
			}
		}

		// Work out the next wake-up: a retry of a photo the panel failed to
		// show, a scheduled photo change, or a retry of the welcome screen.
		due, ok := p.due(s, photos)
		action := func() { p.advance(ctx, s, photos, Next) }
		if ph := p.pendingPhoto(); ph != nil && contains(photos, ph.ID) {
			due, ok = p.retryAtTime(), true
			action = func() { p.retry(ctx, s, *ph) }
			p.setNextAt(due)
		} else if ok {
			p.setNextAt(due)
		} else {
			p.setNextAt(time.Time{})
			if r := p.pendingRetry(); !r.IsZero() {
				due, ok, action = r, true, func() {}
			}
		}

		var timer *time.Timer
		var fire <-chan time.Time
		if ok {
			wait := due.Sub(p.d.Now())
			if wait <= 0 {
				action()
				continue
			}
			timer = time.NewTimer(wait)
			fire = timer.C
		}

		select {
		case <-ctx.Done():
			stop(timer)
			return nil
		case <-fire:
			action()
		case c := <-p.cmds:
			stop(timer)
			p.handle(ctx, p.latest(c))
		case <-p.d.SettingsChanged:
			stop(timer)
		case <-p.d.Library.Changed():
			stop(timer)
		}
	}
}

func stop(t *time.Timer) {
	if t != nil {
		t.Stop()
	}
}

// latest drains queued commands so a burst of presses made during a slow
// refresh results in one action, not a refresh per press. Pause/resume
// always take effect.
func (p *Player) latest(c Command) Command {
	for {
		select {
		case n := <-p.cmds:
			if c == Pause || c == Resume {
				p.applyPause(c == Pause)
			}
			c = n
		default:
			return c
		}
	}
}

func (p *Player) handle(ctx context.Context, c Command) {
	s := p.d.Settings()
	photos := p.d.Library.Photos()
	switch c {
	case Next, Prev:
		p.advance(ctx, s, photos, c)
	case Pause, Resume:
		p.applyPause(c == Pause)
		p.save()
	case Refresh:
		p.mu.Lock()
		cur, screen := p.current, p.screen
		p.mu.Unlock()
		if cur != nil && screen == ScreenPhoto {
			if err := p.showPhoto(ctx, s, *cur); err != nil {
				p.fail(err)
				if isDisplayError(err) {
					p.setPending(cur)
				}
			}
			return
		}
		p.advance(ctx, s, photos, Next)
	case Clear:
		size := p.d.Display.Size()
		img := render.Solid(size.X, size.Y, p.d.Display.Palette(), color.White)
		err := p.present(ctx, img, config.Landscape, func() { p.screen, p.paused = ScreenClear, true })
		if err != nil {
			p.fail(err)
			return
		}
		p.save()
	case Info:
		p.showMessage(ctx, s, ScreenInfo, s.Name)
	}
}

func (p *Player) applyPause(paused bool) {
	p.mu.Lock()
	p.paused = paused
	p.mu.Unlock()
}

// due returns when the next automatic change should happen.
func (p *Player) due(s config.Settings, photos []library.Photo) (time.Time, bool) {
	if len(photos) == 0 {
		return time.Time{}, false
	}
	p.mu.Lock()
	defer p.mu.Unlock()

	now := p.d.Now()
	var due time.Time
	switch {
	case p.screen == ScreenWelcome || p.screen == ScreenNone:
		due = now // photos arrived: replace the welcome screen right away
	case p.paused:
		return time.Time{}, false
	case p.screen == ScreenPhoto && p.current != nil && !contains(photos, p.current.ID):
		due = now // the photo on screen was deleted or its source disabled
	default:
		due = schedule.Next(p.shownAt, s.Interval.D(), s.Quiet)
	}
	if due.Before(p.retryAt) {
		due = p.retryAt
	}
	return due, true
}

func (p *Player) pendingPhoto() *library.Photo {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.pending
}

func (p *Player) retryAtTime() time.Time {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.retryAt
}

// retry re-sends a photo whose display failed.
func (p *Player) retry(ctx context.Context, s config.Settings, ph library.Photo) {
	if err := p.showPhoto(ctx, s, ph); err != nil {
		p.fail(err)
		if !isDisplayError(err) {
			p.setPending(nil) // the file itself is now bad; move on
		}
	}
}

func (p *Player) setPending(ph *library.Photo) {
	p.mu.Lock()
	p.pending = ph
	p.mu.Unlock()
}

func isDisplayError(err error) bool {
	var de *displayError
	return errors.As(err, &de)
}

// pendingRetry returns the time of a scheduled retry after a display
// error, or zero if none is pending.
func (p *Player) pendingRetry() time.Time {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.retryAt.After(p.d.Now()) {
		return p.retryAt
	}
	return time.Time{}
}

func (p *Player) needsWelcome() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.screen != ScreenWelcome && p.screen != ScreenClear && p.d.Now().After(p.retryAt)
}

// advance moves to the next or previous photo, skipping photos that fail to
// load. A display failure stops immediately and schedules a retry.
func (p *Player) advance(ctx context.Context, s config.Settings, photos []library.Photo, dir Command) {
	const maxSkips = 5
	for i := 0; i < maxSkips; i++ {
		p.mu.Lock()
		var ph library.Photo
		var ok bool
		if dir == Prev {
			ph, ok = p.pl.Prev(photos)
		} else {
			ph, ok = p.pl.Next(photos, s.Order, p.d.Now())
		}
		p.mu.Unlock()
		if !ok {
			return
		}

		err := p.showPhoto(ctx, s, ph)
		if err == nil {
			return
		}
		p.fail(err)
		if isDisplayError(err) {
			p.setPending(&ph)
			return
		}
		if ctx.Err() != nil {
			return
		}
		dir = Next // skip the unreadable photo
	}
}

func (p *Player) showPhoto(ctx context.Context, s config.Settings, ph library.Photo) error {
	size := p.d.Display.Size()
	o := render.OptionsFrom(s, size.X, size.Y, p.d.Display.Palette())
	if s.Caption {
		o.Caption = Caption(ph)
	}
	src, err := render.Load(ph.Path, render.MaxSourceSide(o.Width, o.Height))
	if err != nil {
		return fmt.Errorf("photo %s: %w", ph.ID, err)
	}
	err = p.present(ctx, render.Render(src, o), s.Display.Orientation, func() {
		p.current, p.screen, p.shownAt, p.lastErr, p.pending = &ph, ScreenPhoto, p.d.Now(), "", nil
	})
	if err != nil {
		return err
	}
	p.save()
	return nil
}

func (p *Player) showMessage(ctx context.Context, s config.Settings, screen Screen, title string) {
	size := p.d.Display.Size()
	w, h := size.X, size.Y
	if s.Display.Orientation == config.Portrait {
		w, h = h, w
	}
	img := render.Message(w, h, p.d.Display.Palette(), title, p.d.InfoLines()...)
	if err := p.present(ctx, img, s.Display.Orientation, func() { p.screen = screen }); err != nil {
		p.fail(err)
	}
}

type displayError struct{ err error }

func (e *displayError) Error() string { return "display: " + e.err.Error() }
func (e *displayError) Unwrap() error { return e.err }

// present sends a logical-orientation image to the panel. On success commit
// runs under the lock, so status never shows the new image half-applied.
func (p *Player) present(ctx context.Context, img *image.Paletted, o config.Orientation, commit func()) error {
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	native := img
	if o == config.Portrait {
		native = render.Rotate90(img)
	}

	p.mu.Lock()
	p.refreshing = true
	p.mu.Unlock()
	err := p.d.Display.Show(ctx, native)
	p.mu.Lock()
	p.refreshing = false
	if err == nil {
		p.preview = buf.Bytes()
		p.retryAt = time.Time{}
		commit()
	}
	p.mu.Unlock()
	if err != nil {
		return &displayError{err}
	}
	return nil
}

func (p *Player) fail(err error) {
	p.d.Logger.Printf("player: %v", err)
	p.mu.Lock()
	defer p.mu.Unlock()
	p.lastErr = err.Error()
	if isDisplayError(err) {
		p.retryAt = p.d.Now().Add(p.d.RetryDelay)
	}
}

func (p *Player) setNextAt(t time.Time) {
	p.mu.Lock()
	p.nextAt = t
	p.mu.Unlock()
}

// Caption formats the label drawn on a photo.
func Caption(ph library.Photo) string {
	date := ""
	if !ph.TakenAt.IsZero() {
		date = ph.TakenAt.Format("Jan 2006")
	}
	switch {
	case ph.Title != "" && date != "":
		return ph.Title + " · " + date
	case ph.Title != "":
		return ph.Title
	}
	return date
}

func contains(photos []library.Photo, id string) bool {
	for _, ph := range photos {
		if ph.ID == id {
			return true
		}
	}
	return false
}

// persisted is what survives a restart.
type persisted struct {
	Current *library.Photo `json:"current,omitempty"`
	Screen  Screen         `json:"screen"`
	ShownAt time.Time      `json:"shownAt"`
	Paused  bool           `json:"paused"`
}

func (p *Player) save() {
	if p.d.StatePath == "" {
		return
	}
	p.mu.Lock()
	st := persisted{Current: p.current, Screen: p.screen, ShownAt: p.shownAt, Paused: p.paused}
	p.mu.Unlock()
	b, err := json.Marshal(st)
	if err == nil {
		err = config.WriteFileAtomic(p.d.StatePath, b, 0o644)
	}
	if err != nil {
		p.d.Logger.Printf("player: save state: %v", err)
	}
}

func (p *Player) restore() {
	if p.d.StatePath == "" {
		return
	}
	b, err := os.ReadFile(p.d.StatePath)
	if err != nil {
		return
	}
	var st persisted
	if err := json.Unmarshal(b, &st); err != nil {
		p.d.Logger.Printf("player: ignoring corrupt state: %v", err)
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.paused = st.Paused
	p.shownAt = st.ShownAt
	p.screen = st.Screen
	if st.Current != nil && st.Screen == ScreenPhoto {
		p.current = st.Current
		p.pl.SetCurrent(st.Current.ID)
	}
	if p.screen == ScreenInfo {
		p.screen = ScreenPhoto // info is transient; resume the schedule
	}
}
