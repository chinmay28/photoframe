package api

import (
	"errors"
	"net/http"
	"sort"

	"github.com/chinmay28/photoframe/internal/library"
	"github.com/chinmay28/photoframe/internal/player"
)

// QueueResponse is GET /api/queue: every photo in the slideshow.
type QueueResponse struct {
	Current *library.Photo `json:"current,omitempty"`
	// UpNext is what the frame will show, in order.
	UpNext []library.Photo `json:"upNext"`
	// Rest are the other photos, shown recently in this round, newest first.
	Rest []library.Photo `json:"rest"`
}

func (s *Server) queue(w http.ResponseWriter, r *http.Request) {
	st := s.Player.Status()
	up := s.Player.UpNext(-1)
	listed := map[string]bool{}
	if st.Current != nil {
		listed[st.Current.ID] = true
	}
	for _, ph := range up {
		listed[ph.ID] = true
	}
	rest := []library.Photo{}
	for _, ph := range s.Library.Photos() {
		if !listed[ph.ID] {
			rest = append(rest, ph)
		}
	}
	sort.SliceStable(rest, func(i, j int) bool { return rest[i].TakenAt.After(rest[j].TakenAt) })
	if up == nil {
		up = []library.Photo{}
	}
	writeJSON(w, http.StatusOK, QueueResponse{Current: st.Current, UpNext: up, Rest: rest})
}

// showPhoto puts a photo on the frame now.
func (s *Server) showPhoto(w http.ResponseWriter, r *http.Request) {
	if s.writePlayerError(w, s.Player.Show(r.PathValue("id"))) {
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

// MoveRequest is the body of POST /api/photos/{id}/queue.
type MoveRequest struct {
	Position player.Position `json:"position"` // "front" or "back"
}

func (s *Server) movePhoto(w http.ResponseWriter, r *http.Request) {
	var in MoveRequest
	if !readJSON(w, r, &in) {
		return
	}
	if in.Position != player.Front && in.Position != player.Back {
		writeError(w, http.StatusUnprocessableEntity, `position: must be "front" or "back"`)
		return
	}
	if s.writePlayerError(w, s.Player.Move(r.PathValue("id"), in.Position)) {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// writePlayerError maps a Show/Move error to a response and reports whether
// it wrote one.
func (s *Server) writePlayerError(w http.ResponseWriter, err error) bool {
	switch {
	case err == nil:
		return false
	case errors.Is(err, player.ErrNoPhoto):
		writeError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, player.ErrOnScreen):
		writeError(w, http.StatusConflict, err.Error())
	default:
		writeError(w, http.StatusBadRequest, err.Error())
	}
	return true
}
