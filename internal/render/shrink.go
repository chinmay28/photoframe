package render

import "image"

// shrinkFactor is the largest whole factor that a w×h image can be divided
// by while both sides stay at least as large as a maxSide fit needs. It
// returns 1 when the image needs no whole-factor shrinking.
func shrinkFactor(w, h, maxSide int) int {
	if maxSide <= 0 {
		return 1
	}
	return max(1, max(w, h)/maxSide)
}

// shrinkYCbCr box-averages src by a whole factor f, giving an image of
// ⌊w/f⌋×⌊h/f⌋ with full-resolution chroma. It reads each source sample once,
// so its cost is a small fraction of decoding the JPEG the image came from;
// a general-purpose scaler spends several times the decode on the same job.
// It reports false, doing nothing, for factors below 2, for images smaller
// than f on a side and for chroma layouts it does not handle.
func shrinkYCbCr(src *image.YCbCr, f int) (*image.YCbCr, bool) {
	var cx, cy int // chroma subsampling along x and y
	switch src.SubsampleRatio {
	case image.YCbCrSubsampleRatio444:
		cx, cy = 1, 1
	case image.YCbCrSubsampleRatio422:
		cx, cy = 2, 1
	case image.YCbCrSubsampleRatio420:
		cx, cy = 2, 2
	case image.YCbCrSubsampleRatio440:
		cx, cy = 1, 2
	default:
		return nil, false
	}
	b := src.Bounds()
	w, h := b.Dx()/f, b.Dy()/f
	if f < 2 || w < 1 || h < 1 {
		return nil, false
	}
	dst := image.NewYCbCr(image.Rect(0, 0, w, h), image.YCbCrSubsampleRatio444)

	// Luma: every output pixel is the mean of an f×f block.
	shrinkPlane(dst.Y, dst.YStride, w, h, src.Y[src.YOffset(b.Min.X, b.Min.Y):], src.YStride, f, f)

	// Chroma: the block of samples covering the same f×f pixels. With odd
	// factors and subsampled chroma the blocks are a sample off at most,
	// which is far below what survives the shrink.
	c0 := src.COffset(b.Min.X, b.Min.Y)
	cw := (b.Max.X+cx-1)/cx - b.Min.X/cx
	ch := (b.Max.Y+cy-1)/cy - b.Min.Y/cy
	fx, fy := max(1, f/cx), max(1, f/cy)
	for _, p := range [2]struct{ dst, src []uint8 }{{dst.Cb, src.Cb}, {dst.Cr, src.Cr}} {
		shrinkChroma(p.dst, dst.CStride, w, h, p.src[c0:], src.CStride, cw, ch, f, cx, cy, fx, fy)
	}
	return dst, true
}

// shrinkPlane writes the mean of each fx×fy block of src into dst (w×h).
func shrinkPlane(dst []uint8, dstStride, w, h int, src []uint8, srcStride, fx, fy int) {
	sums := make([]uint32, w)
	n := uint32(fx * fy)
	for y := 0; y < h; y++ {
		clear(sums)
		for j := 0; j < fy; j++ {
			row := src[(y*fy+j)*srcStride:]
			row = row[:w*fx]
			for x := range sums {
				var s uint32
				for _, v := range row[x*fx : x*fx+fx] {
					s += uint32(v)
				}
				sums[x] += s
			}
		}
		out := dst[y*dstStride : y*dstStride+w]
		for x, s := range sums {
			out[x] = uint8((s + n/2) / n)
		}
	}
}

// shrinkChroma is shrinkPlane for a plane subsampled by cx×cy: output pixel
// (x, y) averages the fx×fy samples starting at (x·f/cx, y·f/cy), clamped
// to the cw×ch plane.
func shrinkChroma(dst []uint8, dstStride, w, h int, src []uint8, srcStride, cw, ch, f, cx, cy, fx, fy int) {
	n := uint32(fx * fy)
	for y := 0; y < h; y++ {
		y0 := min(y*f/cy, ch-fy)
		out := dst[y*dstStride : y*dstStride+w]
		for x := range out {
			x0 := min(x*f/cx, cw-fx)
			var s uint32
			for j := 0; j < fy; j++ {
				i := (y0+j)*srcStride + x0
				for _, v := range src[i : i+fx] {
					s += uint32(v)
				}
			}
			out[x] = uint8((s + n/2) / n)
		}
	}
}

// shrinkPixels is shrinkYCbCr for the four-bytes-a-pixel images that decoders
// other than JPEG return, HEIC's among them: it box-averages an *image.RGBA or
// *image.NRGBA by a whole factor f, keeping its type. NRGBA channels are
// averaged as stored, which is exact for the opaque pixels of a photo. It
// reports false, doing nothing, for other types, for factors below 2 and for
// images smaller than f on a side.
func shrinkPixels(src image.Image, f int) (image.Image, bool) {
	var pix []uint8
	var stride int
	b := src.Bounds()
	switch s := src.(type) {
	case *image.RGBA:
		pix, stride = s.Pix[s.PixOffset(b.Min.X, b.Min.Y):], s.Stride
	case *image.NRGBA:
		pix, stride = s.Pix[s.PixOffset(b.Min.X, b.Min.Y):], s.Stride
	default:
		return nil, false
	}
	w, h := b.Dx()/f, b.Dy()/f
	if f < 2 || w < 1 || h < 1 {
		return nil, false
	}
	r := image.Rect(0, 0, w, h)
	var dst []uint8
	var out image.Image
	if _, ok := src.(*image.RGBA); ok {
		d := image.NewRGBA(r)
		dst, out = d.Pix, d
	} else {
		d := image.NewNRGBA(r)
		dst, out = d.Pix, d
	}
	shrinkInterleaved(dst, 4*w, w, h, pix, stride, f)
	return out, true
}

// shrinkInterleaved writes the mean of each f×f block of src, a plane of
// four interleaved channels, into dst (w×h pixels), channel by channel.
func shrinkInterleaved(dst []uint8, dstStride, w, h int, src []uint8, srcStride, f int) {
	sums := make([]uint32, 4*w)
	n := uint32(f * f)
	for y := 0; y < h; y++ {
		clear(sums)
		for j := 0; j < f; j++ {
			row := src[(y*f+j)*srcStride:]
			row = row[:4*w*f]
			for x := 0; x < w; x++ {
				s := sums[4*x : 4*x+4 : 4*x+4]
				block := row[4*x*f : 4*(x+1)*f]
				for k := 0; k < len(block); k += 4 {
					s[0] += uint32(block[k])
					s[1] += uint32(block[k+1])
					s[2] += uint32(block[k+2])
					s[3] += uint32(block[k+3])
				}
			}
		}
		out := dst[y*dstStride : y*dstStride+4*w]
		for i, s := range sums {
			out[i] = uint8((s + n/2) / n)
		}
	}
}
