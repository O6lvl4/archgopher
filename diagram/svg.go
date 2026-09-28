package diagram

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/O6lvl4/archgopher/icons"
)

// svg writes the scene as a self-contained SVG: the icons it uses are
// embedded as symbols, every style is on the element, no font is fetched.
func (g *graph) svg() []byte {
	ops := g.scene()
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %s %s" width="%s" height="%s" font-family="Helvetica Neue, Helvetica, Arial, Hiragino Sans, Noto Sans JP, sans-serif">`+"\n", num(g.w), num(g.h), num(g.w), num(g.h))
	fmt.Fprintf(&b, "<title>%s</title>\n<defs>\n", esc(g.spec.Name))
	fmt.Fprintf(&b, `<marker id="arrow" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="7" markerHeight="7" orient="auto-start-reverse"><path d="M0,0 L10,5 L0,10 z" fill="%s"/></marker>`+"\n", edgeColor)
	for _, name := range iconNames(ops) {
		if s := symbol(name); s != "" {
			b.WriteString(s + "\n")
		}
	}
	b.WriteString("</defs>\n")
	for _, o := range ops {
		switch o.kind {
		case opRect:
			fmt.Fprintf(&b, `<rect x="%s" y="%s" width="%s" height="%s"`, num(o.x), num(o.y), num(o.w), num(o.h))
			if o.rx > 0 {
				fmt.Fprintf(&b, ` rx="%s"`, num(o.rx))
			}
			b.WriteString(paint(o))
			b.WriteString("/>\n")
		case opPoly:
			var d strings.Builder
			for i, p := range o.pts {
				if i == 0 {
					fmt.Fprintf(&d, "M%s,%s", num(p.x), num(p.y))
				} else {
					fmt.Fprintf(&d, " L%s,%s", num(p.x), num(p.y))
				}
			}
			fmt.Fprintf(&b, `<path d="%s" fill="none" stroke="%s" stroke-width="%s"`, d.String(), o.stroke, num(o.width))
			if len(o.dash) > 0 {
				fmt.Fprintf(&b, ` stroke-dasharray="%s"`, dashes(o.dash))
			}
			if o.arrow {
				b.WriteString(` marker-end="url(#arrow)"`)
			}
			b.WriteString("/>\n")
		case opText:
			fmt.Fprintf(&b, `<text x="%s" y="%s" font-size="%s" fill="%s" text-anchor="%s"`, num(o.x), num(o.y), num(o.size), o.color, o.anchor)
			if o.bold {
				b.WriteString(` font-weight="700"`)
			}
			if o.mono {
				b.WriteString(` font-family="SF Mono, Menlo, Consolas, Liberation Mono, monospace"`)
			}
			if o.halo {
				fmt.Fprintf(&b, ` paint-order="stroke" stroke="%s" stroke-width="3" stroke-linejoin="round"`, paperColor)
			}
			if o.squeeze > 0 {
				fmt.Fprintf(&b, ` textLength="%s" lengthAdjust="spacingAndGlyphs"`, num(o.squeeze))
			}
			fmt.Fprintf(&b, ">%s</text>\n", esc(o.text))
		case opIcon:
			fmt.Fprintf(&b, `<use href="#i-%s" x="%s" y="%s" width="%s" height="%s"/>`+"\n", iconID(o.icon), num(o.x), num(o.y), num(o.w), num(o.h))
		}
	}
	b.WriteString("</svg>\n")
	return []byte(b.String())
}

// paint is a rect's fill and stroke attributes.
func paint(o op) string {
	var b strings.Builder
	if o.fill == "" {
		b.WriteString(` fill="none"`)
	} else {
		fmt.Fprintf(&b, ` fill="%s"`, o.fill)
		if o.alpha > 0 && o.alpha < 1 {
			fmt.Fprintf(&b, ` fill-opacity="%s"`, strings.TrimRight(fmt.Sprintf("%.3f", o.alpha), "0"))
		}
	}
	if o.stroke != "" {
		fmt.Fprintf(&b, ` stroke="%s" stroke-width="%s"`, o.stroke, num(o.width))
		if len(o.dash) > 0 {
			fmt.Fprintf(&b, ` stroke-dasharray="%s"`, dashes(o.dash))
		}
	}
	return b.String()
}

func dashes(d []float64) string {
	parts := make([]string, len(d))
	for i, v := range d {
		parts[i] = num(v)
	}
	return strings.Join(parts, " ")
}

// iconSource is the SVG text of an icon or a glyph, or nil.
func iconSource(name string) []byte {
	if g, ok := strings.CutPrefix(name, "glyph:"); ok {
		if s, ok := glyphs[g]; ok {
			return []byte(s)
		}
		return nil
	}
	data, err := icons.Read(name)
	if err != nil {
		return nil
	}
	return data
}

func iconExists(name string) bool { return iconSource(name) != nil }

// iconID is the symbol id of an icon: "aws/lambda" → "aws-lambda".
func iconID(name string) string { return strings.NewReplacer("/", "-", ":", "-").Replace(name) }

var (
	svgOuter   = regexp.MustCompile(`(?s)<svg[^>]*>(.*)</svg>`)
	svgViewBox = regexp.MustCompile(`viewBox="([^"]+)"`)
	svgTitle   = regexp.MustCompile(`(?s)<title>.*?</title>`)
	svgID      = regexp.MustCompile(`id="([^"]+)"`)
	svgRef     = regexp.MustCompile(`url\(#([^)]+)\)`)
	svgHref    = regexp.MustCompile(`href="#([^"]+)"`)
)

// symbol wraps an icon file as a <symbol>, its ids prefixed so several icons
// in one document never share one.
func symbol(name string) string {
	data := iconSource(name)
	if data == nil {
		return ""
	}
	m := svgOuter.FindSubmatch(data)
	if m == nil {
		return ""
	}
	viewBox := "0 0 64 64"
	if vb := svgViewBox.FindSubmatch(data); vb != nil {
		viewBox = string(vb[1])
	}
	body := string(m[1])
	body = svgTitle.ReplaceAllString(body, "")
	prefix := "ic-" + iconID(name) + "-"
	body = svgID.ReplaceAllString(body, `id="`+prefix+`$1"`)
	body = svgRef.ReplaceAllString(body, `url(#`+prefix+`$1)`)
	body = svgHref.ReplaceAllString(body, `href="#`+prefix+`$1"`)
	return fmt.Sprintf(`<symbol id="i-%s" viewBox="%s">%s</symbol>`, iconID(name), viewBox, strings.TrimSpace(body))
}

// esc escapes text for an attribute or element.
func esc(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;").Replace(s)
}
