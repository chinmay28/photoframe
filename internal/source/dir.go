// Package source implements photo sources: local directories (uploads and
// mounted network shares), Immich albums, and single image URLs.
package source

import (
	"bytes"
	"context"
	"errors"
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
// Each Sync also takes the photos it lists as its own: it gives them to the
// user and group the process runs as, and sets them to PhotoMode. So a photo
// added to the folder by any means, by any user, is the frame's, and readable
// by everyone, from the next sync on. Folders are left as they are, so whoever
// made one can keep adding to it. A photo it may neither take over nor read
// is left out of the list, and Sync says so in its error.
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
	var unreadable int
	var firstUnreadable error
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
		if e.IsDir() || !IsImageName(e.Name()) {
			return nil
		}
		info, err := e.Info()
		if err != nil {
			return nil
		}
		info, perr := protect(path, info)
		rel, _ := filepath.Rel(d.Root, path)
		if perr != nil && !readable(path) {
			// Showing it would only fail later, with nothing to say why.
			unreadable++
			if firstUnreadable == nil {
				firstUnreadable = fmt.Errorf("%s: %w", filepath.ToSlash(rel), underlying(perr))
			}
			return nil
		}
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
	if err != nil {
		return photos, err
	}
	if photos == nil {
		photos = []library.Photo{} // scanned, and found none to show
	}
	if unreadable > 0 {
		what := "1 photo"
		if unreadable > 1 {
			what = fmt.Sprintf("%d photos", unreadable)
		}
		return photos, fmt.Errorf("%s left out: not readable by the frame, which could not take them over (%w)",
			what, firstUnreadable)
	}
	return photos, nil
}

// readable reports whether the process can open the file at path.
func readable(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	f.Close()
	return true
}

// underlying strips the operation and path from a file error, leaving the
// reason, e.g. "operation not permitted".
func underlying(err error) error {
	var pe *fs.PathError
	if errors.As(err, &pe) {
		return pe.Err
	}
	return err
}

// protect makes the regular file at path the process's own and sets it to
// PhotoMode, changing only what differs. It returns the file's info
// afterwards and the first change that wasn't permitted, if any: taking a
// file from another user needs root or CAP_CHOWN, and a share may be
// read-only. Such a photo is left as it is; whether it can still be shown is
// for the caller to find out. Symlinks are left alone, since changing one
// would change a file outside the folder.
func protect(path string, info fs.FileInfo) (fs.FileInfo, error) {
	if !info.Mode().IsRegular() {
		return info, nil
	}
	changed, err := takeOwnership(path, info)
	if info.Mode().Perm() != PhotoMode {
		if cerr := os.Chmod(path, PhotoMode); cerr == nil {
			changed = true
		} else if err == nil {
			err = cerr
		}
	}
	if changed {
		if after, serr := os.Lstat(path); serr == nil {
			info = after
		}
	}
	return info, err
}

// takeOwnership gives the file at path to the user and group the process
// runs as, and reports whether it changed anything. It never follows a
// symlink.
func takeOwnership(path string, info fs.FileInfo) (bool, error) {
	uid, gid, ok := owner(info)
	if !ok || (uid == os.Geteuid() && gid == os.Getegid()) {
		return false, nil
	}
	if err := os.Lchown(path, os.Geteuid(), os.Getegid()); err != nil {
		return false, err
	}
	return true, nil
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
