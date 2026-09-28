package diagram

import (
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"

	"github.com/O6lvl4/archgopher/icons"
)

// clouds names the frame drawn around a provider's nodes.
var clouds = map[string]string{
	"aws": "AWS Cloud", "azure": "Microsoft Azure", "gcp": "Google Cloud", "cloudflare": "Cloudflare", "conoha": "ConoHa",
}

const style = `
text { font-family: "Helvetica Neue", Helvetica, Arial, "Hiragino Sans", "Noto Sans JP", sans-serif; fill: #16191f; }
.mono { font-family: "SF Mono", Menlo, Consolas, "Liberation Mono", monospace; }
.name { font-size: 11px; font-weight: 700; }
.id { font-size: 9.5px; fill: #545b64; }
.chip { font-size: 9px; fill: #6b7580; }
.halo { paint-order: stroke; stroke: #ffffff; stroke-width: 3px; stroke-linejoin: round; }
.edge { fill: none; stroke: #545b64; stroke-width: 1.3; }
.elabel { font-size: 10px; }
.cloud { fill: none; stroke: #242f3e; stroke-width: 1.4; }
.cloud-tile { fill: #242f3e; } .cloud-glyph { fill: #ffffff; }
.region { fill: none; stroke: #147eba; stroke-width: 1.2; stroke-dasharray: 6 4; }
.frame-vpc { fill: rgba(140,79,255,0.035); stroke: #8c4fff; stroke-width: 1.4; }
.frame-other { fill: none; stroke: #545b64; stroke-width: 1.1; stroke-dasharray: 4 3; }
.title { font-size: 12px; font-weight: 700; }
.title-cloud { fill: #242f3e; } .title-region { fill: #147eba; } .title-vpc { fill: #8c4fff; } .title-other { fill: #545b64; }
.sub { font-size: 9.5px; fill: #6b7580; }
.missing { fill: #f2f3f3; stroke: #879596; stroke-width: 1; }
`

// svg renders the laid-out graph.
func (g *graph) svg() []byte {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %s %s" width="%s" height="%s" font-size="11">`+"\n", num(g.w), num(g.h), num(g.w), num(g.h))
	fmt.Fprintf(&b, "<title>%s</title>\n", esc(g.spec.Name))
	b.WriteString("<style>" + style + "</style>\n")
	b.WriteString("<defs>\n")
	b.WriteString(`<marker id="arrow" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="7" markerHeight="7" orient="auto-start-reverse"><path d="M0,0 L10,5 L0,10 z" fill="#545b64"/></marker>` + "\n")
	for _, name := range g.iconNames() {
		if s := symbol(name); s != "" {
			b.WriteString(s + "\n")
		}
	}
	b.WriteString("</defs>\n")
	fmt.Fprintf(&b, `<rect width="%s" height="%s" fill="#ffffff"/>`+"\n", num(g.w), num(g.h))
	if g.cloud.w > 0 {
		c := g.cloud
		fmt.Fprintf(&b, `<rect class="cloud" x="%s" y="%s" width="%s" height="%s" rx="4"/>`+"\n", num(c.x), num(c.y), num(c.w), num(c.h))
		fmt.Fprintf(&b, `<g transform="translate(%s,%s)"><rect class="cloud-tile" width="22" height="22" rx="2"/><path class="cloud-glyph" d="M6.5,15.5 h9.5 a3,3 0 0 0 0.4,-6 a4.2,4.2 0 0 0 -8,-1.6 a3.6,3.6 0 0 0 -1.9,7.6 z"/></g>`+"\n", num(c.x+8), num(c.y+8))
		fmt.Fprintf(&b, `<text class="title title-cloud" x="%s" y="%s">%s</text>`+"\n", num(c.x+38), num(c.y+23), esc(clouds[g.provider]))
	}
	if g.region.w > 0 {
		r := g.region
		fmt.Fprintf(&b, `<rect class="region" x="%s" y="%s" width="%s" height="%s" rx="4"/>`+"\n", num(r.x), num(r.y), num(r.w), num(r.h))
		fmt.Fprintf(&b, `<text class="title title-region" x="%s" y="%s">Region %s</text>`+"\n", num(r.x+12), num(r.y+20), esc(g.spec.Region))
	}
	for _, f := range g.frames {
		if len(f.members) == 0 {
			continue
		}
		cls, title := "other", "other"
		if isVPC(f.g.Kind) {
			cls, title = "vpc", "vpc"
		}
		label := f.g.Label
		if label == "" {
			label = f.g.ID
		}
		fmt.Fprintf(&b, `<rect class="frame-%s" x="%s" y="%s" width="%s" height="%s" rx="4"/>`+"\n", cls, num(f.x), num(f.y), num(f.w), num(f.h))
		fmt.Fprintf(&b, `<text class="title title-%s" x="%s" y="%s">%s <tspan class="mono" font-weight="400">%s</tspan></text>`+"\n", title, num(f.x+12), num(f.y+20), esc(f.g.Kind), esc(label))
	}
	for _, e := range g.edges {
		b.WriteString(g.edgeSVG(e))
	}
	for _, d := range g.nodes {
		b.WriteString(g.nodeSVG(d))
	}
	b.WriteString("</svg>\n")
	return []byte(b.String())
}

