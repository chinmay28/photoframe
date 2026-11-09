// Package api serves the photoframe's JSON API and the embedded PWA.
//
// The API is meant for the local network and has no login. State-changing
// requests must be same-origin (checked via Origin / Sec-Fetch-Site) so a
// web page elsewhere cannot drive the frame through a visitor's browser.
package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/chinmay28/photoframe/internal/config"
	"github.com/chinmay28/photoframe/internal/library"
	"github.com/chinmay28/photoframe/internal/player"
	"github.com/chinmay28/photoframe/internal/sysinfo"
)

// Store is the settings store.
type Store interface {
	Get() config.Settings
	Update(func(*config.Settings) error) (config.Settings, error)
}

// Library is the photo catalogue.
type Library interface {
	Photos() []library.Photo
	Photo(id string) (library.Photo, bool)
	Status() map[string]library.Status
	SyncSource(ctx context.Context, id string) error
}

// Player is the slideshow.
type Player interface {
	Do(player.Command) bool
	Status() player.Status
	Preview() []byte
	UpNext(n int) []library.Photo // n < 0: everything coming up
	Show(photoID string) error
	Move(photoID string, pos player.Position) error
}

// Thumbnailer makes thumbnails.
type Thumbnailer interface {
	// Thumb returns a JPEG; it gives up with ctx's error when ctx ends.
	Thumb(context.Context, library.Photo) ([]byte, error)
	Remove(id string)
}

// Server wires the API to its collaborators. All fields except Images,
// Static, Restart and Logger are required.
type Server struct {
	Store       Store
	Library     Library
	Player      Player
	Adjustments Adjustments
	Thumbs      Thumbnailer
	Images      Thumbnailer // larger renditions for the editor; nil uses Thumbs
	UploadDir   func(sourceID string) string
	Device      func() sysinfo.Info
	Restart     func() // restarts the service; nil disables the endpoint
	Static      fs.FS  // the PWA; nil serves only the API
	Logger      *log.Logger
}

// Handler returns the HTTP handler.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/status", s.status)
	mux.HandleFunc("POST /api/control/{cmd}", s.control)
	mux.HandleFunc("GET /api/preview.png", s.preview)
	mux.HandleFunc("GET /api/settings", s.getSettings)
	mux.HandleFunc("PUT /api/settings", s.putSettings)
	mux.HandleFunc("GET /api/sources", s.listSources)
	mux.HandleFunc("POST /api/sources", s.createSource)
	mux.HandleFunc("PUT /api/sources/{id}", s.updateSource)
	mux.HandleFunc("DELETE /api/sources/{id}", s.deleteSource)
	mux.HandleFunc("POST /api/sources/{id}/sync", s.syncSource)
	mux.HandleFunc("POST /api/sources/{id}/photos", s.upload)
	mux.HandleFunc("GET /api/photos", s.listPhotos)
	mux.HandleFunc("GET /api/photos/{id}/thumb", s.thumb)
	mux.HandleFunc("GET /api/photos/{id}/image", s.image)
	mux.HandleFunc("POST /api/photos/{id}/show", s.showPhoto)
	mux.HandleFunc("POST /api/photos/{id}/queue", s.movePhoto)
	mux.HandleFunc("GET /api/queue", s.queue)
	mux.HandleFunc("GET /api/photos/{id}/adjustment", s.getAdjustment)
	mux.HandleFunc("PUT /api/photos/{id}/adjustment", s.putAdjustment)
	mux.HandleFunc("DELETE /api/photos/{id}/adjustment", s.deleteAdjustment)
	mux.HandleFunc("DELETE /api/photos/{id}", s.deletePhoto)
	mux.HandleFunc("GET /api/device", s.device)
	mux.HandleFunc("POST /api/device/restart", s.restart)
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusNotFound, "no such endpoint")
	})
	if s.Static != nil {
		mux.Handle("/", staticHandler(s.Static))
	}
	return sameOrigin(mux)
}

// StatusResponse is GET /api/status.
type StatusResponse struct {
	Name   string          `json:"name"`
	Player player.Status   `json:"player"`
	UpNext []library.Photo `json:"upNext"`
}

func (s *Server) status(w http.ResponseWriter, r *http.Request) {
	n := 4
	if v, err := strconv.Atoi(r.URL.Query().Get("upNext")); err == nil && v >= 0 && v <= 50 {
		n = v
	}
	up := s.Player.UpNext(n)
	if up == nil {
		up = []library.Photo{}
	}
	getJSON(w, r, StatusResponse{Name: s.Store.Get().Name, Player: s.Player.Status(), UpNext: up})
}

