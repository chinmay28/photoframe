package api

import (
	"io"
	"net/http"
	"testing"

	"github.com/chinmay28/photoframe/internal/library"
)

// get fetches path, sending tag as If-None-Match when it is not empty.
func (e *env) get(t *testing.T, path, tag string) *http.Response {
	t.Helper()
	req, _ := http.NewRequest("GET", e.srv.URL+path, nil)
	if tag != "" {
		req.Header.Set("If-None-Match", tag)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	return resp
}

// Every GET the app repeats must be answerable with 304: polling an idle
// frame should cost headers, not bodies.
func TestRepeatedGETsAreNotModified(t *testing.T) {
	e := newEnv(t)
	e.lib.photos = []library.Photo{{ID: "p1", SourceID: "uploads"}}
	paths := map[string]string{
		"/api/status":               revalidate,
		"/api/settings":             revalidate,
		"/api/sources":              revalidate,
		"/api/photos":               revalidate,
		"/api/queue":                revalidate,
		"/api/photos/p1/adjustment": revalidate,
		"/api/preview.png":          revalidate,
		"/api/photos/p1/thumb":      dayLong,
		"/api/photos/p1/image":      dayLong,
		"/":                         revalidate,
		"/app.js":                   revalidate,
		"/sources":                  revalidate, // SPA fallback to index.html
	}
	for path, cc := range paths {
		first := e.get(t, path, "")
		body, _ := io.ReadAll(first.Body)
		tag := first.Header.Get("ETag")
		if first.StatusCode != http.StatusOK || tag == "" || len(body) == 0 {
			t.Errorf("%s: status %d, ETag %q, %d bytes", path, first.StatusCode, tag, len(body))
			continue
		}
		if got := first.Header.Get("Cache-Control"); got != cc {
			t.Errorf("%s: Cache-Control %q, want %q", path, got, cc)
		}
		again := e.get(t, path, tag)
		if b, _ := io.ReadAll(again.Body); again.StatusCode != http.StatusNotModified || len(b) != 0 {
			t.Errorf("%s: revalidation gave %d with %d bytes, want 304 and none", path, again.StatusCode, len(b))
		}
		if stale := e.get(t, path, `"something-else"`); stale.StatusCode != http.StatusOK {
			t.Errorf("%s: a stale ETag gave %d, want 200", path, stale.StatusCode)
		}
	}
}

// A changed answer must not be mistaken for the cached one.
func TestETagChangesWithContent(t *testing.T) {
	e := newEnv(t)
	before := e.get(t, "/api/status", "").Header.Get("ETag")
	e.player.upNext = []library.Photo{{ID: "p2", SourceID: "uploads"}}
	after := e.get(t, "/api/status", before)
	if after.StatusCode != http.StatusOK || after.Header.Get("ETag") == before {
		t.Fatalf("status %d, ETag %q (was %q)", after.StatusCode, after.Header.Get("ETag"), before)
	}
}

func TestETagMatches(t *testing.T) {
	for header, want := range map[string]bool{
		`"abc"`:          true,
		`W/"abc"`:        true,
		`"x", "abc"`:     true,
		`"x" , W/"abc" `: true,
		`*`:              true,
		`"abcd"`:         false,
		`abc`:            false,
		``:               false,
	} {
		r, _ := http.NewRequest("GET", "/", nil)
		if header != "" {
			r.Header.Set("If-None-Match", header)
		}
		if got := etagMatches(r, `"abc"`); got != want {
			t.Errorf("If-None-Match %q: %v, want %v", header, got, want)
		}
	}
}
