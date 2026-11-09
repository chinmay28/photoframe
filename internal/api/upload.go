package api

import (
	"errors"
	"fmt"
	"image"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/chinmay28/photoframe/internal/config"
	"github.com/chinmay28/photoframe/internal/source"
)

const (
	maxUploadRequest = 512 << 20
	maxUploadFile    = 40 << 20
)

// UploadResult is the response to an upload.
type UploadResult struct {
	Saved    []string `json:"saved"`
	Rejected []string `json:"rejected,omitempty"`
}

// upload stores multipart "photos" files in an upload source's folder.
func (s *Server) upload(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	src, ok := s.Store.Get().Source(id)
	if !ok {
		writeError(w, http.StatusNotFound, "no such source")
		return
	}
	if src.Type != config.SourceUpload {
		writeError(w, http.StatusBadRequest, "photos can only be uploaded to an upload source")
		return
	}
	dir := s.UploadDir(id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		s.logf("upload: %v", err)
		writeError(w, http.StatusInternalServerError, "cannot store photos")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxUploadRequest)
	mr, err := r.MultipartReader()
	if err != nil {
		writeError(w, http.StatusBadRequest, "send multipart/form-data with 'photos' files")
		return
	}
	res := UploadResult{Saved: []string{}}
	for {
		part, err := mr.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			writeError(w, http.StatusBadRequest, "upload interrupted: "+err.Error())
			return
		}
		if part.FormName() != "photos" || part.FileName() == "" {
			part.Close()
			continue
		}
		name, err := saveUpload(dir, part.FileName(), part)
		part.Close()
		if err != nil {
			res.Rejected = append(res.Rejected, fmt.Sprintf("%s: %v", part.FileName(), err))
			continue
		}
		res.Saved = append(res.Saved, name)
	}
	if len(res.Saved) > 0 && src.Enabled {
		if err := s.Library.SyncSource(r.Context(), id); err != nil {
			s.logf("upload: rescan %s: %v", id, err)
		}
	}
	code := http.StatusCreated
	if len(res.Saved) == 0 {
		code = http.StatusUnprocessableEntity
	}
	writeJSON(w, code, res)
}

var unsafeName = regexp.MustCompile(`[^A-Za-z0-9._ -]+`)

// saveUpload writes one file into dir under a sanitised, unique name after
// checking that it decodes as an image.
func saveUpload(dir, original string, r io.Reader) (string, error) {
	base := filepath.Base(strings.ReplaceAll(original, "\\", "/"))
	if !source.IsImageName(base) {
		return "", errors.New("not a JPEG, PNG, GIF or WebP file")
	}
	ext := strings.ToLower(filepath.Ext(base))
	stem := strings.Trim(unsafeName.ReplaceAllString(strings.TrimSuffix(base, filepath.Ext(base)), "_"), " ._-")
	if stem == "" {
		stem = "photo"
	}
	if len(stem) > 80 {
		stem = stem[:80]
	}

	tmp, err := os.CreateTemp(dir, ".upload-*")
	if err != nil {
		return "", err
	}
	defer os.Remove(tmp.Name())
	n, err := io.Copy(tmp, io.LimitReader(r, maxUploadFile+1))
	if err == nil && n > maxUploadFile {
		err = fmt.Errorf("larger than %d MB", maxUploadFile>>20)
	}
	if err == nil {
		_, err = tmp.Seek(0, io.SeekStart)
	}
	if err == nil {
		if _, _, derr := image.DecodeConfig(tmp); derr != nil {
			err = errors.New("not a readable image")
		}
	}
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return "", err
	}

	name := stem + ext
	for i := 2; ; i++ {
		target := filepath.Join(dir, name)
		if _, err := os.Lstat(target); errors.Is(err, os.ErrNotExist) {
			if err := os.Link(tmp.Name(), target); err != nil {
				// Filesystems without hard links: fall back to rename.
				if err := os.Rename(tmp.Name(), target); err != nil {
					return "", err
				}
			}
			os.Chmod(target, 0o644)
			return name, nil
		}
		name = fmt.Sprintf("%s-%d%s", stem, i, ext)
	}
}

// deletePhoto removes a photo from an upload source.
func (s *Server) deletePhoto(w http.ResponseWriter, r *http.Request) {
	ph, ok := s.Library.Photo(r.PathValue("id"))
	if !ok {
		writeError(w, http.StatusNotFound, "no such photo")
		return
	}
	src, ok := s.Store.Get().Source(ph.SourceID)
	if !ok || src.Type != config.SourceUpload {
		writeError(w, http.StatusBadRequest, "only uploaded photos can be deleted here")
		return
	}
	// Only ever delete files inside the source's own upload folder.
	dir, err := filepath.Abs(s.UploadDir(src.ID))
	if err == nil {
		var p string
		p, err = filepath.Abs(ph.Path)
		if err == nil && !strings.HasPrefix(p, dir+string(filepath.Separator)) {
			err = errors.New("photo is outside the upload folder")
		}
		if err == nil {
			err = os.Remove(p)
		}
	}
	if err != nil {
		s.logf("delete %s: %v", ph.ID, err)
		writeError(w, http.StatusInternalServerError, "could not delete photo")
		return
	}
	s.Thumbs.Remove(ph.ID)
	if err := s.Library.SyncSource(r.Context(), src.ID); err != nil {
		s.logf("delete: rescan %s: %v", src.ID, err)
	}
	w.WriteHeader(http.StatusNoContent)
}