func (s *Server) control(w http.ResponseWriter, r *http.Request) {
	cmd := player.Command(r.PathValue("cmd"))
	if !cmd.Valid() {
		writeError(w, http.StatusNotFound, fmt.Sprintf("unknown command %q", cmd))
		return
	}
	if !s.Player.Do(cmd) {
		writeError(w, http.StatusServiceUnavailable, "the frame is busy; try again shortly")
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

func (s *Server) preview(w http.ResponseWriter, r *http.Request) {
	b := s.Player.Preview()
	if b == nil {
		writeError(w, http.StatusNotFound, "nothing shown yet")
		return
	}
	writeBody(w, r, "image/png", revalidate, b)
}

func (s *Server) getSettings(w http.ResponseWriter, r *http.Request) {
	getJSON(w, r, s.Store.Get().Redacted())
}

// putSettings replaces every setting except sources, which have their own
// endpoints.
func (s *Server) putSettings(w http.ResponseWriter, r *http.Request) {
	var in config.Settings
	if !readJSON(w, r, &in) {
		return
	}
	out, err := s.Store.Update(func(cur *config.Settings) error {
		in.Sources = cur.Sources
		*cur = in
		return nil
	})
	if s.writeUpdateError(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, out.Redacted())
}

// SourceView is a source plus its library status.
type SourceView struct {
	config.Source
	Status *library.Status `json:"status,omitempty"`
}

func (s *Server) listSources(w http.ResponseWriter, r *http.Request) {
	st := s.Library.Status()
	out := []SourceView{}
	for _, src := range s.Store.Get().Redacted().Sources {
		v := SourceView{Source: src}
		if ls, ok := st[src.ID]; ok {
			v.Status = &ls
		}
		out = append(out, v)
	}
	getJSON(w, r, out)
}

func (s *Server) createSource(w http.ResponseWriter, r *http.Request) {
	var in config.Source
	if !readJSON(w, r, &in) {
		return
	}
	in.APIKeySet = false
	out, err := s.Store.Update(func(cur *config.Settings) error {
		if in.ID == "" {
			in.ID = uniqueID(*cur, in.Name)
		}
		if _, exists := cur.Source(in.ID); exists {
			return &config.ValidationError{Err: fmt.Errorf("a source with id %q already exists", in.ID)}
		}
		cur.Sources = append(cur.Sources, in)
		return nil
	})
	if s.writeUpdateError(w, err) {
		return
	}
	src, _ := out.Redacted().Source(in.ID)
	writeJSON(w, http.StatusCreated, src)
}

func (s *Server) updateSource(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var in config.Source
	if !readJSON(w, r, &in) {
		return
	}
	in.ID, in.APIKeySet = id, false
	out, err := s.Store.Update(func(cur *config.Settings) error {
		for i, old := range cur.Sources {
			if old.ID != id {
				continue
			}
			if in.Type != old.Type {
				return &config.ValidationError{Err: errors.New("a source's type cannot change; create a new source")}
			}
			if in.APIKey == "" {
				// Keep the stored key, but only for the same server: the key
				// must never be sent somewhere the user did not type it for.
				if in.URL != old.URL && old.APIKey != "" {
					return &config.ValidationError{Err: errors.New("apiKey: re-enter the key when changing the server URL")}
				}
				in.APIKey = old.APIKey
			}
			cur.Sources[i] = in
			return nil
		}
		return errNotFound
	})
	if s.writeUpdateError(w, err) {
		return
	}
	src, _ := out.Redacted().Source(id)
	writeJSON(w, http.StatusOK, src)
}

func (s *Server) deleteSource(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	_, err := s.Store.Update(func(cur *config.Settings) error {
		for i, src := range cur.Sources {
			if src.ID == id {
				cur.Sources = append(cur.Sources[:i], cur.Sources[i+1:]...)
				return nil
			}
		}
		return errNotFound
	})
	if s.writeUpdateError(w, err) {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) syncSource(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if src, ok := s.Store.Get().Source(id); !ok || !src.Enabled {
		writeError(w, http.StatusNotFound, "no enabled source "+strconv.Quote(id))
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
		defer cancel()
		if err := s.Library.SyncSource(ctx, id); err != nil {
			s.logf("sync %s: %v", id, err)
		}
	}()
	w.WriteHeader(http.StatusAccepted)
}

func (s *Server) listPhotos(w http.ResponseWriter, r *http.Request) {
	srcID := r.URL.Query().Get("source")
	out := []library.Photo{}
	for _, ph := range s.Library.Photos() {
		if srcID == "" || ph.SourceID == srcID {
			out = append(out, ph)
		}
	}
	getJSON(w, r, out)
}

func (s *Server) thumb(w http.ResponseWriter, r *http.Request) {
	s.serveThumb(w, r, s.Thumbs)
}

func (s *Server) serveThumb(w http.ResponseWriter, r *http.Request, t Thumbnailer) {
	ph, ok := s.Library.Photo(r.PathValue("id"))
	if !ok {
		writeError(w, http.StatusNotFound, "no such photo")
		return
	}
	b, err := t.Thumb(r.Context(), ph)
	if r.Context().Err() != nil {
		return // the app stopped waiting; nobody is there to answer
	}
	if err != nil {
		s.logf("thumb %s: %v", ph.ID, err)
		writeError(w, http.StatusInternalServerError, "could not make a thumbnail")
		return
	}
	writeBody(w, r, "image/jpeg", dayLong, b)
}

func (s *Server) device(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.Device())
}

func (s *Server) restart(w http.ResponseWriter, r *http.Request) {
	if s.Restart == nil {
		writeError(w, http.StatusNotImplemented, "restart is not available")
		return
	}
	w.WriteHeader(http.StatusAccepted)
	go func() {
		time.Sleep(500 * time.Millisecond) // let the response go out
		s.Restart()
	}()
}

var errNotFound = errors.New("not found")

// writeUpdateError maps a Store.Update error to a response and reports
// whether it wrote one.
func (s *Server) writeUpdateError(w http.ResponseWriter, err error) bool {
	var ve *config.ValidationError
	switch {
	case err == nil:
		return false
	case errors.Is(err, errNotFound):
		writeError(w, http.StatusNotFound, "not found")
	case errors.As(err, &ve):
		writeError(w, http.StatusUnprocessableEntity, ve.Error())
	default:
		s.logf("update settings: %v", err)
		writeError(w, http.StatusInternalServerError, "could not save settings")
	}
	return true
}

func (s *Server) logf(format string, a ...any) {
	if s.Logger != nil {
		s.Logger.Printf(format, a...)
	}
}

var nonID = regexp.MustCompile(`[^a-z0-9]+`)

// uniqueID derives a source ID from a name, e.g. "Family album" -> "family-album".
func uniqueID(s config.Settings, name string) string {
	base := strings.Trim(nonID.ReplaceAllString(strings.ToLower(name), "-"), "-")
	if len(base) > 32 {
		base = strings.Trim(base[:32], "-")
	}
	if base == "" {
		base = "source"
	}
	id := base
	for i := 2; ; i++ {
		if _, exists := s.Source(id); !exists {
			return id
		}
		id = fmt.Sprintf("%s-%d", base, i)
	}
}

func readJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	if ct := r.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		writeError(w, http.StatusUnsupportedMediaType, "send application/json")
		return false
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

type errorBody struct {
	Error string `json:"error"`
}

func writeError(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, errorBody{Error: msg})
}

// sameOrigin rejects cross-site state-changing requests.
func sameOrigin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			next.ServeHTTP(w, r)
			return
		}
		if site := r.Header.Get("Sec-Fetch-Site"); site != "" && site != "same-origin" && site != "none" {
			writeError(w, http.StatusForbidden, "cross-site request refused")
			return
		}
		if origin := r.Header.Get("Origin"); origin != "" {
			u, err := url.Parse(origin)
			if err != nil || u.Host != r.Host {
				writeError(w, http.StatusForbidden, "cross-origin request refused")
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// staticHandler serves the PWA. Unknown paths fall back to index.html so
// client-side routes work after a reload. Every file is revalidated by ETag:
// a new build of the frame's software is picked up on the next visit, and an
// unchanged one costs a 304 per file.
func staticHandler(fsys fs.FS) http.Handler {
	files := http.FileServerFS(fsys)
	tags := staticETags(fsys)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := strings.TrimPrefix(r.URL.Path, "/")
		if p != "" {
			if _, err := fs.Stat(fsys, p); err != nil {
				r = r.Clone(r.Context())
				r.URL.Path = "/"
				p = ""
			}
		}
		if p == "" {
			p = "index.html"
		}
		if tag := tags[p]; tag != "" {
			w.Header().Set("ETag", tag) // http.ServeContent answers If-None-Match from it
		}
		w.Header().Set("Cache-Control", revalidate)
		files.ServeHTTP(w, r)
	})
}
