package adjust

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/chinmay28/photoframe/internal/config"
)

func TestZeroAdjustmentIsZero(t *testing.T) {
	if !(Adjustment{}).IsZero() {
		t.Fatal("zero value should be zero")
	}
	if !(Adjustment{Zoom: 1}).IsZero() {
		t.Fatal("zoom 1 changes nothing")
	}
	for _, a := range []Adjustment{
		{Rotate: 90}, {Fit: config.FitFit}, {Zoom: 2}, {Position: &Point{0.5, 0.5}}, {Brightness: 0.1},
	} {
		if a.IsZero() {
			t.Fatalf("%+v reported zero", a)
		}
	}
}

func TestValidate(t *testing.T) {
	valid := []Adjustment{
		{},
		{Rotate: 270, Fit: config.FitFill, Zoom: 4, Position: &Point{0, 1}, Brightness: -1},
		{Zoom: 1},
	}
	for _, a := range valid {
		if err := a.Validate(); err != nil {
			t.Errorf("%+v: %v", a, err)
		}
	}
	invalid := []Adjustment{
		{Rotate: 45},
		{Rotate: -90},
		{Fit: "stretch"},
		{Zoom: 0.5},
		{Zoom: 4.5},
		{Position: &Point{-0.1, 0.5}},
		{Position: &Point{0.5, 1.1}},
		{Brightness: 1.5},
	}
	for _, a := range invalid {
		if a.Validate() == nil {
			t.Errorf("%+v: want error", a)
		}
	}
}

func TestStoreRoundTripAndPersistence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "adjustments.json")
	st, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if !st.Get("p1").IsZero() {
		t.Fatal("unknown photo should have no adjustment")
	}
	want := Adjustment{Rotate: 90, Zoom: 2, Position: &Point{0.25, 0.75}}
	if _, err := st.Set("p1", want); err != nil {
		t.Fatal(err)
	}

	again, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	got := again.Get("p1")
	if got.Rotate != 90 || got.Zoom != 2 || got.Position == nil || *got.Position != *want.Position {
		t.Fatalf("reloaded %+v, want %+v", got, want)
	}
}

func TestStoreGetReturnsCopy(t *testing.T) {
	st, _ := Open(filepath.Join(t.TempDir(), "a.json"))
	st.Set("p", Adjustment{Position: &Point{0.1, 0.1}})
	a := st.Get("p")
	a.Position.X = 0.9
	if st.Get("p").Position.X != 0.1 {
		t.Fatal("mutating a returned adjustment changed the store")
	}
}

func TestStoreZeroRemovesEntry(t *testing.T) {
	path := filepath.Join(t.TempDir(), "a.json")
	st, _ := Open(path)
	st.Set("p", Adjustment{Rotate: 180})
	if _, err := st.Set("p", Adjustment{Zoom: 1}); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(path)
	if string(b) != "{\n  \"photos\": {}\n}\n" {
		t.Fatalf("file still lists the photo:\n%s", b)
	}
	if err := st.Delete("missing"); err != nil {
		t.Fatalf("deleting an unknown photo: %v", err)
	}
}

func TestStoreRejectsInvalid(t *testing.T) {
	st, _ := Open(filepath.Join(t.TempDir(), "a.json"))
	st.Set("p", Adjustment{Rotate: 90})
	_, err := st.Set("p", Adjustment{Rotate: 12})
	var ve *config.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("err = %v, want ValidationError", err)
	}
	if st.Get("p").Rotate != 90 {
		t.Fatal("a rejected update changed the stored adjustment")
	}
	if _, err := st.Set("", Adjustment{Rotate: 90}); !errors.As(err, &ve) {
		t.Fatalf("empty id: err = %v", err)
	}
}

func TestOpenDropsInvalidEntries(t *testing.T) {
	path := filepath.Join(t.TempDir(), "a.json")
	os.WriteFile(path, []byte(`{"photos":{"good":{"rotate":90},"bad":{"rotate":7},"noop":{"zoom":1}}}`), 0o600)
	st, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if st.Get("good").Rotate != 90 || !st.Get("bad").IsZero() || !st.Get("noop").IsZero() {
		t.Fatalf("good=%+v bad=%+v", st.Get("good"), st.Get("bad"))
	}
}

func TestOpenCorruptFileFails(t *testing.T) {
	path := filepath.Join(t.TempDir(), "a.json")
	os.WriteFile(path, []byte("{"), 0o600)
	if _, err := Open(path); err == nil {
		t.Fatal("want error for corrupt file")
	}
}
