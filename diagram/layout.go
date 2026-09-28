package diagram

import (
	"math"
	"sort"
)

const (
	iconSize  = 48.0
	nodeW     = 176.0 // room for the label lines under the icon
	lineH     = 12.0
	colGap    = 40.0
	rowGap    = 28.0
	extGap    = 70.0 // between the sources outside the cloud and its frame
	marginX   = 40.0
	marginY   = 40.0
	framePad  = 22.0 // inside a frame, around its members
	frameHead = 30.0 // a frame's title
	// positionScale maps the web UI's card grid (236 × 140) onto icons.
	positionScale = 0.75
)

func (d *node) height() float64 { return iconSize + 4 + lineH*float64(len(d.lines)) }
func (d *node) top() float64    { return d.cy - iconSize/2 }
func (d *node) bottom() float64 { return d.top() + d.height() }
func (d *node) left() float64   { return d.cx - nodeW/2 }
func (d *node) right() float64  { return d.cx + nodeW/2 }

// cell is what the layered layout places: one node, or a whole group laid
// out on its own and placed as one block.
type cell struct {
	id     string
	w, h   float64
	x, y   float64 // top-left corner
	ext    bool
	layer  int
	order  int
	degree int
	node   *node
	frame  *frame
	inner  *lay
	dummy  bool // a bend of a long edge, holding a lane in a column it crosses
}

type cedge struct {
	from, to *cell
	back     bool    // closes a cycle; ignored by the layering
	via      []*cell // the dummies a long edge bends through, left to right
}

// lay is one run of the layered layout over cells, from (0,0).
type lay struct {
	cells []*cell
	edges []*cedge
	segs  []*cedge // the edges with long ones split at their dummies
	w, h  float64
}

const dummyH = 14.0

// layout places every node, then fits the frames around them.
func (g *graph) layout() {
	if len(g.nodes) == 0 {
		g.w, g.h = 2*marginX, 2*marginY
		return
	}
	if g.usePositions() {
		for range 5 {
			g.fitFrames()
			if !g.pushIntruders() {
				break
			}
		}
		g.fitFrames()
		g.outer()
		return
	}
	member := map[*node]*frame{}
	for _, f := range g.frames {
		for _, m := range f.members {
			member[m] = f
		}
	}
	cedges := map[*edge]*cedge{}
	// Each group is laid out on its own, then placed as one block.
	inner := map[*frame]*lay{}
	for _, f := range g.frames {
		if len(f.members) == 0 {
			continue
		}
		l := &lay{}
		cellOf := map[*node]*cell{}
		for _, m := range f.members {
			c := &cell{id: m.id, w: nodeW, h: m.height(), node: m}
			cellOf[m] = c
			l.cells = append(l.cells, c)
		}
		for _, e := range g.edges {
			if a, b := cellOf[e.from], cellOf[e.to]; a != nil && b != nil && a != b {
				ce := &cedge{from: a, to: b}
				l.edges = append(l.edges, ce)
				cedges[e] = ce
			}
		}
		l.run()
		inner[f] = l
	}
	outer := &lay{}
	cellOf := map[*node]*cell{}
	fcell := map[*frame]*cell{}
	for _, d := range g.nodes {
		if member[d] != nil {
			continue
		}
		c := &cell{id: d.id, w: nodeW, h: d.height(), node: d, ext: d.ext}
		cellOf[d] = c
		outer.cells = append(outer.cells, c)
	}
	for _, f := range g.frames {
		l := inner[f]
		if l == nil {
			continue
		}
		c := &cell{id: "group:" + f.g.ID, w: l.w + 2*framePad, h: l.h + 2*framePad + frameHead, frame: f, inner: l}
		fcell[f] = c
		outer.cells = append(outer.cells, c)
	}
	resolve := func(d *node) *cell {
		if c := cellOf[d]; c != nil {
			return c
		}
		return fcell[member[d]]
	}
	for _, e := range g.edges {
		if a, b := resolve(e.from), resolve(e.to); a != nil && b != nil && a != b {
			ce := &cedge{from: a, to: b}
			outer.edges = append(outer.edges, ce)
			cedges[e] = ce
		}
	}
	outer.run()
	origin := map[*cell]point{} // where each lay's (0,0) landed
	for _, c := range outer.cells {
		if c.dummy {
			continue
		}
		if c.node != nil {
			c.node.cx, c.node.cy = c.x+c.w/2, c.y+iconSize/2
			continue
		}
		f := c.frame
		f.x, f.y, f.w, f.h = c.x, c.y, c.w, c.h
		for _, ic := range c.inner.cells {
			origin[ic] = point{f.x + framePad, f.y + frameHead + framePad}
			if ic.node != nil {
				ic.node.cx = f.x + framePad + ic.x + ic.w/2
				ic.node.cy = f.y + frameHead + framePad + ic.y + iconSize/2
			}
		}
	}
	for e, ce := range cedges {
		for _, v := range ce.via {
			o := origin[v]
			e.via = append(e.via, point{o.x + v.x + v.w/2, o.y + v.y + v.h/2})
		}
	}
	g.outer()
}

