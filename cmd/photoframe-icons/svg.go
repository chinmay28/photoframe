package main

import (
	"encoding/xml"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"strconv"
	"strings"

	"golang.org/x/image/vector"
)

// The app icon, web/icons/icon.svg, is drawn from a handful of flat shapes.
// This reads exactly the SVG it uses — rect (with rx), circle, and paths of
// absolute M, L, Q and Z, each with a #RRGGBB fill and an optional opacity —
// and refuses anything else, so a richer redraw of the artwork fails loudly
// here instead of rendering wrong.

type shape struct {
	fill    color.NRGBA
	opacity float64
	// path is in viewBox units: moves, lines, quads and closes.
	path []segment
}

type segment struct {
	op  byte // 'M', 'L', 'Q', 'C', 'Z'
	pts []float32
}

type artwork struct {
	size   float32 // the viewBox is 0 0 size size
	shapes []shape
}

// plate is the first shape, the rounded tile the scene sits on: its colour
// becomes the adaptive icon's background, and the adaptive foreground is
// everything else.
func (a artwork) plate() color.NRGBA { return a.shapes[0].fill }

func parseSVG(data []byte) (artwork, error) {
	var doc struct {
		ViewBox string `xml:"viewBox,attr"`
		Nodes   []struct {
			XMLName xml.Name
			Attrs   []xml.Attr `xml:",any,attr"`
		} `xml:",any"`
	}
	if err := xml.Unmarshal(data, &doc); err != nil {
		return artwork{}, err
	}
	vb := strings.Fields(doc.ViewBox)
	if len(vb) != 4 || vb[0] != "0" || vb[1] != "0" || vb[2] != vb[3] {
		return artwork{}, fmt.Errorf("viewBox %q: want a square from 0 0", doc.ViewBox)
	}
	size, err := strconv.ParseFloat(vb[2], 32)
	if err != nil {
		return artwork{}, err
	}
	art := artwork{size: float32(size)}
	for i, n := range doc.Nodes {
		at := map[string]string{}
		for _, a := range n.Attrs {
			at[a.Name.Local] = a.Value
		}
		num := func(k string) float32 {
			if err != nil {
				return 0
			}
			v, e := strconv.ParseFloat(at[k], 32)
			if e != nil && at[k] != "" {
				err = fmt.Errorf("element %d: %s=%q: %v", i, k, at[k], e)
			}
			return float32(v)
		}
		s := shape{opacity: 1}
		if s.fill, err = hexColour(at["fill"]); err != nil {
			return artwork{}, fmt.Errorf("element %d: %v", i, err)
		}
		if o, ok := at["opacity"]; ok {
			if s.opacity, err = strconv.ParseFloat(o, 64); err != nil {
				return artwork{}, err
			}
		}
		switch n.XMLName.Local {
		case "rect":
			s.path = rect(num("x"), num("y"), num("width"), num("height"), num("rx"))
		case "circle":
			s.path = circle(num("cx"), num("cy"), num("r"))
		case "path":
			s.path, err = pathData(at["d"])
		default:
			return artwork{}, fmt.Errorf("element %d: <%s> is not drawn by this generator", i, n.XMLName.Local)
		}
		if err != nil {
			return artwork{}, err
		}
		art.shapes = append(art.shapes, s)
	}
	if len(art.shapes) == 0 {
		return artwork{}, fmt.Errorf("no shapes")
	}
	return art, nil
}

func hexColour(s string) (color.NRGBA, error) {
	if len(s) != 7 || s[0] != '#' {
		return color.NRGBA{}, fmt.Errorf("fill %q: want #RRGGBB", s)
	}
	v, err := strconv.ParseUint(s[1:], 16, 32)
	if err != nil {
		return color.NRGBA{}, fmt.Errorf("fill %q: %v", s, err)
	}
	return color.NRGBA{uint8(v >> 16), uint8(v >> 8), uint8(v), 0xff}, nil
}

// kappa places a cubic's control points to approximate a quarter circle.
const kappa = 0.5522847498

func rect(x, y, w, h, r float32) []segment {
	if r <= 0 {
		return []segment{{'M', []float32{x, y}}, {'L', []float32{x + w, y}}, {'L', []float32{x + w, y + h}}, {'L', []float32{x, y + h}}, {'Z', nil}}
	}
	r = min(r, w/2, h/2)
	k := r * kappa
	return []segment{
		{'M', []float32{x + r, y}},
		{'L', []float32{x + w - r, y}},
		{'C', []float32{x + w - r + k, y, x + w, y + r - k, x + w, y + r}},
		{'L', []float32{x + w, y + h - r}},
		{'C', []float32{x + w, y + h - r + k, x + w - r + k, y + h, x + w - r, y + h}},
		{'L', []float32{x + r, y + h}},
		{'C', []float32{x + r - k, y + h, x, y + h - r + k, x, y + h - r}},
		{'L', []float32{x, y + r}},
		{'C', []float32{x, y + r - k, x + r - k, y, x + r, y}},
		{'Z', nil},
	}
}

