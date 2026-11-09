package api

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"testing/fstest"

	"github.com/chinmay28/photoframe/internal/adjust"
	"github.com/chinmay28/photoframe/internal/config"
	"github.com/chinmay28/photoframe/internal/library"
	"github.com/chinmay28/photoframe/internal/player"
	"github.com/chinmay28/photoframe/internal/sysinfo"
)

type fakeLib struct {
	mu     sync.Mutex
	photos []library.Photo
	synced []string
}

func (l *fakeLib) Photos() []library.Photo {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]library.Photo(nil), l.photos...)
}
func (l *fakeLib) Photo(id string) (library.Photo, bool) {
	for _, p := range l.Photos() {
		if p.ID == id {
			return p, true
		}
	}
	return library.Photo{}, false
}
func (l *fakeLib) Status() map[string]library.Status {
	return map[string]library.Status{"uploads": {Count: len(l.Photos())}}
}
func (l *fakeLib) SyncSource(_ context.Context, id string) error {
	l.mu.Lock()
	l.synced = append(l.synced, id)
	l.mu.Unlock()
	return nil
}

type fakePlayer struct {
	cmds    []player.Command
	current *library.Photo
	upNext  []library.Photo
	shown   []string
	moves   []string
}

func (p *fakePlayer) Show(id string) error {
	if id == "nope" {
		return player.ErrNoPhoto
	}
	p.shown = append(p.shown, id)
	return nil
}
func (p *fakePlayer) Move(id string, pos player.Position) error {
	if p.current != nil && p.current.ID == id {
		return player.ErrOnScreen
	}
	if id == "nope" {
		return player.ErrNoPhoto
	}
	p.moves = append(p.moves, id+":"+string(pos))
	return nil
}

func (p *fakePlayer) Do(c player.Command) bool { p.cmds = append(p.cmds, c); return true }
func (p *fakePlayer) Status() player.Status {
	return player.Status{Screen: player.ScreenPhoto, Current: p.current}
}
func (p *fakePlayer) Preview() []byte              { return []byte("png") }
func (p *fakePlayer) UpNext(n int) []library.Photo { return p.upNext }

type fakeThumbs struct{ removed []string }

func (f *fakeThumbs) Thumb(library.Photo) ([]byte, error) { return []byte("jpg"), nil }
func (f *fakeThumbs) Remove(id string)                    { f.removed = append(f.removed, id) }

type env struct {
	srv    *httptest.Server
	store  *config.Store
	adj    *adjust.Store
	lib    *fakeLib
	player *fakePlayer
	thumbs *fakeThumbs
	data   string
}

func newEnv(t *testing.T) *env {
	t.Helper()
	data := t.TempDir()
	store, err := config.Open(filepath.Join(data, "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	adj, err := adjust.Open(filepath.Join(data, "adjustments.json"))
	if err != nil {
		t.Fatal(err)
	}
	e := &env{store: store, adj: adj, lib: &fakeLib{}, player: &fakePlayer{}, thumbs: &fakeThumbs{}, data: data}
	s := &Server{
		Store: store, Library: e.lib, Player: e.player, Thumbs: e.thumbs, Adjustments: adj,
		UploadDir: func(id string) string { return filepath.Join(data, "uploads", id) },
		Device:    func() sysinfo.Info { return sysinfo.Info{Hostname: "photoframe"} },
		Static:    fstest.MapFS{"index.html": {Data: []byte("<!doctype html>app")}, "app.js": {Data: []byte("js")}},
	}
	e.srv = httptest.NewServer(s.Handler())
	t.Cleanup(e.srv.Close)
	return e
}

func (e *env) do(t *testing.T, method, path string, body any) *http.Response {
	t.Helper()
	var r io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		r = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, e.srv.URL+path, r)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	return resp
}

func decode[T any](t *testing.T, resp *http.Response) T {
	t.Helper()
	var v T
	if err := json.NewDecoder(resp.Body).Decode(&v); err != nil {
		t.Fatal(err)
	}
	return v
}

func TestGetAndPutSettings(t *testing.T) {
	e := newEnv(t)
	s := decode[config.Settings](t, e.do(t, "GET", "/api/settings", nil))
	s.Order = config.OrderNewest
	s.Sources = nil // ignored by PUT
	resp := e.do(t, "PUT", "/api/settings", s)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("PUT status %d", resp.StatusCode)
	}
	got := e.store.Get()
	if got.Order != config.OrderNewest || len(got.Sources) != 1 {
		t.Fatalf("settings after PUT: order=%s sources=%d", got.Order, len(got.Sources))
	}

	s.Interval = config.Duration(1)
	if resp := e.do(t, "PUT", "/api/settings", s); resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("invalid PUT status %d", resp.StatusCode)
	}
}

