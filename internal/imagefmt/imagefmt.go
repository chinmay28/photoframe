// Package imagefmt is the one place that says which image files photoframe
// reads and how it decodes them. Importing it registers every decoder with
// the image package (JPEG, PNG, GIF, WebP and HEIC/HEIF), IsImageName
// answers for the same set by file name, and Decode decodes any of them.
//
// HEIC is decoded by github.com/gen2brain/h265, a pure-Go HEVC decoder (NEON
// on arm64), so builds stay CGO-free and nothing needs installing on the Pi.
package imagefmt

import (
	"bytes"
	"image"
	_ "image/gif" // register decoders
	_ "image/jpeg"
	_ "image/png"
	"io"
	"path/filepath"
	"strings"

	"github.com/gen2brain/h265/heic"
	_ "golang.org/x/image/webp"
)

// HEIF is the format name Decode and image.Decode report for HEIC and HEIF.
const HEIF = "heic"

// exts are the file name extensions of the formats registered here.
var exts = map[string]bool{
	".jpg": true, ".jpeg": true, ".png": true, ".gif": true, ".webp": true,
	".heic": true, ".heif": true,
}

// IsImageName reports whether name has the extension of a format this
// package decodes. Case does not matter.
func IsImageName(name string) bool { return exts[strings.ToLower(filepath.Ext(name))] }

// heifBrands are the ftyp major brands the heic package registers: HEVC
// stills and sequences, and the generic image brands.
var heifBrands = []string{"heic", "heix", "heim", "heis", "hevc", "hevx", "hevm", "hevs", "mif1", "msf1"}

// IsHEIF reports whether head, the start of a file, is a HEIF file: an ftyp
// box with a HEIF major brand. Twelve bytes are enough to tell.
func IsHEIF(head []byte) bool {
	if len(head) < 12 || string(head[4:8]) != "ftyp" {
		return false
	}
	for _, br := range heifBrands {
		if string(head[8:12]) == br {
			return true
		}
	}
	return false
}

// Decode decodes an image in any registered format and names the format as
// image.Decode does.
//
// A HEIF image comes back upright: its clap, irot and imir boxes, which is
// where phones record cropping and how the camera was held, are applied
// (image.Decode leaves them out). Other formats come back as stored, and
// applying their EXIF orientation is up to the caller.
func Decode(b []byte) (image.Image, string, error) {
	if IsHEIF(b) {
		img, err := heic.Decode(bytes.NewReader(b), heic.Options{AutoRotate: true})
		return img, HEIF, err
	}
	return image.Decode(bytes.NewReader(b))
}

// HEIFExif returns the EXIF data of the HEIF file r: the TIFF stream that
// github.com/rwcarlsen/goexif decodes. It fails when r is not HEIF or
// carries no EXIF.
func HEIFExif(r io.Reader) ([]byte, error) { return heic.RawExif(r) }
