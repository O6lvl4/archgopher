package diagram

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"regexp"
	"strconv"

	"github.com/fogleman/gg"
	"github.com/srwiley/oksvg"
	"github.com/srwiley/rasterx"

	"github.com/O6lvl4/archgopher/model"
)

// PNG draws the declaration as a PNG, scale pixels to one unit of the SVG
// (2 makes a picture crisp on a high-density screen).
func PNG(spec model.Spec, lookup Lookup, scale float64) ([]byte, error) {
	g, err := build(spec, lookup)
	if err != nil {
		return nil, err
	}
	g.layout()
	img, err := g.raster(scale)
	if err != nil {
		return nil, err
	}
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

// raster draws the scene into an image, every unit scaled by s.
func (g *graph) raster(s float64) (*image.RGBA, error) {
	if s <= 0 {
		s = 1
	}
	w, h := int(math.Ceil(g.w*s)), int(math.Ceil(g.h*s))
	if w == 0 || h == 0 {
		return nil, fmt.Errorf("nothing to draw")
	}
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	dc := gg.NewContextForRGBA(img)
	fs := loadFonts()
	for _, o := range g.scene() {
		switch o.kind {
		case opRect:
			drawRect(dc, o, s)
		case opPoly:
			drawPoly(dc, o, s)
		case opText:
			drawText(dc, fs, o, s)
		case opIcon:
			if err := drawIcon(img, o, s); err != nil {
				return nil, err
			}
		}
	}
	return img, nil
}

func drawRect(dc *gg.Context, o op, s float64) {
	if o.rx > 0 {
		dc.DrawRoundedRectangle(o.x*s, o.y*s, o.w*s, o.h*s, o.rx*s)
	} else {
		dc.DrawRectangle(o.x*s, o.y*s, o.w*s, o.h*s)
	}
	if o.fill != "" {
		dc.SetColor(rgba(o.fill, o.alpha))
		if o.stroke != "" {
			dc.FillPreserve()
		} else {
			dc.Fill()
		}
	}
	if o.stroke != "" {
		dc.SetColor(rgba(o.stroke, 0))
		dc.SetLineWidth(o.width * s)
		dc.SetDash(scaled(o.dash, s)...)
		dc.Stroke()
		dc.SetDash()
	}
}

func drawPoly(dc *gg.Context, o op, s float64) {
	if len(o.pts) < 2 {
		return
	}
	pts := o.pts
	end := pts[len(pts)-1]
	if o.arrow {
		// Stop the line short of the tip so the head is not drawn over.
		prev := pts[len(pts)-2]
		dx, dy := end.x-prev.x, end.y-prev.y
		l := math.Hypot(dx, dy)
		if l > 0 {
			pts = append(append([]point{}, pts[:len(pts)-1]...), point{end.x - dx/l*6, end.y - dy/l*6})
		}
	}
	dc.SetColor(rgba(o.stroke, 0))
	dc.SetLineWidth(o.width * s)
	dc.SetDash(scaled(o.dash, s)...)
	for i, p := range pts {
		if i == 0 {
			dc.MoveTo(p.x*s, p.y*s)
		} else {
			dc.LineTo(p.x*s, p.y*s)
		}
	}
	dc.Stroke()
	dc.SetDash()
	if o.arrow {
		prev := o.pts[len(o.pts)-2]
		dx, dy := end.x-prev.x, end.y-prev.y
		l := math.Hypot(dx, dy)
		if l == 0 {
			return
		}
		ux, uy := dx/l, dy/l
		bx, by := end.x-ux*8, end.y-uy*8
		dc.MoveTo(end.x*s, end.y*s)
		dc.LineTo((bx-uy*4)*s, (by+ux*4)*s)
		dc.LineTo((bx+uy*4)*s, (by-ux*4)*s)
		dc.ClosePath()
		dc.Fill()
	}
}

func drawText(dc *gg.Context, fs *fontSet, o op, s float64) {
	size := o.size * s
	face := fs.face(o, size)
	if face == nil {
		return
	}
	dc.SetFontFace(face)
	w, _ := dc.MeasureString(o.text)
	if o.squeeze > 0 && w > o.squeeze*s {
		size *= o.squeeze * s / w
		if face = fs.face(o, size); face == nil {
			return
		}
		dc.SetFontFace(face)
		w, _ = dc.MeasureString(o.text)
	}
	x, y := o.x*s, o.y*s
	switch o.anchor {
	case "middle":
		x -= w / 2
	case "end":
		x -= w
	}
	if o.halo {
		dc.SetColor(rgba(paperColor, 0))
		r := 1.2 * s
		for _, d := range [][2]float64{{-r, 0}, {r, 0}, {0, -r}, {0, r}, {-r, -r}, {r, -r}, {-r, r}, {r, r}} {
			dc.DrawString(o.text, x+d[0], y+d[1])
		}
	}
	dc.SetColor(rgba(o.color, 0))
	dc.DrawString(o.text, x, y)
}

// drawIcon rasterizes an icon's SVG straight into the image.
func drawIcon(img *image.RGBA, o op, s float64) error {
	src := iconSource(o.icon)
	if src == nil {
		return fmt.Errorf("icon %q: not found", o.icon)
	}
	icon, err := oksvg.ReadIconStream(bytes.NewReader(transformsForOksvg(src)))
	if err != nil {
		return fmt.Errorf("icon %q: %v", o.icon, err)
	}
	icon.SetTarget(o.x*s, o.y*s, o.w*s, o.h*s)
	b := img.Bounds()
	scanner := rasterx.NewScannerGV(b.Dx(), b.Dy(), img, b)
	icon.Draw(rasterx.NewDasher(b.Dx(), b.Dy(), scanner), 1)
	return nil
}

var (
	spacedTranslate = regexp.MustCompile(`translate\(\s*(-?[\d.]+)\s+(-?[\d.]+)\s*\)`)
	singleScale     = regexp.MustCompile(`scale\(\s*(-?[\d.]+)\s*\)`)
)

// transformsForOksvg rewrites the transform forms oksvg does not read, a
// translate with its two numbers separated by a space and a scale with one
// number, into the forms it does.
func transformsForOksvg(src []byte) []byte {
	src = spacedTranslate.ReplaceAll(src, []byte("translate($1,$2)"))
	return singleScale.ReplaceAll(src, []byte("scale($1,$1)"))
}

// rgba parses "#rrggbb"; alpha 0 means opaque.
func rgba(hex string, alpha float64) color.Color {
	v, err := strconv.ParseUint(hex[1:], 16, 32)
	if err != nil || len(hex) != 7 {
		return color.Black
	}
	a := 1.0
	if alpha > 0 {
		a = alpha
	}
	return color.NRGBA{R: uint8(v >> 16), G: uint8(v >> 8), B: uint8(v), A: uint8(math.Round(a * 255))}
}

func scaled(d []float64, s float64) []float64 {
	if len(d) == 0 {
		return nil
	}
	out := make([]float64, len(d))
	for i, v := range d {
		out[i] = v * s
	}
	return out
}