// usePositions takes the positions the web UI saved when every drawn node has one.
func (g *graph) usePositions() bool {
	for _, d := range g.nodes {
		if d.src.Position == nil {
			return false
		}
	}
	for _, d := range g.nodes {
		d.cx = (d.src.Position.X + 118) * positionScale
		d.cy = (d.src.Position.Y + 70) * positionScale
	}
	return true
}

// run lays the cells out left to right from (0,0).
func (l *lay) run() {
	for _, e := range l.edges {
		e.from.degree++
		e.to.degree++
	}
	l.layers()
	l.split()
	l.orderLayers()
	l.place()
	l.straighten()
}

// straighten pulls each bend of a long edge onto the straight line between
// its ends, where that line clears the cells of the bend's column, so an edge
// that has room runs straight and one that has not keeps its lane.
func (l *lay) straighten() {
	cols := l.columns()
	center := func(c *cell) (float64, float64) {
		if c.node != nil {
			return c.x + c.w/2, c.y + iconSize/2
		}
		return c.x + c.w/2, c.y + c.h/2
	}
	for _, e := range l.edges {
		if len(e.via) == 0 {
			continue
		}
		x0, y0 := center(e.from)
		x1, y1 := center(e.to)
		for _, v := range e.via {
			vx, _ := center(v)
			want := y0 + (y1-y0)*(vx-x0)/(x1-x0)
			clear := true
			for _, c := range cols[v.layer] {
				if c.dummy {
					continue
				}
				if want > c.y-10 && want < c.y+c.h+10 {
					clear = false
					break
				}
			}
			if clear {
				v.y = want - v.h/2
			}
		}
	}
}

// split bends every edge that skips columns through a dummy cell in each
// column it crosses, so the ordering keeps a lane for it and it is drawn
// around the cells there instead of over them.
func (l *lay) split() {
	for _, e := range l.edges {
		if e.back || e.to.layer-e.from.layer <= 1 {
			l.segs = append(l.segs, e)
			continue
		}
		prev := e.from
		for layer := e.from.layer + 1; layer < e.to.layer; layer++ {
			d := &cell{id: "dummy", h: dummyH, layer: layer, dummy: true}
			l.cells = append(l.cells, d)
			e.via = append(e.via, d)
			l.segs = append(l.segs, &cedge{from: prev, to: d})
			prev = d
		}
		l.segs = append(l.segs, &cedge{from: prev, to: e.to})
	}
}

