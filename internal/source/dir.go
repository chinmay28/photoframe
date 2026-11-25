// Package source implements photo sources: local directories (uploads and
// mounted network shares), Immich albums, and single image URLs.
package source

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/chinmay28/photoframe/internal/imagefmt"
	"github.com/chinmay28/photoframe/internal/library"
	"github.com/rwcarlsen/goexif/exif"
)

// IsImageName reports whether name has a supported image extension.
func IsImageName(name string) bool { return imagefmt.IsImageName(name) }

// PhotoMode is the permission a Dir gives every photo it lists: the owner may
// write, everyone may read.
const PhotoMode fs.FileMode = 0o644

// Dir lists images under Root recursively. Hidden files and directories are
// skipped. Photos in sub-folders are titled with the folder name.
//
// Each Sync also takes the photos it lists as its own: it gives them, and the
// sub-folders they are in, to the user and group the process runs as, and sets
// the photos to PhotoMode. So a photo added to the folder by any means, by any
// user, is the frame's, and readable by everyone, from the next sync on. Root itself is
// left as it is. See protect for what happens when that isn't permitted.
type Dir struct {
	SourceID string
	Root     string

	meta map[string]fileMeta // cache of EXIF dates, keyed by path
}

type fileMeta struct {
	size    int64
	mod     time.Time
	takenAt time.Time
}

// Sync implements library.Source.
func (d *Dir) Sync(ctx context.Context) ([]library.Photo, error) {
	if st, err := os.Stat(d.Root); err != nil {
		return nil, fmt.Errorf("folder %s: %w", d.Root, err)
	} else if !st.IsDir() {
		return nil, fmt.Errorf("folder %s: not a directory", d.Root)
	}
	if d.meta == nil {
		d.meta = map[string]fileMeta{}
	}
	seen := map[string]bool{}
	var photos []library.Photo
	err := filepath.WalkDir(d.Root, func(path string, e fs.DirEntry, err error) error {
		if err != nil {
			return nil // skip unreadable entries rather than failing the whole scan
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if strings.HasPrefix(e.Name(), ".") && path != d.Root {
			if e.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if e.IsDir() {
			if info, err := e.Info(); err == nil && path != d.Root {
				takeOwnership(path, info)
			}
			return nil
		}
		if !IsImageName(e.Name()) {
			return nil
		}
		info, err := e.Info()
		if err != nil {
			return nil
		}
		info = protect(path, info)
		rel, _ := filepath.Rel(d.Root, path)
		seen[path] = true
		photos = append(photos, library.Photo{
			ID:       library.PhotoID(d.SourceID, filepath.ToSlash(rel)),
			SourceID: d.SourceID,
			Path:     path,
			TakenAt:  d.takenAt(path, info),
			Title:    folderTitle(rel),
			Name:     e.Name(),
		})
		return nil
	})
	for p := range d.meta {
		if !seen[p] {
			delete(d.meta, p)
		}
	}
	return photos, err
}

// protect makes the regular file at path the process's own and sets it to
// PhotoMode, changing only what differs, and returns its file info
// afterwards. It is best effort: where a change isn't permitted (taking a
// file from another user needs root or CAP_CHOWN; a share may be read-only)
// the photo is left as it is, and it is still shown. Symlinks are left alone,
// since changing one would change a file outside the folder.
func protect(path string, info fs.FileInfo) fs.FileInfo {
	if !info.Mode().IsRegular() {
		return info
	}
	changed := takeOwnership(path, info)
	if info.Mode().Perm() != PhotoMode && os.Chmod(path, PhotoMode) == nil {
		changed = true
	}
	if changed {
		if after, err := os.Lstat(path); err == nil {
			return after
		}
	}
	return info
}

// takeOwnership gives the file or directory at path to the user and group the
// process runs as, and reports whether it changed anything. It never follows
// a symlink.
func takeOwnership(path string, info fs.FileInfo) bool {
	uid, gid, ok := owner(info)
	if !ok || (uid == os.Geteuid() && gid == os.Getegid()) {
		return false
	}
	return os.Lchown(path, os.Geteuid(), os.Getegid()) == nil
}

func (d *Dir) takenAt(path string, info fs.FileInfo) time.Time {
	if m, ok := d.meta[path]; ok && m.size == info.Size() && m.mod.Equal(info.ModTime()) {
		return m.takenAt
	}
	t := ExifDate(path)
	if t.IsZero() {
		t = info.ModTime()
	}
	d.meta[path] = fileMeta{size: info.Size(), mod: info.ModTime(), takenAt: t}
	return t
}

// ExifDate returns the EXIF capture time of the image at path, or the zero
// time if it has none.
func ExifDate(path string) time.Time {
	f, err := os.Open(path)
	if err != nil {
		return time.Time{}
	}
	defer f.Close()
	x, err := exif.Decode(exifStream(f))
	if err != nil {
		return time.Time{}
	}
	t, err := x.DateTime()
	if err != nil {
		return time.Time{}
	}
	return t
}

// exifStream returns what to hand the EXIF reader for the file f: f itself
// for JPEG and the like, or the EXIF block of a HEIF file, which the reader
// cannot find by itself.
func exifStream(f io.ReadSeeker) io.Reader {
	head := make([]byte, 12)
	n, _ := io.ReadFull(f, head)
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return bytes.NewReader(nil)
	}
	if !imagefmt.IsHEIF(head[:n]) {
		return f
	}
	tiff, err := imagefmt.HEIFExif(f)
	if err != nil {
		return bytes.NewReader(nil)
	}
	return bytes.NewReader(tiff)
}

func folderTitle(rel string) string {
	dir := filepath.Dir(rel)
	if dir == "." {
		return ""
	}
	return filepath.Base(dir)
}
