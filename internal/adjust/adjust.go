// Package adjust defines per-photo adjustments (rotation, framing, zoom,
// position and brightness) and a file-backed store that remembers them by
// photo ID.
package adjust

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"math"
	"os"
	"sync"

	"github.com/chinmay28/photoframe/internal/config"
)

// Zoom limits. 1 shows the whole framing window; MaxZoom crops into it.
const (
	MinZoom = 1.0
	MaxZoom = 4.0
)

// Point is a pair of fractions within [0, 1].
type Point struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

// Adjustment is how one photo should be shown. The zero value changes
// nothing, so a photo without an adjustment renders exactly as before.
type Adjustment struct {
	// Rotate turns the photo clockwise by 0, 90, 180 or 270 degrees, after
	// its EXIF orientation is applied.
	Rotate int `json:"rotate,omitempty"`
	// Fit overrides the slideshow's framing for this photo when set.
	Fit config.Fit `json:"fit,omitempty"`
	// Zoom magnifies the framing window; 0 means 1 (no zoom).
	Zoom float64 `json:"zoom,omitempty"`
	// Position aligns the photo in the frame like CSS object-position: on
	// each axis 0 lines up the left (top) edges of photo and frame, 1 the
	// right (bottom) edges and 0.5 centres. It applies to the rotated photo
	// whether it is cropped or bordered. Nil lets the framing mode decide:
	// smart crop, or the centre.
	Position *Point `json:"position,omitempty"`
	// Brightness is in [-1, 1]; 0 leaves the photo unchanged.
	Brightness float64 `json:"brightness,omitempty"`
}

// IsZero reports whether a changes nothing.
func (a Adjustment) IsZero() bool {
	return a.Rotate == 0 && a.Fit == "" && (a.Zoom == 0 || a.Zoom == 1) && a.Position == nil && a.Brightness == 0
}

// ZoomOrOne returns the zoom factor, treating 0 as 1.
func (a Adjustment) ZoomOrOne() float64 {
	if a.Zoom == 0 {
		return 1
	}
	return a.Zoom
}

// Normalize returns a with equivalent values in canonical form: a zoom of 1
// becomes 0 so IsZero holds for adjustments that change nothing.
func (a Adjustment) Normalize() Adjustment {
	if a.Zoom == 1 {
		a.Zoom = 0
	}
	if a.Position != nil {
		f := *a.Position
		a.Position = &f
	}
	return a
}

// Validate checks every field and returns all problems joined together.
func (a Adjustment) Validate() error {
	var errs []error
	switch a.Rotate {
	case 0, 90, 180, 270:
	default:
		errs = append(errs, fmt.Errorf("rotate: must be 0, 90, 180 or 270"))
	}
	switch a.Fit {
	case "", config.FitSmart, config.FitFill, config.FitFit:
	default:
		errs = append(errs, fmt.Errorf("fit: unknown value %q", a.Fit))
	}
	if a.Zoom != 0 && !(a.Zoom >= MinZoom && a.Zoom <= MaxZoom) {
		errs = append(errs, fmt.Errorf("zoom: must be within [%g, %g]", MinZoom, MaxZoom))
	}
	if f := a.Position; f != nil && !(inUnit(f.X) && inUnit(f.Y)) {
		errs = append(errs, errors.New("position: x and y must be within [0, 1]"))
	}
	if !(a.Brightness >= -1 && a.Brightness <= 1) {
		errs = append(errs, errors.New("brightness: must be within [-1, 1]"))
	}
	return errors.Join(errs...)
}

func inUnit(v float64) bool { return v >= 0 && v <= 1 && !math.IsNaN(v) }

// Store remembers adjustments by photo ID in a JSON file. It is safe for
// concurrent use. Photos without an adjustment are not stored.
type Store struct {
	path string

	mu  sync.RWMutex
	all map[string]Adjustment
}

// file is the on-disk format.
type file struct {
	Photos map[string]Adjustment `json:"photos"`
}

// Open loads adjustments from path; a missing file is an empty store.
// Entries that fail validation are dropped rather than failing startup.
func Open(path string) (*Store, error) {
	st := &Store{path: path, all: map[string]Adjustment{}}
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return st, nil
	}
	if err != nil {
		return nil, err
	}
	var f file
	if err := json.Unmarshal(b, &f); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	for id, a := range f.Photos {
		a = a.Normalize()
		if a.Validate() == nil && !a.IsZero() {
			st.all[id] = a
		}
	}
	return st, nil
}

// Get returns the adjustment for a photo, or the zero Adjustment.
func (st *Store) Get(id string) Adjustment {
	st.mu.RLock()
	defer st.mu.RUnlock()
	return st.all[id].Normalize() // copies Position
}

// Set validates and saves a photo's adjustment. A zero adjustment removes
// the entry. Invalid input returns a *config.ValidationError and changes
// nothing.
func (st *Store) Set(id string, a Adjustment) (Adjustment, error) {
	if id == "" {
		return Adjustment{}, &config.ValidationError{Err: errors.New("photo id: required")}
	}
	a = a.Normalize()
	if err := a.Validate(); err != nil {
		return Adjustment{}, &config.ValidationError{Err: err}
	}
	st.mu.Lock()
	defer st.mu.Unlock()
	prev, had := st.all[id]
	if a.IsZero() {
		delete(st.all, id)
	} else {
		st.all[id] = a
	}
	if err := st.save(); err != nil {
		if had {
			st.all[id] = prev
		} else {
			delete(st.all, id)
		}
		return Adjustment{}, err
	}
	return a.Normalize(), nil
}

// Delete forgets a photo's adjustment.
func (st *Store) Delete(id string) error {
	_, err := st.Set(id, Adjustment{})
	return err
}

// save writes the store; the caller holds mu.
func (st *Store) save() error {
	b, err := json.MarshalIndent(file{Photos: st.all}, "", "  ")
	if err != nil {
		return err
	}
	return config.WriteFileAtomic(st.path, append(b, '\n'), 0o600)
}