// layers assigns each cell its column: the longest path from a source,
// sources outside the cloud first, cycles broken by ignoring the edge that
// closes them, and cells without edges beside the busiest cell of their module.
func (l *lay) layers() {
	succ := map[*cell][]*cedge{}
	for _, e := range l.edges {
		succ[e.from] = append(succ[e.from], e)
	}
	// Depth-first: an edge to a cell still on the stack closes a cycle.
	state := map[*cell]int{} // 0 new, 1 on the stack, 2 done
	var visit func(c *cell)
	visit = func(c *cell) {
		state[c] = 1
		for _, e := range succ[c] {
			switch state[e.to] {
			case 0:
				visit(e.to)
			case 1:
				e.back = true
			}
		}
		state[c] = 2
	}
	for _, c := range l.cells {
		if state[c] == 0 {
			visit(c)
		}
	}
	indeg := map[*cell]int{}
	for _, e := range l.edges {
		if !e.back {
			indeg[e.to]++
		}
	}
	var queue []*cell
	for _, c := range l.cells {
		if indeg[c] == 0 {
			queue = append(queue, c)
		}
	}
	for len(queue) > 0 {
		c := queue[0]
		queue = queue[1:]
		for _, e := range succ[c] {
			if e.back {
				continue
			}
			if e.to.layer < c.layer+1 {
				e.to.layer = c.layer + 1
			}
			indeg[e.to]--
			if indeg[e.to] == 0 {
				queue = append(queue, e.to)
			}
		}
	}
	hasExt := false
	for _, c := range l.cells {
		if c.ext {
			hasExt = true
		}
	}
	if hasExt {
		for _, c := range l.cells {
			if c.ext {
				c.layer = 0
			} else {
				c.layer++
			}
		}
	}
	for _, c := range l.cells {
		if c.degree > 0 || c.ext {
			continue
		}
		if o := l.busiest(module(c.id), c); o != nil {
			c.layer = o.layer
		} else if hasExt {
			c.layer = 1
		}
	}
}

// busiest is the cell of a module with the most edges, other than skip.
func (l *lay) busiest(mod string, skip *cell) *cell {
	var best *cell
	for _, c := range l.cells {
		if c == skip || c.degree == 0 || module(c.id) != mod {
			continue
		}
		if best == nil || c.degree > best.degree {
			best = c
		}
	}
	return best
}

// columns groups the cells by layer, each column in its current order.
func (l *lay) columns() [][]*cell {
	last := 0
	for _, c := range l.cells {
		last = max(last, c.layer)
	}
	cols := make([][]*cell, last+1)
	for _, c := range l.cells {
		cols[c.layer] = append(cols[c.layer], c)
	}
	for _, col := range cols {
		sort.SliceStable(col, func(i, j int) bool { return col[i].order < col[j].order })
	}
	return cols
}

// orderLayers sorts each column by the mean position of its neighbours in
// the columns beside it, a few sweeps each way.
func (l *lay) orderLayers() {
	for i, c := range l.cells {
		c.order = i
	}
	pred := map[*cell][]*cell{}
	succ := map[*cell][]*cell{}
	for _, e := range l.segs {
		if e.back {
			continue
		}
		pred[e.to] = append(pred[e.to], e.from)
		succ[e.from] = append(succ[e.from], e.to)
	}
	cols := l.columns()
	sweep := func(col []*cell, near map[*cell][]*cell) {
		key := map[*cell]float64{}
		for i, c := range col {
			key[c] = float64(i)
			if ns := near[c]; len(ns) > 0 {
				sum := 0.0
				for _, n := range ns {
					sum += float64(n.order)
				}
				key[c] = sum / float64(len(ns))
			}
		}
		sort.SliceStable(col, func(i, j int) bool { return key[col[i]] < key[col[j]] })
		for i, c := range col {
			c.order = i
		}
	}
	for range 4 {
		for i := 1; i < len(cols); i++ {
			sweep(cols[i], pred)
		}
		for i := len(cols) - 2; i >= 0; i-- {
			sweep(cols[i], succ)
		}
	}
}

// place gives each column the width of its widest cell and stacks its
// cells, centred on the tallest column.
func (l *lay) place() {
	cols := l.columns()
	tallest := 0.0
	heights := make([]float64, len(cols))
	widths := make([]float64, len(cols))
	for i, col := range cols {
		for j, c := range col {
			heights[i] += c.h
			if j > 0 {
				heights[i] += rowGap
			}
			widths[i] = math.Max(widths[i], c.w)
		}
		tallest = math.Max(tallest, heights[i])
	}
	hasExt := len(cols) > 0 && len(cols[0]) > 0 && cols[0][0].ext
	x := 0.0
	for i, col := range cols {
		if hasExt && i == 1 {
			x += extGap
		}
		y := (tallest - heights[i]) / 2
		for _, c := range col {
			c.x = x + (widths[i]-c.w)/2
			c.y = y
			y += c.h + rowGap
		}
		x += widths[i]
		if i < len(cols)-1 {
			x += colGap
		}
	}
	l.w, l.h = x, tallest
}

