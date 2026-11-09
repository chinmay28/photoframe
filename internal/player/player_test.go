package player

import (
	"context"
	"errors"
	"image"
	"image/color"
	"image/png"
	"log"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/chinmay28/photoframe/internal/adjust"
	"github.com/chinmay28/photoframe/internal/config"
	"github.com/chinmay28/photoframe/internal/library"
)

var pal = color.Palette{color.Black, color.White, color.RGBA{255, 0, 0, 255}}

type fakeDisplay struct {
	mu    sync.Mutex
	shown []*image.Paletted
	fail  error
	ch    chan struct{}
}

func newDisplay() *fakeDisplay { return &fakeDisplay{ch: make(chan struct{}, 100)} }

func (d *fakeDisplay) Size() image.Point      { return image.Pt(80, 48) }
func (d *fakeDisplay) Palette() color.Palette { return pal }
func (d *fakeDisplay) Show(_ context.Context, img *image.Paletted) error {
	d.mu.Lock()
	err := d.fail
	if err == nil {
		d.shown = append(d.shown, img)
	}
	d.mu.Unlock()
	d.ch <- struct{}{}
	return err
}

func (d *fakeDisplay) count() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return len(d.shown)
}

// wait blocks until n more Show calls happened.
func (d *fakeDisplay) wait(t *testing.T, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		select {
		case <-d.ch:
		case <-time.After(3 * time.Second):
			t.Fatalf("timed out waiting for Show #%d", i+1)
		}
	}
}

// waitIdle waits for n Show calls and for the player to record the result.
func (h *harness) wait(t *testing.T, n int) {
	t.Helper()
	h.disp.wait(t, n)
	deadline := time.Now().Add(time.Second)
	for h.p.Status().Refreshing && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	time.Sleep(5 * time.Millisecond) // let Run finish bookkeeping after Show returns
}

func (d *fakeDisplay) expectNone(t *testing.T) {
	t.Helper()
	select {
	case <-d.ch:
		t.Fatal("unexpected Show")
	case <-time.After(100 * time.Millisecond):
	}
}

type fakeLib struct {
	mu      sync.Mutex
	photos  []library.Photo
	changed chan struct{}
}

func (l *fakeLib) Photos() []library.Photo {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]library.Photo(nil), l.photos...)
}
func (l *fakeLib) Changed() <-chan struct{} { return l.changed }
func (l *fakeLib) set(p []library.Photo) {
	l.mu.Lock()
	l.photos = p
	l.mu.Unlock()
	l.changed <- struct{}{}
}

func writePNG(t *testing.T, path string) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, image.NewRGBA(image.Rect(0, 0, 40, 30))); err != nil {
		t.Fatal(err)
	}
}

func makePhotos(t *testing.T, n int) []library.Photo {
	dir := t.TempDir()
	var out []library.Photo
	for i := 0; i < n; i++ {
		p := filepath.Join(dir, string(rune('a'+i))+".png")
		writePNG(t, p)
		out = append(out, library.Photo{ID: string(rune('a' + i)), Path: p, TakenAt: time.Date(2024, 1, i+1, 0, 0, 0, 0, time.UTC)})
	}
	return out
}

type harness struct {
	p        *Player
	disp     *fakeDisplay
	lib      *fakeLib
	settings config.Settings
	adjMu    sync.Mutex
	adj      map[string]adjust.Adjustment
	cancel   context.CancelFunc
	done     chan struct{}
}

func start(t *testing.T, photos []library.Photo, statePath string, mutate func(*config.Settings)) *harness {
	t.Helper()
	s := config.Default()
	s.Quiet.Enabled = false
	s.Caption = false
	if mutate != nil {
		mutate(&s)
	}
	h := &harness{disp: newDisplay(), lib: &fakeLib{photos: photos, changed: make(chan struct{}, 1)}, settings: s}
	h.p = New(Deps{
		Display: h.disp, Library: h.lib,
		Settings:        func() config.Settings { return h.settings },
		SettingsChanged: make(chan struct{}),
		Adjustment: func(id string) adjust.Adjustment {
			h.adjMu.Lock()
			defer h.adjMu.Unlock()
			return h.adj[id]
		},
		StatePath:  statePath,
		Logger:     log.New(testWriter{t}, "", 0),
		RetryDelay: 50 * time.Millisecond,
		Seed:       1,
	})
	ctx, cancel := context.WithCancel(context.Background())
	h.cancel, h.done = cancel, make(chan struct{})
	go func() { h.p.Run(ctx); close(h.done) }()
	t.Cleanup(h.stop)
	return h
}

func (h *harness) stop() {
	h.cancel()
	<-h.done
}

func TestShowsPhotoImmediatelyOnFirstStart(t *testing.T) {
	state := filepath.Join(t.TempDir(), "state.json")
	h := start(t, makePhotos(t, 3), state, nil)
	h.wait(t, 1)
	h.disp.expectNone(t) // 30-minute interval: nothing more for now

	st := h.p.Status()
	if st.Screen != ScreenPhoto || st.Current == nil || st.Total != 3 || st.NextAt.IsZero() {
		t.Fatalf("status = %+v", st)
	}
	if h.p.Preview() == nil {
		t.Fatal("no preview after showing a photo")
	}
	if _, err := os.Stat(state); err != nil {
		t.Fatal("state not saved")
	}
}

