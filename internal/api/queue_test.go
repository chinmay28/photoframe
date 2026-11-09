package api

import (
	"net/http"
	"testing"
	"time"

	"github.com/chinmay28/photoframe/internal/library"
)

func TestQueueListsEveryPhoto(t *testing.T) {
	e := newEnv(t)
	day := func(d int) time.Time { return time.Date(2026, 1, d, 0, 0, 0, 0, time.UTC) }
	cur := library.Photo{ID: "c", TakenAt: day(1)}
	e.lib.photos = []library.Photo{cur, {ID: "u1", TakenAt: day(2)}, {ID: "r-old", TakenAt: day(3)}, {ID: "r-new", TakenAt: day(9)}}
	e.player.current = &cur
	e.player.upNext = []library.Photo{{ID: "u1"}}

	q := decode[QueueResponse](t, e.do(t, "GET", "/api/queue", nil))
	if q.Current == nil || q.Current.ID != "c" || len(q.UpNext) != 1 || q.UpNext[0].ID != "u1" {
		t.Fatalf("queue = %+v", q)
	}
	if len(q.Rest) != 2 || q.Rest[0].ID != "r-new" || q.Rest[1].ID != "r-old" {
		t.Fatalf("rest = %+v, want newest first", q.Rest)
	}
}

func TestShowPhoto(t *testing.T) {
	e := newEnv(t)
	if resp := e.do(t, "POST", "/api/photos/p1/show", nil); resp.StatusCode != http.StatusAccepted {
		t.Fatalf("status %d", resp.StatusCode)
	}
	if resp := e.do(t, "POST", "/api/photos/nope/show", nil); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown photo status %d", resp.StatusCode)
	}
	if len(e.player.shown) != 1 || e.player.shown[0] != "p1" {
		t.Fatalf("shown = %v", e.player.shown)
	}
}

func TestMovePhoto(t *testing.T) {
	e := newEnv(t)
	cur := library.Photo{ID: "c"}
	e.player.current = &cur
	for _, tc := range []struct {
		id   string
		body any
		want int
	}{
		{"p1", MoveRequest{Position: "front"}, http.StatusNoContent},
		{"p2", MoveRequest{Position: "back"}, http.StatusNoContent},
		{"p1", MoveRequest{Position: "middle"}, http.StatusUnprocessableEntity},
		{"c", MoveRequest{Position: "front"}, http.StatusConflict},
		{"nope", MoveRequest{Position: "back"}, http.StatusNotFound},
		{"p1", map[string]any{"where": "front"}, http.StatusBadRequest},
	} {
		if resp := e.do(t, "POST", "/api/photos/"+tc.id+"/queue", tc.body); resp.StatusCode != tc.want {
			t.Errorf("%s %v: status %d, want %d", tc.id, tc.body, resp.StatusCode, tc.want)
		}
	}
	if got := e.player.moves; len(got) != 2 || got[0] != "p1:front" || got[1] != "p2:back" {
		t.Fatalf("moves = %v", got)
	}
}
