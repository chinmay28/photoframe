package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDefaultIsValid(t *testing.T) {
	if err := Default().Validate(); err != nil {
		t.Fatalf("Default().Validate() = %v", err)
	}
}

func TestValidateRejects(t *testing.T) {
	cases := map[string]func(*Settings){
		"short interval":   func(s *Settings) { s.Interval = Duration(time.Minute) },
		"bad order":        func(s *Settings) { s.Order = "random" },
		"bad fit":          func(s *Settings) { s.Fit = "zoom" },
		"bad border":       func(s *Settings) { s.Border = "red" },
		"empty name":       func(s *Settings) { s.Name = " " },
		"same quiet times": func(s *Settings) { s.Quiet.Until = s.Quiet.From },
		"bad panel":        func(s *Settings) { s.Display.Panel = "lcd" },
		"saturation range": func(s *Settings) { s.Display.Saturation = 1.5 },
		"dup source id": func(s *Settings) {
			s.Sources = append(s.Sources, s.Sources[0])
		},
		"folder relative path": func(s *Settings) {
			s.Sources = append(s.Sources, Source{ID: "nas", Name: "NAS", Type: SourceFolder, Path: "photos"})
		},
		"immich without key": func(s *Settings) {
			s.Sources = append(s.Sources, Source{ID: "im", Name: "Im", Type: SourceImmich, URL: "http://immich:2283", AlbumID: "a"})
		},
		"url not http": func(s *Settings) {
			s.Sources = append(s.Sources, Source{ID: "u", Name: "U", Type: SourceURL, URL: "file:///etc/passwd"})
		},
		"bad id": func(s *Settings) {
			s.Sources = append(s.Sources, Source{ID: "Bad ID", Name: "x", Type: SourceUpload})
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			s := Default()
			mutate(&s)
			if err := s.Validate(); err == nil {
				t.Fatal("Validate() = nil, want error")
			}
		})
	}
}

func TestJSONRoundTrip(t *testing.T) {
	in := Default()
	in.Sources = append(in.Sources, Source{ID: "im", Name: "Family", Type: SourceImmich, Enabled: true,
		URL: "http://immich.lan:2283", APIKey: "k", AlbumID: "abc"})
	b, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"interval":"30m0s"`) || !strings.Contains(string(b), `"from":"22:00"`) {
		t.Fatalf("unexpected encoding: %s", b)
	}
	var out Settings
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	if out.Interval != in.Interval || out.Quiet != in.Quiet || len(out.Sources) != 2 || out.Sources[1] != in.Sources[1] {
		t.Fatalf("round trip mismatch: %+v", out)
	}
}

func TestClockParse(t *testing.T) {
	if c, err := ParseClock("07:30"); err != nil || c != 450 {
		t.Fatalf("ParseClock = %v, %v", c, err)
	}
	if _, err := ParseClock("25:00"); err == nil {
		t.Fatal("want error for 25:00")
	}
}

func TestRedacted(t *testing.T) {
	s := Default()
	s.Sources = append(s.Sources, Source{ID: "im", Name: "x", Type: SourceImmich, APIKey: "secret"})
	r := s.Redacted()
	if r.Sources[1].APIKey != "" || !r.Sources[1].APIKeySet {
		t.Fatalf("redaction failed: %+v", r.Sources[1])
	}
	if s.Sources[1].APIKey != "secret" {
		t.Fatal("Redacted mutated the original")
	}
}

func TestStoreCreatesUpdatesAndNotifies(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	st, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("settings file not created: %v", err)
	}
	ch := st.Subscribe()

	if _, err := st.Update(func(s *Settings) error { s.Order = OrderNewest; return nil }); err != nil {
		t.Fatal(err)
	}
	select {
	case <-ch:
	default:
		t.Fatal("no notification after Update")
	}

	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if reopened.Get().Order != OrderNewest {
		t.Fatal("update not persisted")
	}
}

func TestStoreRejectsInvalidUpdate(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "s.json"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = st.Update(func(s *Settings) error { s.Order = "nope"; return nil })
	var ve *ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("Update error = %v, want ValidationError", err)
	}
	if st.Get().Order != OrderShuffle {
		t.Fatal("invalid update was applied")
	}
}

func TestStoreGetReturnsCopy(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "s.json"))
	if err != nil {
		t.Fatal(err)
	}
	s := st.Get()
	s.Sources[0].Name = "changed"
	if st.Get().Sources[0].Name == "changed" {
		t.Fatal("Get exposed internal slice")
	}
}