func TestRestartDoesNotRefreshEarly(t *testing.T) {
	photos := makePhotos(t, 3)
	state := filepath.Join(t.TempDir(), "state.json")
	h := start(t, photos, state, nil)
	h.wait(t, 1)
	first := h.p.Status().Current.ID
	h.stop()

	h2 := start(t, photos, state, nil)
	h2.disp.expectNone(t)
	if st := h2.p.Status(); st.Current == nil || st.Current.ID != first {
		t.Fatalf("restored status = %+v", st)
	}
}

func TestIntervalAdvances(t *testing.T) {
	h := start(t, makePhotos(t, 3), "", func(s *config.Settings) { s.Interval = config.Duration(80 * time.Millisecond) })
	h.wait(t, 3)
}

func TestNextPrevCommands(t *testing.T) {
	h := start(t, makePhotos(t, 4), "", func(s *config.Settings) { s.Order = config.OrderOldest })
	h.wait(t, 1)
	a := h.p.Status().Current.ID
	h.p.Do(Next)
	h.wait(t, 1)
	b := h.p.Status().Current.ID
	if a == b {
		t.Fatal("Next did not change photo")
	}
	h.p.Do(Prev)
	h.wait(t, 1)
	if got := h.p.Status().Current.ID; got != a {
		t.Fatalf("Prev went to %s, want %s", got, a)
	}
}

func TestPauseStopsSchedule(t *testing.T) {
	h := start(t, makePhotos(t, 3), "", func(s *config.Settings) { s.Interval = config.Duration(80 * time.Millisecond) })
	h.wait(t, 1)
	h.p.Do(Pause)
	time.Sleep(20 * time.Millisecond)
	for len(h.disp.ch) > 0 { // drop a refresh that raced with the pause
		<-h.disp.ch
	}
	h.disp.expectNone(t)
	if !h.p.Status().Paused {
		t.Fatal("not paused")
	}
	h.p.Do(Resume)
	h.wait(t, 1)
}

func TestWelcomeThenPhotosArrive(t *testing.T) {
	h := start(t, nil, "", nil)
	h.wait(t, 1)
	if h.p.Status().Screen != ScreenWelcome {
		t.Fatalf("screen = %q", h.p.Status().Screen)
	}
	h.disp.expectNone(t) // welcome is shown once, not repeatedly

	h.lib.set(makePhotos(t, 2))
	h.wait(t, 1)
	if h.p.Status().Screen != ScreenPhoto {
		t.Fatalf("screen = %q after photos arrived", h.p.Status().Screen)
	}
}

func TestUnreadablePhotoIsSkipped(t *testing.T) {
	photos := makePhotos(t, 2)
	photos = append(photos, library.Photo{ID: "bad", Path: filepath.Join(t.TempDir(), "missing.jpg")})
	h := start(t, photos, "", func(s *config.Settings) { s.Order = config.OrderOldest })
	for i := 0; i < 3; i++ {
		if i > 0 {
			h.p.Do(Next)
		}
		h.wait(t, 1)
		if h.p.Status().Current.ID == "bad" {
			t.Fatal("showed an unreadable photo")
		}
	}
}

func TestDisplayErrorRetries(t *testing.T) {
	h := start(t, makePhotos(t, 2), "", func(s *config.Settings) { s.Interval = config.Duration(time.Hour) })
	h.wait(t, 1) // first photo shown fine

	h.disp.mu.Lock()
	h.disp.fail = errors.New("spi timeout")
	h.disp.mu.Unlock()
	h.p.Do(Refresh)
	h.wait(t, 1)
	if h.p.Status().Error == "" {
		t.Fatal("display error not reported")
	}

	// Even with an hour-long interval, a failed refresh is retried after
	// RetryDelay; once the panel recovers the error clears.
	h.disp.mu.Lock()
	h.disp.fail = nil
	h.disp.mu.Unlock()
	h.wait(t, 1)
	if err := h.p.Status().Error; err != "" {
		t.Fatalf("error not cleared: %q", err)
	}
}

func TestClearPausesAndBlanks(t *testing.T) {
	h := start(t, makePhotos(t, 2), "", nil)
	h.wait(t, 1)
	h.p.Do(Clear)
	h.wait(t, 1)
	st := h.p.Status()
	if st.Screen != ScreenClear || !st.Paused {
		t.Fatalf("status = %+v", st)
	}
	img := h.disp.shown[len(h.disp.shown)-1]
	for _, px := range img.Pix {
		if px != 1 {
			t.Fatal("clear screen is not all white")
		}
	}
}

