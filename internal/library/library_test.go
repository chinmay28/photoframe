package library

import (
	"context"
	"errors"
	"testing"

	"github.com/chinmay28/photoframe/internal/config"
)

type stubSource struct {
	photos []Photo
	err    error
	calls  int
}

func (s *stubSource) Sync(context.Context) ([]Photo, error) {
	s.calls++
	return s.photos, s.err
}

func cfg(id string, enabled bool) config.Source {
	return config.Source{ID: id, Name: id, Type: config.SourceUpload, Enabled: enabled}
}

func TestLibraryAggregatesEnabledSources(t *testing.T) {
	stubs := map[string]*stubSource{
		"a": {photos: []Photo{{ID: "a-2"}, {ID: "a-1"}}},
		"b": {photos: []Photo{{ID: "b-1"}}},
		"c": {photos: []Photo{{ID: "c-1"}}},
	}
	l := New(func(c config.Source) (Source, error) { return stubs[c.ID], nil })
	l.Configure([]config.Source{cfg("a", true), cfg("b", true), cfg("c", false)})
	l.SyncAll(context.Background())

	got := l.Photos()
	if len(got) != 3 || got[0].ID != "a-1" || got[2].ID != "b-1" {
		t.Fatalf("Photos() = %+v", got)
	}
	if _, ok := l.Photo("c-1"); ok {
		t.Fatal("disabled source contributed photos")
	}
	if st := l.Status()["a"]; st.Count != 2 || st.LastSync.IsZero() {
		t.Fatalf("status = %+v", st)
	}
}

func TestLibraryKeepsCachedPhotosOnError(t *testing.T) {
	s := &stubSource{photos: []Photo{{ID: "x-1"}}}
	l := New(func(config.Source) (Source, error) { return s, nil })
	l.Configure([]config.Source{cfg("x", true)})
	l.SyncAll(context.Background())

	s.photos, s.err = nil, errors.New("offline")
	if err := l.SyncSource(context.Background(), "x"); err == nil {
		t.Fatal("want error")
	}
	if len(l.Photos()) != 1 {
		t.Fatal("photos dropped after a failed sync")
	}
	if l.Status()["x"].Error != "offline" {
		t.Fatalf("status = %+v", l.Status()["x"])
	}
}

func TestConfigureReusesUnchangedSources(t *testing.T) {
	built := 0
	l := New(func(config.Source) (Source, error) {
		built++
		return &stubSource{photos: []Photo{{ID: "p"}}}, nil
	})
	l.Configure([]config.Source{cfg("x", true)})
	l.SyncAll(context.Background())
	l.Configure([]config.Source{cfg("x", true)})
	if built != 1 || len(l.Photos()) != 1 {
		t.Fatalf("built=%d photos=%d; unchanged source should be reused", built, len(l.Photos()))
	}
	changed := cfg("x", true)
	changed.Name = "renamed"
	l.Configure([]config.Source{changed})
	if built != 2 || len(l.Photos()) != 0 {
		t.Fatalf("built=%d photos=%d; changed source should be rebuilt", built, len(l.Photos()))
	}
}

func TestFactoryErrorIsReported(t *testing.T) {
	l := New(func(config.Source) (Source, error) { return nil, errors.New("bad config") })
	l.Configure([]config.Source{cfg("x", true)})
	if err := l.SyncSource(context.Background(), "x"); err == nil {
		t.Fatal("want error")
	}
	if l.Status()["x"].Error != "bad config" {
		t.Fatalf("status = %+v", l.Status()["x"])
	}
}

func TestChangedFires(t *testing.T) {
	l := New(func(config.Source) (Source, error) { return &stubSource{}, nil })
	l.Configure(nil)
	select {
	case <-l.Changed():
	default:
		t.Fatal("Configure should signal Changed")
	}
}

func TestPhotoIDStable(t *testing.T) {
	if PhotoID("s", "a/b.jpg") != PhotoID("s", "a/b.jpg") || PhotoID("s", "a") == PhotoID("s", "b") {
		t.Fatal("PhotoID not stable/unique")
	}
}
