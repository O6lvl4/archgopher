package diagram

import (
	"fmt"
	"math"
	"sort"
	"strings"
)

// An op is one drawing instruction. The SVG writer and the raster read the
// same list, so the two pictures cannot drift apart.
type op struct {
	kind opKind
	// rect and text
	x, y, w, h float64
	rx         float64
	// poly
	pts   []point
	arrow bool
	// paint
	fill   string  // "" for none
	alpha  float64 // of the fill; 0 means opaque
	stroke string  // "" for none
	width  float64
	dash   []float64
	// text
	text    string
	size    float64
	bold    bool
	mono    bool
	color   string
	anchor  string // start, middle or end
	halo    bool
	squeeze float64 // draw the text no wider than this, when set
	// icon: "<provider>/<name>" from the icons, or "glyph:<name>" from glyphs
	icon string
}

type opKind int

const (
	opRect opKind = iota
	opPoly
	opText
	opIcon
)

// The palette. Frames follow the AWS architecture group colours.
const (
	inkColor      = "#16191f"
	mutedColor    = "#545b64"
	chipColor     = "#6b7580"
	edgeColor     = "#545b64"
	cloudColor    = "#242f3e"
	regionColor   = "#147eba"
	vpcColor      = "#8c4fff"
	otherColor    = "#545b64"
	missingFill   = "#f2f3f3"
	missingStroke = "#879596"
	paperColor    = "#ffffff"
)

// glyphs are small pictures of the diagram's own, drawn like icons.
var glyphs = map[string]string{
	"cloud": `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 22 22"><rect width="22" height="22" rx="2" fill="` + cloudColor + `"/><path fill="#ffffff" d="M6.5,15.5 h9.5 a3,3 0 0 0 0.4,-6 a4.2,4.2 0 0 0 -8,-1.6 a3.6,3.6 0 0 0 -1.9,7.6 z"/></svg>`,
}

// clouds names the frame drawn around a provider's nodes.
var clouds = map[string]string{
	"aws": "AWS Cloud", "azure": "Microsoft Azure", "gcp": "Google Cloud", "cloudflare": "Cloudflare", "conoha": "ConoHa",
}

// scene lists what to draw, back to front.
func (g *graph) scene() []op {
	var ops []op
	ops = append(ops, op{kind: opRect, x: 0, y: 0, w: g.w, h: g.h, fill: paperColor})
	if g.cloud.w > 0 {
		c := g.cloud
		ops = append(ops,
			op{kind: opRect, x: c.x, y: c.y, w: c.w, h: c.h, rx: 4, stroke: cloudColor, width: 1.4},
			op{kind: opIcon, icon: "glyph:cloud", x: c.x + 8, y: c.y + 8, w: 22, h: 22},
			op{kind: opText, text: clouds[g.provider], x: c.x + 38, y: c.y + 23, size: 12, bold: true, color: cloudColor, anchor: "start"},
		)
	}
	if g.region.w > 0 {
		r := g.region
		ops = append(ops,
			op{kind: opRect, x: r.x, y: r.y, w: r.w, h: r.h, rx: 4, stroke: regionColor, width: 1.2, dash: []float64{6, 4}},
			op{kind: opText, text: "Region " + g.spec.Region, x: r.x + 12, y: r.y + 20, size: 12, bold: true, color: regionColor, anchor: "start"},
		)
	}
	for _, f := range g.frames {
		if len(f.members) == 0 {
			continue
		}
		label := f.g.Label
		if label == "" {
			label = f.g.ID
		}
		if isVPC(f.g.Kind) {
			ops = append(ops, op{kind: opRect, x: f.x, y: f.y, w: f.w, h: f.h, rx: 4, fill: vpcColor, alpha: 0.035, stroke: vpcColor, width: 1.4})
		} else {
			ops = append(ops, op{kind: opRect, x: f.x, y: f.y, w: f.w, h: f.h, rx: 4, stroke: otherColor, width: 1.1, dash: []float64{4, 3}})
		}
		color := otherColor
		if isVPC(f.g.Kind) {
			color = vpcColor
		}
		ops = append(ops,
			op{kind: opText, text: f.g.Kind, x: f.x + 12, y: f.y + 20, size: 12, bold: true, color: color, anchor: "start"},
			op{kind: opText, text: label, x: f.x + 12 + 7.5*float64(len(f.g.Kind)+1), y: f.y + 20, size: 12, mono: true, color: color, anchor: "start"},
		)
	}
	for _, e := range g.edges {
		ops = append(ops, g.edgeOps(e)...)
	}
	for _, d := range g.nodes {
		ops = append(ops, g.nodeOps(d)...)
	}
	return ops
}