func TestPortraitIsRotatedToNative(t *testing.T) {
	h := start(t, makePhotos(t, 1), "", func(s *config.Settings) { s.Display.Orientation = config.Portrait })
	h.wait(t, 1)
	if b := h.disp.shown[0].Bounds(); b.Dx() != 80 || b.Dy() != 48 {
		t.Fatalf("native image is %v, want 80x48", b)
	}
}

func TestUpNext(t *testing.T) {
	h := start(t, makePhotos(t, 5), "", nil)
	h.wait(t, 1)
	up := h.p.UpNext(3)
	if len(up) != 3 {
		t.Fatalf("UpNext = %d", len(up))
	}
	h.p.Do(Next)
	h.wait(t, 1)
	if h.p.Status().Current.ID != up[0].ID {
		t.Fatalf("Next showed %s, UpNext promised %s", h.p.Status().Current.ID, up[0].ID)
	}
}

func TestCaption(t *testing.T) {
	d := time.Date(2025, 8, 3, 0, 0, 0, 0, time.UTC)
	cases := map[string]library.Photo{
		"Emerald Bay · Aug 2025": {Title: "Emerald Bay", TakenAt: d},
		"Aug 2025":               {TakenAt: d},
		"Emerald Bay":            {Title: "Emerald Bay"},
	}
	for want, ph := range cases {
		if got := Caption(ph); got != want {
			t.Errorf("Caption = %q, want %q", got, want)
		}
	}
}

type testWriter struct{ t *testing.T }

func (w testWriter) Write(b []byte) (int, error) { w.t.Log(string(b)); return len(b), nil }

type noPanelErr struct{}

func (noPanelErr) Error() string      { return "panel not connected: open /dev/spidev0.0: no such file" }
func (noPanelErr) Disconnected() bool { return true }

func TestRunsWithoutPanel(t *testing.T) {
	state := filepath.Join(t.TempDir(), "state.json")
	h := start(t, makePhotos(t, 3), state, func(s *config.Settings) { s.Interval = config.Duration(80 * time.Millisecond) })
	h.disp.mu.Lock()
	h.disp.fail = noPanelErr{}
	h.disp.mu.Unlock()

	// The schedule keeps advancing and the preview keeps updating.
	h.wait(t, 3)
	st := h.p.Status()
	if !st.PanelMissing || st.Error != "" || st.Current == nil || st.Screen != ScreenPhoto {
		t.Fatalf("status without panel = %+v", st)
	}
	if h.p.Preview() == nil {
		t.Fatal("no preview without a panel")
	}

	// Panel attached: the next refresh reaches it and the flag clears.
	h.disp.mu.Lock()
	h.disp.fail = nil
	h.disp.mu.Unlock()
	h.wait(t, 1)
	if h.p.Status().PanelMissing {
		t.Fatal("PanelMissing not cleared after a successful refresh")
	}
}

func TestRestartAfterNoPanelDrawsImmediately(t *testing.T) {
	photos := makePhotos(t, 2)
	state := filepath.Join(t.TempDir(), "state.json")
	h := start(t, photos, state, nil)
	h.disp.mu.Lock()
	h.disp.fail = noPanelErr{}
	h.disp.mu.Unlock()
	h.p.Do(Next)
	h.wait(t, 2) // the first show may have succeeded; the Next one had no panel
	h.stop()

	// Rebooted with the panel attached and a 30-minute interval: it must draw
	// now rather than wait, because nothing of ours is on the panel.
	h2 := start(t, photos, state, nil)
	h2.wait(t, 1)
	if h2.p.Status().PanelMissing {
		t.Fatal("still marked missing after drawing")
	}
}

// allIndex reports whether every pixel of img is palette index i.
func allIndex(img *image.Paletted, i uint8) bool {
	for _, px := range img.Pix {
		if px != i {
			return false
		}
	}
	return true
}

func TestPhotoAdjustmentIsApplied(t *testing.T) {
	photos := makePhotos(t, 1) // black PNGs
	h := start(t, photos, "", nil)
	h.wait(t, 1)
	if !allIndex(h.disp.shown[0], 0) {
		t.Fatal("unadjusted black photo is not all black")
	}

	h.adjMu.Lock()
	h.adj = map[string]adjust.Adjustment{photos[0].ID: {Brightness: 1}}
	h.adjMu.Unlock()
	h.p.Do(Refresh)
	h.wait(t, 1)
	if !allIndex(h.disp.shown[1], 1) {
		t.Fatal("brightness +100% did not turn the black photo white")
	}
}

func TestStatusUsesLatestPhotoDetails(t *testing.T) {
	photos := makePhotos(t, 1)
	state := filepath.Join(t.TempDir(), "state.json")
	h := start(t, photos, state, nil)
	h.wait(t, 1)
	h.stop()

	// The photo gained a name since it was shown and saved.
	photos[0].Name = "IMG_0001.png"
	h2 := start(t, photos, state, nil)
	h2.disp.expectNone(t)
	if st := h2.p.Status(); st.Current == nil || st.Current.Name != "IMG_0001.png" {
		t.Fatalf("current = %+v, want the library's name", st.Current)
	}
}