func TestSourcesCRUDAndKeyHandling(t *testing.T) {
	e := newEnv(t)
	resp := e.do(t, "POST", "/api/sources", config.Source{Name: "Family album", Type: config.SourceImmich, Enabled: true,
		URL: "http://immich.lan:2283", APIKey: "secret", AlbumID: "abc"})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create status %d", resp.StatusCode)
	}
	created := decode[config.Source](t, resp)
	if created.ID != "family-album" || created.APIKey != "" || !created.APIKeySet {
		t.Fatalf("created = %+v", created)
	}

	// Rename without resending the key keeps it.
	upd := created
	upd.Name = "Family"
	if resp := e.do(t, "PUT", "/api/sources/family-album", upd); resp.StatusCode != http.StatusOK {
		t.Fatalf("update status %d", resp.StatusCode)
	}
	if src, _ := e.store.Get().Source("family-album"); src.APIKey != "secret" || src.Name != "Family" {
		t.Fatalf("stored = %+v", src)
	}

	// Pointing at another server without the key is refused.
	upd.URL = "http://attacker.example"
	if resp := e.do(t, "PUT", "/api/sources/family-album", upd); resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("URL change without key: status %d", resp.StatusCode)
	}

	list := decode[[]SourceView](t, e.do(t, "GET", "/api/sources", nil))
	if len(list) != 2 || list[0].Status == nil {
		t.Fatalf("list = %+v", list)
	}
	for _, v := range list {
		if v.APIKey != "" {
			t.Fatal("API key leaked in list")
		}
	}

	if resp := e.do(t, "DELETE", "/api/sources/family-album", nil); resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete status %d", resp.StatusCode)
	}
	if resp := e.do(t, "DELETE", "/api/sources/family-album", nil); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("second delete status %d", resp.StatusCode)
	}
}

func TestControl(t *testing.T) {
	e := newEnv(t)
	if resp := e.do(t, "POST", "/api/control/next", nil); resp.StatusCode != http.StatusAccepted {
		t.Fatalf("status %d", resp.StatusCode)
	}
	if resp := e.do(t, "POST", "/api/control/explode", nil); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown command status %d", resp.StatusCode)
	}
	if len(e.player.cmds) != 1 || e.player.cmds[0] != player.Next {
		t.Fatalf("cmds = %v", e.player.cmds)
	}
}

func TestCrossOriginWritesRefused(t *testing.T) {
	e := newEnv(t)
	req, _ := http.NewRequest("POST", e.srv.URL+"/api/control/next", nil)
	req.Header.Set("Origin", "https://evil.example")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status %d, want 403", resp.StatusCode)
	}

	req, _ = http.NewRequest("POST", e.srv.URL+"/api/control/next", nil)
	req.Header.Set("Sec-Fetch-Site", "cross-site")
	resp, _ = http.DefaultClient.Do(req)
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("Sec-Fetch-Site status %d, want 403", resp.StatusCode)
	}
	if len(e.player.cmds) != 0 {
		t.Fatal("cross-origin command reached the player")
	}
}

func TestJSONRequired(t *testing.T) {
	e := newEnv(t)
	req, _ := http.NewRequest("PUT", e.srv.URL+"/api/settings", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "text/plain")
	resp, _ := http.DefaultClient.Do(req)
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnsupportedMediaType {
		t.Fatalf("status %d", resp.StatusCode)
	}
}

func uploadBody(t *testing.T, files map[string][]byte) (*bytes.Buffer, string) {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	for name, data := range files {
		fw, _ := mw.CreateFormFile("photos", name)
		fw.Write(data)
	}
	mw.Close()
	return &buf, mw.FormDataContentType()
}