// isVPC tells the network boundaries (VPC, VNet, VPC network) from other groups.
func isVPC(kind string) bool {
	k := strings.ToLower(kind)
	return strings.Contains(k, "vpc") || strings.Contains(k, "vnet") || strings.Contains(k, "network")
}

// edgeOps draws one edge from icon edge to icon edge through its bends, with
// its label beside the first stretch.
func (g *graph) edgeOps(e *edge) []op {
	pts := make([]point, 0, len(e.via)+2)
	first, last := point{e.to.cx, e.to.cy}, point{e.from.cx, e.from.cy}
	if len(e.via) > 0 {
		first, last = e.via[0], e.via[len(e.via)-1]
	}
	pts = append(pts, anchorFrom(e.from, first))
	pts = append(pts, e.via...)
	pts = append(pts, anchorTo(e.to, last))
	ops := []op{{kind: opPoly, pts: pts, stroke: edgeColor, width: 1.3, arrow: true}}
	if e.label != "" {
		a, b := pts[0], pts[1]
		mx, my := (a.x+b.x)/2, (a.y+b.y)/2
		l := math.Hypot(b.x-a.x, b.y-a.y)
		if l == 0 {
			l = 1
		}
		nx, ny := -(b.y-a.y)/l, (b.x-a.x)/l
		if ny > 0 {
			nx, ny = -nx, -ny
		}
		ops = append(ops, op{kind: opText, text: e.label, x: mx + nx*9, y: my + ny*9 + 3, size: 10, color: inkColor, anchor: "middle", halo: true})
	}
	return ops
}

// anchorFrom is the point on a node's edge facing p: a side of the icon when
// p is off to one side, else the bottom of the labels or the top of the icon.
func anchorFrom(a *node, p point) point {
	switch dx := p.x - a.cx; {
	case dx >= 40:
		return point{a.cx + iconSize/2, a.cy}
	case dx <= -40:
		return point{a.cx - iconSize/2, a.cy}
	case p.y >= a.cy:
		return point{a.cx, a.bottom()}
	default:
		return point{a.cx, a.top()}
	}
}

// anchorTo is the point on a node's edge an arrow from p arrives at.
func anchorTo(c *node, p point) point {
	switch dx := c.cx - p.x; {
	case dx >= 40:
		return point{c.cx - iconSize/2, c.cy}
	case dx <= -40:
		return point{c.cx + iconSize/2, c.cy}
	case p.y < c.cy:
		return point{c.cx, c.top()}
	default:
		return point{c.cx, c.bottom()}
	}
}

// nodeOps draws a node's icon with its label lines under it.
func (g *graph) nodeOps(d *node) []op {
	x, y := d.cx-iconSize/2, d.top()
	var ops []op
	if d.meta.Icon != "" && iconExists(d.meta.Icon) {
		ops = append(ops, op{kind: opIcon, icon: d.meta.Icon, x: x, y: y, w: iconSize, h: iconSize})
	} else {
		ops = append(ops, op{kind: opRect, x: x, y: y, w: iconSize, h: iconSize, rx: 4, fill: missingFill, stroke: missingStroke, width: 1})
	}
	for i, line := range d.lines {
		t := op{kind: opText, text: line, x: d.cx, y: y + iconSize + 13 + lineH*float64(i), anchor: "middle", halo: true}
		switch {
		case i == 0:
			t.size, t.bold, t.color = 11, true, inkColor
		case i == 1 && line == d.id:
			t.size, t.mono, t.color = 9.5, true, mutedColor
		default:
			t.size, t.color, t.halo = 9, chipColor, true
		}
		if len([]rune(line)) > 30 {
			t.squeeze = nodeW - 6
		}
		ops = append(ops, t)
	}
	return ops
}

// iconNames lists the icons and glyphs a scene uses, once each, sorted.
func iconNames(ops []op) []string {
	seen := map[string]bool{}
	for _, o := range ops {
		if o.kind == opIcon {
			seen[o.icon] = true
		}
	}
	names := make([]string, 0, len(seen))
	for n := range seen {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// num formats a coordinate without trailing zeros.
func num(f float64) string {
	return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.1f", f), "0"), ".")
}
