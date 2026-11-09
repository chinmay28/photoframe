package api

import (
	"net/http"
	"testing"

	"github.com/chinmay28/photoframe/internal/adjust"
	"github.com/chinmay28/photoframe/internal/config"
	"github.com/chinmay28/photoframe/internal/library"
	"github.com/chinmay28/photoframe/internal/player"
)

func TestAdjustmentLifecycle(t *testing.T) {
	e := newEnv(t)
	e.lib.photos = []library.Photo{{ID: "p1", SourceID: "uploads", Name: "IMG_1.jpg"}}

	got := decode[AdjustmentView](t, e.do(t, "GET", "/api/photos/p1/adjustment", nil))
	if !got.Adjustment.IsZero() || got.Photo.Name != "IMG_1.jpg" || got.OnScreen {
		t.Fatalf("initial = %+v", got)
	}

	in := adjust.Adjustment{Rotate: 90, Fit: config.FitFit, Zoom: 1.5, Position: &adjust.Point{X: 0, Y: 1}, Brightness: 0.2}
	resp := e.do(t, "PUT", "/api/photos/p1/adjustment", in)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("PUT status %d", resp.StatusCode)
	}
	if out := decode[AdjustmentView](t, resp); out.Adjustment.Rotate != 90 || out.Adjustment.Position == nil {
		t.Fatalf("PUT = %+v", out)
	}
	if s := e.adj.Get("p1"); s.Zoom != 1.5 || s.Fit != config.FitFit {
		t.Fatalf("stored = %+v", s)
	}
	if len(e.player.cmds) != 0 {
		t.Fatalf("photo not on screen, but the frame was told to %v", e.player.cmds)
	}

	if resp := e.do(t, "DELETE", "/api/photos/p1/adjustment", nil); resp.StatusCode != http.StatusOK {
		t.Fatalf("DELETE status %d", resp.StatusCode)
	}
	if !e.adj.Get("p1").IsZero() {
		t.Fatal("reset kept the adjustment")
	}
}

func TestAdjustmentOnScreenRefreshesFrame(t *testing.T) {
	e := newEnv(t)
	ph := library.Photo{ID: "p1", SourceID: "uploads"}
	e.lib.photos = []library.Photo{ph}
	e.player.current = &ph

	out := decode[AdjustmentView](t, e.do(t, "PUT", "/api/photos/p1/adjustment", adjust.Adjustment{Rotate: 180}))
	if !out.OnScreen {
		t.Fatal("onScreen = false for the current photo")
	}
	if len(e.player.cmds) != 1 || e.player.cmds[0] != player.Refresh {
		t.Fatalf("cmds = %v, want refresh", e.player.cmds)
	}
}

func TestAdjustmentErrors(t *testing.T) {
	e := newEnv(t)
	e.lib.photos = []library.Photo{{ID: "p1", SourceID: "uploads"}}
	for _, tc := range []struct {
		method, path string
		body         any
		want         int
	}{
		{"GET", "/api/photos/nope/adjustment", nil, http.StatusNotFound},
		{"PUT", "/api/photos/nope/adjustment", adjust.Adjustment{Rotate: 90}, http.StatusNotFound},
		{"PUT", "/api/photos/p1/adjustment", adjust.Adjustment{Rotate: 45}, http.StatusUnprocessableEntity},
		{"PUT", "/api/photos/p1/adjustment", map[string]any{"tilt": 3}, http.StatusBadRequest},
	} {
		if resp := e.do(t, tc.method, tc.path, tc.body); resp.StatusCode != tc.want {
			t.Errorf("%s %s: status %d, want %d", tc.method, tc.path, resp.StatusCode, tc.want)
		}
	}
	if !e.adj.Get("p1").IsZero() {
		t.Fatal("a rejected request stored an adjustment")
	}
}

func TestImageFallsBackToThumbs(t *testing.T) {
	e := newEnv(t)
	e.lib.photos = []library.Photo{{ID: "p1", SourceID: "uploads"}}
	resp := e.do(t, "GET", "/api/photos/p1/image", nil)
	if resp.StatusCode != http.StatusOK || resp.Header.Get("Content-Type") != "image/jpeg" {
		t.Fatalf("image status %d %s", resp.StatusCode, resp.Header.Get("Content-Type"))
	}
}