func TestUpload(t *testing.T) {
	e := newEnv(t)
	var img bytes.Buffer
	png.Encode(&img, image.NewRGBA(image.Rect(0, 0, 2, 2)))
	body, ct := uploadBody(t, map[string][]byte{
		"../../etc/Beach Day.PNG": img.Bytes(),
		"notes.txt":               []byte("hi"),
		"fake.jpg":                []byte("not really"),
	})
	resp, err := http.Post(e.srv.URL+"/api/sources/uploads/photos", ct, body)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status %d", resp.StatusCode)
	}
	res := decode[UploadResult](t, resp)
	if len(res.Saved) != 1 || res.Saved[0] != "Beach Day.png" || len(res.Rejected) != 2 {
		t.Fatalf("result = %+v", res)
	}
	if _, err := os.Stat(filepath.Join(e.data, "uploads", "uploads", "Beach Day.png")); err != nil {
		t.Fatal("file not stored in the upload folder")
	}
	if len(e.lib.synced) != 1 {
		t.Fatal("library not rescanned after upload")
	}

	// Same name again gets a suffix.
	body, ct = uploadBody(t, map[string][]byte{"Beach Day.png": img.Bytes()})
	resp2, err := http.Post(e.srv.URL+"/api/sources/uploads/photos", ct, body)
	if err != nil {
		t.Fatal(err)
	}
	defer resp2.Body.Close()
	if res := decode[UploadResult](t, resp2); len(res.Saved) != 1 || res.Saved[0] != "Beach Day-2.png" {
		t.Fatalf("second upload = %+v", res)
	}
}

func TestDeleteUploadedPhoto(t *testing.T) {
	e := newEnv(t)
	dir := filepath.Join(e.data, "uploads", "uploads")
	os.MkdirAll(dir, 0o755)
	inside := filepath.Join(dir, "a.png")
	os.WriteFile(inside, []byte("x"), 0o644)
	outside := filepath.Join(e.data, "settings.json")
	e.lib.photos = []library.Photo{
		{ID: "uploads-1", SourceID: "uploads", Path: inside},
		{ID: "uploads-2", SourceID: "uploads", Path: outside},
	}
	e.adj.Set("uploads-1", adjust.Adjustment{Rotate: 90})
	if resp := e.do(t, "DELETE", "/api/photos/uploads-1", nil); resp.StatusCode != http.StatusNoContent {
		t.Fatalf("status %d", resp.StatusCode)
	}
	if _, err := os.Stat(inside); !os.IsNotExist(err) {
		t.Fatal("file not deleted")
	}
	if !e.adj.Get("uploads-1").IsZero() {
		t.Fatal("deleted photo's adjustment was kept")
	}
	if resp := e.do(t, "DELETE", "/api/photos/uploads-2", nil); resp.StatusCode == http.StatusNoContent {
		t.Fatal("deleted a file outside the upload folder")
	}
	if _, err := os.Stat(outside); err != nil {
		t.Fatal("file outside upload folder was removed")
	}
}

func TestStatusPreviewThumbDevice(t *testing.T) {
	e := newEnv(t)
	e.lib.photos = []library.Photo{{ID: "p1", SourceID: "uploads"}}
	st := decode[StatusResponse](t, e.do(t, "GET", "/api/status", nil))
	if st.Name != "Photoframe" || st.Player.Screen != player.ScreenPhoto || st.UpNext == nil {
		t.Fatalf("status = %+v", st)
	}
	if resp := e.do(t, "GET", "/api/preview.png", nil); resp.Header.Get("Content-Type") != "image/png" {
		t.Fatal("preview content type")
	}
	if resp := e.do(t, "GET", "/api/photos/p1/thumb", nil); resp.StatusCode != http.StatusOK {
		t.Fatalf("thumb status %d", resp.StatusCode)
	}
	if resp := e.do(t, "GET", "/api/photos/nope/thumb", nil); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("missing thumb status %d", resp.StatusCode)
	}
	if d := decode[sysinfo.Info](t, e.do(t, "GET", "/api/device", nil)); d.Hostname != "photoframe" {
		t.Fatalf("device = %+v", d)
	}
	if resp := e.do(t, "POST", "/api/device/restart", nil); resp.StatusCode != http.StatusNotImplemented {
		t.Fatalf("restart without hook: %d", resp.StatusCode)
	}
}

func TestStaticFallsBackToIndex(t *testing.T) {
	e := newEnv(t)
	resp := e.do(t, "GET", "/sources", nil)
	b, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(b), "app") {
		t.Fatalf("SPA fallback body = %q", b)
	}
	if resp := e.do(t, "GET", "/api/unknown", nil); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown api status %d", resp.StatusCode)
	}
}

func TestUniqueID(t *testing.T) {
	s := config.Default()
	if id := uniqueID(s, "Uploads!"); id != "uploads-2" {
		t.Fatalf("uniqueID = %q", id)
	}
	if id := uniqueID(s, "  ✨  "); id != "source" {
		t.Fatalf("uniqueID = %q", id)
	}
}
