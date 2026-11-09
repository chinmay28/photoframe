// Package config defines the photoframe's user-editable settings, their
// validation rules, and a file-backed store that notifies subscribers when
// settings change.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// Order selects how the next photo is chosen.
type Order string

const (
	OrderShuffle   Order = "shuffle"   // random, no repeats until every photo was shown
	OrderNewest    Order = "newest"    // by date taken, newest first
	OrderOldest    Order = "oldest"    // by date taken, oldest first
	OrderOnThisDay Order = "onthisday" // photos taken around today's date in any year
)

// Fit selects how a photo is fitted to the panel.
type Fit string

const (
	FitSmart Fit = "smart" // crop to fill, keeping the most detailed region
	FitFill  Fit = "fill"  // crop to fill, centred
	FitFit   Fit = "fit"   // show the whole photo with a border
)

// Dither selects the error-diffusion or ordered dithering method.
type Dither string

const (
	DitherFloydSteinberg Dither = "fs"
	DitherAtkinson       Dither = "atkinson"
	DitherOrdered        Dither = "ordered"
)

// Orientation is how the frame hangs on the wall.
type Orientation string

const (
	Landscape Orientation = "landscape"
	Portrait  Orientation = "portrait"
)

// Panel identifies the attached e-ink panel variant.
type Panel string

const (
	PanelSpectra6 Panel = "spectra6" // Inky Impression 7.3" (2025, Spectra 6)
	PanelACeP7    Panel = "acep7"    // Inky Impression 7.3" (2023, 7-colour ACeP)
)

// SourceType is the kind of photo source.
type SourceType string

const (
	SourceUpload SourceType = "upload" // photos uploaded through the app, stored on the frame
	SourceFolder SourceType = "folder" // a local or mounted (SMB/NFS) directory
	SourceImmich SourceType = "immich" // an Immich album
	SourceURL    SourceType = "url"    // a single image URL that is re-fetched periodically
)

// Border colours available for FitFit.
const (
	BorderWhite = "white"
	BorderBlack = "black"
)

// Interval limits. E-ink full refreshes are slow and visible, so very short
// intervals are refused.
const (
	MinInterval = 5 * time.Minute
	MaxInterval = 7 * 24 * time.Hour
)

// Settings is the complete user configuration.
type Settings struct {
	Name     string   `json:"name"`
	Interval Duration `json:"interval"`
	Order    Order    `json:"order"`
	Fit      Fit      `json:"fit"`
	Border   string   `json:"border"`
	Caption  bool     `json:"caption"`
	Quiet    Quiet    `json:"quiet"`
	Display  Display  `json:"display"`
	Sources  []Source `json:"sources"`
}

// Quiet hours: no automatic refreshes between From and Until (local time).
// The window may wrap past midnight.
type Quiet struct {
	Enabled bool      `json:"enabled"`
	From    ClockTime `json:"from"`
	Until   ClockTime `json:"until"`
}

// Display holds panel and image tuning settings.
type Display struct {
	Panel       Panel       `json:"panel"`
	Orientation Orientation `json:"orientation"`
	Dither      Dither      `json:"dither"`
	// Saturation and Contrast are adjustments in [-1, 1]; 0 leaves the image unchanged.
	Saturation float64 `json:"saturation"`
	Contrast   float64 `json:"contrast"`
}

// Source configures one photo source. Which fields apply depends on Type.
type Source struct {
	ID      string     `json:"id"`
	Name    string     `json:"name"`
	Type    SourceType `json:"type"`
	Enabled bool       `json:"enabled"`
	Path    string     `json:"path,omitempty"`    // folder
	URL     string     `json:"url,omitempty"`     // immich base URL, or image URL
	APIKey  string     `json:"apiKey,omitempty"`  // immich
	AlbumID string     `json:"albumId,omitempty"` // immich
	// APIKeySet is output-only: it reports that a key is stored while the
	// key itself is redacted.
	APIKeySet bool `json:"apiKeySet,omitempty"`
}

// Default returns the settings used on first boot.
func Default() Settings {
	return Settings{
		Name:     "Photoframe",
		Interval: Duration(30 * time.Minute),
		Order:    OrderShuffle,
		Fit:      FitSmart,
		Border:   BorderWhite,
		Caption:  true,
		Quiet:    Quiet{Enabled: true, From: MustClock("22:00"), Until: MustClock("07:00")},
		Display: Display{
			Panel:       PanelSpectra6,
			Orientation: Landscape,
			Dither:      DitherFloydSteinberg,
		},
		Sources: []Source{{ID: "uploads", Name: "Uploaded from phones", Type: SourceUpload, Enabled: true}},
	}
}

// Clone returns a deep copy.
func (s Settings) Clone() Settings {
	s.Sources = append([]Source(nil), s.Sources...)
	return s
}

// Source looks up a source by ID.
func (s Settings) Source(id string) (Source, bool) {
	for _, src := range s.Sources {
		if src.ID == id {
			return src, true
		}
	}
	return Source{}, false
}

// Redacted returns a copy safe to send to clients: secrets are removed and
// APIKeySet reports whether one is stored.
func (s Settings) Redacted() Settings {
	out := s.Clone()
	for i := range out.Sources {
		out.Sources[i].APIKeySet = out.Sources[i].APIKey != ""
		out.Sources[i].APIKey = ""
	}
	return out
}

var idPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,39}$`)

// ValidID reports whether id is usable as a source ID.
func ValidID(id string) bool { return idPattern.MatchString(id) }

// Validate checks every field and returns all problems joined together.
func (s Settings) Validate() error {
	var errs []error
	add := func(format string, a ...any) { errs = append(errs, fmt.Errorf(format, a...)) }

	if strings.TrimSpace(s.Name) == "" || len(s.Name) > 64 {
		add("name: must be 1-64 characters")
	}
	if d := s.Interval.D(); d < MinInterval || d > MaxInterval {
		add("interval: must be between %s and %s", MinInterval, MaxInterval)
	}
	switch s.Order {
	case OrderShuffle, OrderNewest, OrderOldest, OrderOnThisDay:
	default:
		add("order: unknown value %q", s.Order)
	}
	switch s.Fit {
	case FitSmart, FitFill, FitFit:
	default:
		add("fit: unknown value %q", s.Fit)
	}
	if s.Border != BorderWhite && s.Border != BorderBlack {
		add("border: must be %q or %q", BorderWhite, BorderBlack)
	}
	if !s.Quiet.From.Valid() || !s.Quiet.Until.Valid() {
		add("quiet: invalid time")
	} else if s.Quiet.Enabled && s.Quiet.From == s.Quiet.Until {
		add("quiet: from and until must differ")
	}
	switch s.Display.Panel {
	case PanelSpectra6, PanelACeP7:
	default:
		add("display.panel: unknown value %q", s.Display.Panel)
	}
	switch s.Display.Orientation {
	case Landscape, Portrait:
	default:
		add("display.orientation: unknown value %q", s.Display.Orientation)
	}
	switch s.Display.Dither {
	case DitherFloydSteinberg, DitherAtkinson, DitherOrdered:
	default:
		add("display.dither: unknown value %q", s.Display.Dither)
	}
	if s.Display.Saturation < -1 || s.Display.Saturation > 1 {
		add("display.saturation: must be within [-1, 1]")
	}
	if s.Display.Contrast < -1 || s.Display.Contrast > 1 {
		add("display.contrast: must be within [-1, 1]")
	}

	seen := map[string]bool{}
	for i, src := range s.Sources {
		if err := src.Validate(); err != nil {
			add("sources[%d]: %w", i, err)
		}
		if seen[src.ID] {
			add("sources[%d]: duplicate id %q", i, src.ID)
		}
		seen[src.ID] = true
	}
	return errors.Join(errs...)
}

// Validate checks a single source.
func (s Source) Validate() error {
	if !ValidID(s.ID) {
		return fmt.Errorf("id %q: use lower-case letters, digits and dashes", s.ID)
	}
	if strings.TrimSpace(s.Name) == "" || len(s.Name) > 64 {
		return errors.New("name: must be 1-64 characters")
	}
	switch s.Type {
	case SourceUpload:
	case SourceFolder:
		if !filepath.IsAbs(s.Path) {
			return errors.New("path: must be an absolute directory path")
		}
	case SourceImmich:
		if err := checkHTTPURL(s.URL); err != nil {
			return err
		}
		if s.APIKey == "" {
			return errors.New("apiKey: required for immich")
		}
		if s.AlbumID == "" || strings.ContainsAny(s.AlbumID, "/?#") {
			return errors.New("albumId: required for immich")
		}
	case SourceURL:
		if err := checkHTTPURL(s.URL); err != nil {
			return err
		}
	default:
		return fmt.Errorf("type: unknown value %q", s.Type)
	}
	return nil
}

func checkHTTPURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return fmt.Errorf("url: %q is not an http(s) URL", raw)
	}
	return nil
}

// Duration is a time.Duration encoded in JSON as a string such as "30m".
type Duration time.Duration

// D returns the value as a time.Duration.
func (d Duration) D() time.Duration { return time.Duration(d) }

func (d Duration) MarshalJSON() ([]byte, error) {
	return json.Marshal(time.Duration(d).String())
}

func (d *Duration) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return fmt.Errorf("duration must be a string like \"30m\": %w", err)
	}
	v, err := time.ParseDuration(s)
	if err != nil {
		return err
	}
	*d = Duration(v)
	return nil
}

// ClockTime is a time of day in minutes after midnight, encoded as "HH:MM".
type ClockTime int

// ParseClock parses "HH:MM" (24-hour).
func ParseClock(s string) (ClockTime, error) {
	t, err := time.Parse("15:04", s)
	if err != nil {
		return 0, fmt.Errorf("time of day %q: want HH:MM", s)
	}
	return ClockTime(t.Hour()*60 + t.Minute()), nil
}

// MustClock is ParseClock that panics; for constants.
func MustClock(s string) ClockTime {
	c, err := ParseClock(s)
	if err != nil {
		panic(err)
	}
	return c
}

// Valid reports whether c is within a day.
func (c ClockTime) Valid() bool { return c >= 0 && c < 24*60 }

func (c ClockTime) String() string { return fmt.Sprintf("%02d:%02d", int(c)/60, int(c)%60) }

func (c ClockTime) MarshalJSON() ([]byte, error) { return json.Marshal(c.String()) }

func (c *ClockTime) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	v, err := ParseClock(s)
	if err != nil {
		return err
	}
	*c = v
	return nil
}
