// Package heictest makes HEIC files for tests.
package heictest

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"testing"

	"github.com/gen2brain/h265/heic"
)

// Gradient is a w×h image whose every pixel differs from its neighbours, so
// a decode that loses or swaps its content shows.
func Gradient(w, h int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.SetRGBA(x, y, color.RGBA{uint8(x * 255 / max(1, w-1)), uint8(y * 255 / max(1, h-1)), 128, 255})
		}
	}
	return img
}

// Encode writes img as a HEIC still with the given EXIF (a TIFF stream, as
// from DateTime; nil for none).
func Encode(t testing.TB, img image.Image, exif []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := heic.Encode(&buf, img, heic.EncodeOptions{Quality: 90, Exif: exif}); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// DateTime is a minimal little-endian EXIF TIFF stream whose IFD0 holds only
// a DateTime tag set to dt, as "2006:01:02 15:04:05".
func DateTime(dt string) []byte {
	b := []byte("II*\x00\x08\x00\x00\x00")
	b = binary.LittleEndian.AppendUint16(b, 1) // one entry
	b = binary.LittleEndian.AppendUint16(b, 0x0132)
	b = binary.LittleEndian.AppendUint16(b, 2) // ASCII
	b = binary.LittleEndian.AppendUint32(b, uint32(len(dt)+1))
	b = binary.LittleEndian.AppendUint32(b, 26) // value offset
	b = binary.LittleEndian.AppendUint32(b, 0)  // no next IFD
	return append(append(b, dt...), 0)
}
