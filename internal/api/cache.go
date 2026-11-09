package api

import (
	"bytes"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"io/fs"
	"net/http"
	"strconv"
	"strings"
)

// The app polls the frame and redraws its screens often, over Wi-Fi, from a
// board with little to spare. Nearly all of those answers are the same as
// last time, so every GET carries an ETag: the browser sends it back and an
// unchanged answer costs a few header bytes (304) instead of the body.

// Cache-Control values. Both keep the browser's copy; they differ in whether
// it asks before using it.
const (
	// revalidate: always ask, reuse the copy on 304. For anything that can
	// change at any moment.
	revalidate = "no-cache"
	// dayLong: use the copy for a day without asking. For renditions of a
	// photo, which change only when its file is replaced.
	dayLong = "private, max-age=86400"
)

// etagOf returns a strong ETag for a response body.
func etagOf(b []byte) string {
	sum := sha1.Sum(b)
	return `"` + hex.EncodeToString(sum[:10]) + `"`
}

// etagMatches reports whether the request's If-None-Match names tag.
func etagMatches(r *http.Request, tag string) bool {
	for _, v := range r.Header.Values("If-None-Match") {
		for _, t := range strings.Split(v, ",") {
			t = strings.TrimPrefix(strings.TrimSpace(t), "W/")
			if t == tag || t == "*" {
				return true
			}
		}
	}
	return false
}

// writeBody answers a GET with body, or with 304 when the client already
// has exactly that body.
func writeBody(w http.ResponseWriter, r *http.Request, contentType, cacheControl string, body []byte) {
	tag := etagOf(body)
	h := w.Header()
	h.Set("ETag", tag)
	h.Set("Cache-Control", cacheControl)
	if etagMatches(r, tag) {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	h.Set("Content-Type", contentType)
	h.Set("Content-Length", strconv.Itoa(len(body)))
	if r.Method != http.MethodHead {
		w.Write(body)
	}
}

// getJSON answers a GET with v as JSON, revalidated on every use.
func getJSON(w http.ResponseWriter, r *http.Request, v any) {
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(v); err != nil {
		writeError(w, http.StatusInternalServerError, "could not encode the response")
		return
	}
	writeBody(w, r, "application/json", revalidate, buf.Bytes())
}

// staticETags hashes every file of the embedded app once. Embedded files
// carry no modification time, so without these the browser has nothing to
// revalidate with and downloads the whole app again on each visit.
func staticETags(fsys fs.FS) map[string]string {
	tags := map[string]string{}
	fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if b, err := fs.ReadFile(fsys, p); err == nil {
			tags[p] = etagOf(b)
		}
		return nil
	})
	return tags
}