// isVPC tells the network boundaries (VPC, VNet, VPC network) from other groups.
func isVPC(kind string) bool {
	k := strings.ToLower(kind)
	return strings.Contains(k, "vpc") || strings.Contains(k, "vnet") || strings.Contains(k, "network")
}

// edgeSVG draws one edge from icon edge to icon edge through its bends, with
// its label beside the first stretch.
func (g *graph) edgeSVG(e *edge) string {
	pts := make([]point, 0, len(e.via)+2)
	first, last := point{e.to.cx, e.to.cy}, point{e.from.cx, e.from.cy}
	if len(e.via) > 0 {
		first, last = e.via[0], e.via[len(e.via)-1]
	}
	pts = append(pts, anchorFrom(e.from, first))
	pts = append(pts, e.via...)
	pts = append(pts, anchorTo(e.to, last))
	var d strings.Builder
	for i, p := range pts {
		if i == 0 {
			fmt.Fprintf(&d, "M%s,%s", num(p.x), num(p.y))
		} else {
			fmt.Fprintf(&d, " L%s,%s", num(p.x), num(p.y))
		}
	}
	out := fmt.Sprintf(`<path class="edge" d="%s" marker-end="url(#arrow)"/>`+"\n", d.String())
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
		out += fmt.Sprintf(`<text class="elabel halo" x="%s" y="%s" text-anchor="middle">%s</text>`+"\n", num(mx+nx*9), num(my+ny*9+3), esc(e.label))
	}
	return out
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

// nodeSVG draws a node's icon with its label lines under it.
func (g *graph) nodeSVG(d *node) string {
	var b strings.Builder
	x, y := d.cx-iconSize/2, d.top()
	if d.meta.Icon != "" && iconExists(d.meta.Icon) {
		fmt.Fprintf(&b, `<use href="#i-%s" x="%s" y="%s" width="%s" height="%s"/>`+"\n", iconID(d.meta.Icon), num(x), num(y), num(iconSize), num(iconSize))
	} else {
		fmt.Fprintf(&b, `<rect class="missing" x="%s" y="%s" width="%s" height="%s" rx="4"/>`+"\n", num(x), num(y), num(iconSize), num(iconSize))
	}
	for i, line := range d.lines {
		cls := "chip"
		switch i {
		case 0:
			cls = "name halo"
		case 1:
			if line == d.id {
				cls = "id mono halo"
			}
		}
		squeeze := ""
		if len([]rune(line)) > 30 {
			squeeze = fmt.Sprintf(` textLength="%s" lengthAdjust="spacingAndGlyphs"`, num(nodeW-6))
		}
		fmt.Fprintf(&b, `<text class="%s" x="%s" y="%s" text-anchor="middle"%s>%s</text>`+"\n", cls, num(d.cx), num(y+iconSize+13+lineH*float64(i)), squeeze, esc(line))
	}
	return b.String()
}

// iconNames lists the icons the drawn nodes use, once each.
func (g *graph) iconNames() []string {
	seen := map[string]bool{}
	for _, d := range g.nodes {
		if d.meta.Icon != "" {
			seen[d.meta.Icon] = true
		}
	}
	names := make([]string, 0, len(seen))
	for n := range seen {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

func iconExists(name string) bool {
	_, err := icons.Read(name)
	return err == nil
}

// iconID is the symbol id of an icon: "aws/lambda" → "aws-lambda".
func iconID(name string) string { return strings.ReplaceAll(name, "/", "-") }

var (
	svgOuter = regexp.MustCompile(`(?s)<svg[^>]*>(.*)</svg>`)
	svgTitle = regexp.MustCompile(`(?s)<title>.*?</title>`)
	svgID    = regexp.MustCompile(`id="([^"]+)"`)
	svgRef   = regexp.MustCompile(`url\(#([^)]+)\)`)
	svgHref  = regexp.MustCompile(`href="#([^"]+)"`)
)

// symbol wraps an icon file as a <symbol>, its ids prefixed so several icons
// in one document never share one.
func symbol(name string) string {
	data, err := icons.Read(name)
	if err != nil {
		return ""
	}
	m := svgOuter.FindSubmatch(data)
	if m == nil {
		return ""
	}
	body := string(m[1])
	body = svgTitle.ReplaceAllString(body, "")
	prefix := "ic-" + iconID(name) + "-"
	body = svgID.ReplaceAllString(body, `id="`+prefix+`$1"`)
	body = svgRef.ReplaceAllString(body, `url(#`+prefix+`$1)`)
	body = svgHref.ReplaceAllString(body, `href="#`+prefix+`$1"`)
	return fmt.Sprintf(`<symbol id="i-%s" viewBox="0 0 64 64">%s</symbol>`, iconID(name), strings.TrimSpace(body))
}

// num formats a coordinate without trailing zeros.
func num(f float64) string {
	return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.1f", f), "0"), ".")
}

// esc escapes text for an attribute or element.
func esc(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;").Replace(s)
}