func circle(cx, cy, r float32) []segment {
	k := r * kappa
	return []segment{
		{'M', []float32{cx + r, cy}},
		{'C', []float32{cx + r, cy + k, cx + k, cy + r, cx, cy + r}},
		{'C', []float32{cx - k, cy + r, cx - r, cy + k, cx - r, cy}},
		{'C', []float32{cx - r, cy - k, cx - k, cy - r, cx, cy - r}},
		{'C', []float32{cx + k, cy - r, cx + r, cy - k, cx + r, cy}},
		{'Z', nil},
	}
}

func pathData(d string) ([]segment, error) {
	tok := strings.Fields(strings.NewReplacer(",", " ", "M", " M ", "L", " L ", "Q", " Q ", "Z", " Z ").Replace(d))
	var out []segment
	args := map[byte]int{'M': 2, 'L': 2, 'Q': 4, 'Z': 0}
	for i := 0; i < len(tok); {
		op := tok[i]
		n, ok := args[op[0]]
		if len(op) != 1 || !ok {
			return nil, fmt.Errorf("path %q: %q is not M, L, Q or Z", d, op)
		}
		i++
		if i+n > len(tok) {
			return nil, fmt.Errorf("path %q: %s needs %d numbers", d, op, n)
		}
		pts := make([]float32, n)
		for j := range pts {
			v, err := strconv.ParseFloat(tok[i+j], 32)
			if err != nil {
				return nil, fmt.Errorf("path %q: %v", d, err)
			}
			pts[j] = float32(v)
		}
		i += n
		out = append(out, segment{op[0], pts})
	}
	return out, nil
}

// render draws the artwork into a size×size image. The viewBox is scaled to
// span px pixels with its corner at (off, off); skipPlate leaves the first
// shape out. Pixels nothing covers stay transparent.
func (a artwork) render(size int, px, off float32, skipPlate bool) *image.NRGBA {
	dst := image.NewNRGBA(image.Rect(0, 0, size, size))
	scale := px / a.size
	for i, s := range a.shapes {
		if skipPlate && i == 0 {
			continue
		}
		z := vector.NewRasterizer(size, size)
		at := func(p []float32, k int) (float32, float32) { return off + p[k]*scale, off + p[k+1]*scale }
		for _, seg := range s.path {
			switch seg.op {
			case 'M':
				z.MoveTo(at(seg.pts, 0))
			case 'L':
				z.LineTo(at(seg.pts, 0))
			case 'Q':
				x1, y1 := at(seg.pts, 0)
				x, y := at(seg.pts, 2)
				z.QuadTo(x1, y1, x, y)
			case 'C':
				x1, y1 := at(seg.pts, 0)
				x2, y2 := at(seg.pts, 2)
				x, y := at(seg.pts, 4)
				z.CubeTo(x1, y1, x2, y2, x, y)
			case 'Z':
				z.ClosePath()
			}
		}
		c := s.fill
		c.A = uint8(float64(c.A)*s.opacity + 0.5)
		z.Draw(dst, dst.Bounds(), image.NewUniform(c), image.Point{})
	}
	return dst
}

// roundMask cuts img to the circle inscribed in it, anti-aliased: the
// developer badge as a round icon.
func roundMask(img image.Image) *image.NRGBA {
	b := img.Bounds()
	w := float32(b.Dx())
	dst := image.NewNRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	z := vector.NewRasterizer(b.Dx(), b.Dy())
	for _, seg := range circle(w/2, float32(b.Dy())/2, w/2) {
		switch seg.op {
		case 'M':
			z.MoveTo(seg.pts[0], seg.pts[1])
		case 'C':
			z.CubeTo(seg.pts[0], seg.pts[1], seg.pts[2], seg.pts[3], seg.pts[4], seg.pts[5])
		case 'Z':
			z.ClosePath()
		}
	}
	z.DrawOp = draw.Src
	src := image.NewNRGBA(dst.Bounds())
	draw.Draw(src, src.Bounds(), img, b.Min, draw.Src)
	z.Draw(dst, dst.Bounds(), src, image.Point{})
	return dst
}
