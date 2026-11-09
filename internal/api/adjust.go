package api

import (
	"net/http"

	"github.com/chinmay28/photoframe/internal/adjust"
	"github.com/chinmay28/photoframe/internal/library"
	"github.com/chinmay28/photoframe/internal/player"
)

// Adjustments remembers per-photo adjustments.
type Adjustments interface {
	Get(photoID string) adjust.Adjustment
	Set(photoID string, a adjust.Adjustment) (adjust.Adjustment, error)
}

// AdjustmentView is a photo's adjustment as the app sees it.
type AdjustmentView struct {
	Photo      library.Photo     `json:"photo"`
	Adjustment adjust.Adjustment `json:"adjustment"`
	// OnScreen reports that the photo is on the frame now. After a change
	// it means the frame is re-rendering it.
	OnScreen bool `json:"onScreen"`
}

func (s *Server) getAdjustment(w http.ResponseWriter, r *http.Request) {
	ph, ok := s.Library.Photo(r.PathValue("id"))
	if !ok {
		writeError(w, http.StatusNotFound, "no such photo")
		return
	}
	writeJSON(w, http.StatusOK, AdjustmentView{Photo: ph, Adjustment: s.Adjustments.Get(ph.ID), OnScreen: s.onScreen(ph.ID)})
}

// putAdjustment replaces a photo's adjustment; a zero adjustment resets it.
// When the photo is on the frame it is re-rendered right away.
func (s *Server) putAdjustment(w http.ResponseWriter, r *http.Request) {
	var in adjust.Adjustment
	if !readJSON(w, r, &in) {
		return
	}
	s.saveAdjustment(w, r.PathValue("id"), in)
}

func (s *Server) deleteAdjustment(w http.ResponseWriter, r *http.Request) {
	s.saveAdjustment(w, r.PathValue("id"), adjust.Adjustment{})
}

func (s *Server) saveAdjustment(w http.ResponseWriter, id string, in adjust.Adjustment) {
	ph, ok := s.Library.Photo(id)
	if !ok {
		writeError(w, http.StatusNotFound, "no such photo")
		return
	}
	out, err := s.Adjustments.Set(ph.ID, in)
	if s.writeUpdateError(w, err) {
		return
	}
	on := s.onScreen(ph.ID)
	if on {
		s.Player.Do(player.Refresh)
	}
	writeJSON(w, http.StatusOK, AdjustmentView{Photo: ph, Adjustment: out, OnScreen: on})
}

func (s *Server) onScreen(id string) bool {
	st := s.Player.Status()
	return st.Screen == player.ScreenPhoto && st.Current != nil && st.Current.ID == id
}

// image serves a larger rendition than thumb, for editing a photo's framing.
func (s *Server) image(w http.ResponseWriter, r *http.Request) {
	t := s.Images
	if t == nil {
		t = s.Thumbs
	}
	s.serveThumb(w, r, t)
}