// fitFrames draws each group around its members.
func (g *graph) fitFrames() {
	for _, f := range g.frames {
		if len(f.members) == 0 {
			continue
		}
		x0, y0 := math.Inf(1), math.Inf(1)
		x1, y1 := math.Inf(-1), math.Inf(-1)
		for _, d := range f.members {
			x0 = math.Min(x0, d.left())
			y0 = math.Min(y0, d.top())
			x1 = math.Max(x1, d.right())
			y1 = math.Max(y1, d.bottom())
		}
		f.x, f.y = x0-framePad, y0-framePad-frameHead
		f.w, f.h = x1-x0+2*framePad, y1-y0+2*framePad+frameHead
	}
}

// pushIntruders moves nodes that fell inside a frame they do not belong to
// below it, then re-spaces their columns. It reports whether anything moved.
func (g *graph) pushIntruders() bool {
	moved := false
	for _, f := range g.frames {
		if len(f.members) == 0 {
			continue
		}
		inside := map[*node]bool{}
		for _, d := range f.members {
			inside[d] = true
		}
		for _, d := range g.nodes {
			if inside[d] || d.cx < f.x || d.cx > f.x+f.w || d.bottom() < f.y || d.top() > f.y+f.h {
				continue
			}
			d.cy = f.y + f.h + rowGap + iconSize/2
			moved = true
		}
	}
	if moved {
		g.respace()
	}
	return moved
}

// respace pushes overlapping nodes of one column apart, top to bottom.
func (g *graph) respace() {
	byX := map[float64][]*node{}
	for _, d := range g.nodes {
		byX[d.cx] = append(byX[d.cx], d)
	}
	for _, col := range byX {
		sort.SliceStable(col, func(i, j int) bool { return col[i].cy < col[j].cy })
		for i := 1; i < len(col); i++ {
			if want := col[i-1].bottom() + rowGap; col[i].top() < want {
				col[i].cy = want + iconSize/2
			}
		}
	}
}

// outer fits the cloud and region frames around the inside nodes and sets
// the canvas, shifting everything so nothing is left of or above the margins.
func (g *graph) outer() {
	x0, y0 := math.Inf(1), math.Inf(1)
	x1, y1 := math.Inf(-1), math.Inf(-1)
	include := func(l, t, r, b float64) {
		x0, y0 = math.Min(x0, l), math.Min(y0, t)
		x1, y1 = math.Max(x1, r), math.Max(y1, b)
	}
	if g.provider != "" {
		for _, d := range g.nodes {
			if !d.ext {
				include(d.left(), d.top(), d.right(), d.bottom())
			}
		}
		for _, f := range g.frames {
			if len(f.members) > 0 {
				include(f.x, f.y, f.x+f.w, f.y+f.h)
			}
		}
		if x0 < x1 {
			inner := box{x0 - framePad, y0 - framePad - frameHead, x1 - x0 + 2*framePad, y1 - y0 + 2*framePad + frameHead}
			if g.spec.Region == "" {
				g.cloud = inner
			} else {
				g.region = inner
				g.cloud = box{inner.x - 16, inner.y - 16 - frameHead, inner.w + 32, inner.h + 32 + frameHead}
			}
		}
	}
	x0, y0, x1, y1 = math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)
	for _, d := range g.nodes {
		include(d.left(), d.top(), d.right(), d.bottom())
	}
	for _, f := range g.frames {
		if len(f.members) > 0 {
			include(f.x, f.y, f.x+f.w, f.y+f.h)
		}
	}
	if g.cloud.w > 0 {
		include(g.cloud.x, g.cloud.y, g.cloud.x+g.cloud.w, g.cloud.y+g.cloud.h)
	}
	dx, dy := marginX-x0, marginY-y0
	for _, d := range g.nodes {
		d.cx += dx
		d.cy += dy
	}
	for _, e := range g.edges {
		for i := range e.via {
			e.via[i].x += dx
			e.via[i].y += dy
		}
	}
	for _, f := range g.frames {
		f.x += dx
		f.y += dy
	}
	g.cloud.x += dx
	g.cloud.y += dy
	g.region.x += dx
	g.region.y += dy
	g.w = x1 + dx + marginX
	g.h = y1 + dy + marginY
}
