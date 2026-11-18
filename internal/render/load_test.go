package render

import (
	"bytes"
	"encoding/binary"
	"image"
	"os"
	"testing"

	"github.com/chinmay28/photoframe/internal/imagefmt"
	"github.com/chinmay28/photoframe/internal/imagefmt/heictest"
)

func TestDecodeHEICDownscales(t *testing.T) {
	b := heictest.Encode(t, heictest.Gradient(512, 256), nil)
	img, err := Decode(b, 128)
	if err != nil {
		t.Fatal(err)
	}
	if img.Bounds() != image.Rect(0, 0, 128, 64) {
		t.Fatalf("size %v", img.Bounds())
	}
	if l, r := img.RGBAAt(0, 32), img.RGBAAt(127, 32); r.R < l.R+200 {
		t.Fatalf("gradient lost: left %v, right %v", l, r)
	}
}

// The fixture is a portrait photo stored on its side, with both an irot box
// and EXIF orientation 6 saying so, as phones write them. Turned once it is
// 480×640; turned twice it would be on its side again.
func TestDecodeHEICTurnsOnce(t *testing.T) {
	img, err := Load("../imagefmt/testdata/rotated.heic", 0)
	if err != nil {
		t.Fatal(err)
	}
	if img.Bounds() != image.Rect(0, 0, 480, 640) {
		t.Fatalf("size %v, want 480×640", img.Bounds())
	}
}

// exifOrientationSegment is a JPEG APP1 segment holding EXIF orientation o:
// what goexif finds when it scans a file that does not start with TIFF data.
func exifOrientationSegment(o uint16) []byte {
	tiff := []byte("MM\x00\x2a\x00\x00\x00\x08")
	tiff = binary.BigEndian.AppendUint16(tiff, 1)
	tiff = binary.BigEndian.AppendUint16(tiff, 0x0112) // Orientation
	tiff = binary.BigEndian.AppendUint16(tiff, 3)      // SHORT
	tiff = binary.BigEndian.AppendUint32(tiff, 1)
	tiff = binary.BigEndian.AppendUint16(tiff, o)
	tiff = append(tiff, 0, 0, 0, 0, 0, 0) // value padding, no next IFD
	seg := append([]byte("Exif\x00\x00"), tiff...)
	out := []byte{0xff, 0xe1}
	out = binary.BigEndian.AppendUint16(out, uint16(len(seg)+2))
	return append(out, seg...)
}

// goexif scans a file it does not recognise for any FF E1 marker, and those
// bytes can turn up anywhere in HEVC data. What it then reads must not turn
// a HEIF image the decoder has already set upright.
func TestDecodeHEICIgnoresStrayEXIFOrientation(t *testing.T) {
	b, err := os.ReadFile("../imagefmt/testdata/rotated.heic")
	if err != nil {
		t.Fatal(err)
	}
	// In a free box, which a HEIF reader skips.
	seg := exifOrientationSegment(2)
	b = binary.BigEndian.AppendUint32(b, uint32(8+len(seg)))
	b = append(append(b, "free"...), seg...)
	if o := exifOrientation(b); o != 2 {
		t.Fatalf("test data reads as orientation %d, want 2", o)
	}
	raw, _, err := imagefmt.Decode(b)
	if err != nil {
		t.Fatal(err)
	}
	got, err := Decode(b, 0)
	if err != nil {
		t.Fatal(err)
	}
	if want := downscale(raw, 0); !bytes.Equal(got.Pix, want.Pix) {
		t.Fatal("EXIF orientation was applied on top of the HEIF decoder's")
	}
}
