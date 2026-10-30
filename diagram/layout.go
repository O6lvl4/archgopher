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
	port   int  // portIn or portOut: where edges cross the edge of a group
}

// A port is the place an edge crosses its group's frame: edges from outside
// come in at the left edge, edges to outside leave at the right edge, so
// inside the group they hold lanes like any other edge.
const (
	portIn  = 1
	portOut = 2
)

type cedge struct {
	from, to       *cell
	fromOff, toOff float64 // the edge's height at each end, from the top of the cell
	back           bool    // closes a cycle; laid out reversed
	rev            bool    // reversed to break a cycle: drawn from to back to from
	pieces         []*cedge
	// a piece: one column to the next, routed in its lay's coordinates
	pts    []point
	fanout bool // shares a track with the other pieces leaving its source
}

// lay is one run of the layered layout over cells, from (0,0).
type lay struct {
	cells  []*cell
	edges  []*cedge
	segs   []*cedge // the pieces of every edge
	w, h   float64
	broken bool    // cycles already broken
	dots   []point // where a shared track branches, in the lay's coordinates
}

const (
	dummyH    = 14.0
	laneGap   = 10.0 // between a lane and what is beside it in a column
	spreadGap = 12.0 // between the arrows reaching one node
	trackGap  = 12.0 // between the vertical stretches in the gap between two columns
)

// anchor is the offset of a cell's edges from its top when they do not name
// one: the middle of a node's icon, the middle of anything else.
func anchor(c *cell) float64 {
	if c.node != nil {
		return iconSize / 2
	}
	return c.h / 2
}

// layout places every node, then fits the frames around them and routes the
// edges between them.
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
	// The outer lay first, so the edges that close a cycle there are known
	// before the groups give ports to the others.
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
		if len(f.members) == 0 {
			continue
		}
		c := &cell{id: "group:" + f.g.ID, frame: f}
		fcell[f] = c
		outer.cells = append(outer.cells, c)
	}
	resolve := func(d *node) *cell {
		if c := cellOf[d]; c != nil {
			return c
		}
		return fcell[member[d]]
	}
	across := map[*edge]*cedge{}
	for _, e := range g.edges {
		if a, b := resolve(e.from), resolve(e.to); a != nil && b != nil && a != b {
			ce := &cedge{from: a, to: b, fromOff: anchor(a), toOff: anchor(b)}
			outer.edges = append(outer.edges, ce)
			across[e] = ce
		}
	}
	outer.breakCycles()
	// Each group is laid out on its own, with a port for every member an
	// edge from outside reaches and every member an edge to outside leaves.
	within := map[*edge]*cedge{}
	in, out := map[*edge]*cedge{}, map[*edge]*cedge{}
	for _, f := range g.frames {
		if len(f.members) == 0 {
			continue
		}
		l := &lay{}
		mcell := map[*node]*cell{}
		for _, m := range f.members {
			c := &cell{id: m.id, w: nodeW, h: m.height(), node: m}
			mcell[m] = c
			l.cells = append(l.cells, c)
		}
		inPort, outPort := map[*node]*cedge{}, map[*node]*cedge{}
		portEdge := func(ports map[*node]*cedge, m *node, kind int) *cedge {
			if ce := ports[m]; ce != nil {
				return ce
			}
			p := &cell{id: "port:" + m.id, h: dummyH, port: kind}
			l.cells = append(l.cells, p)
			ce := &cedge{from: p, to: mcell[m], fromOff: dummyH / 2, toOff: iconSize / 2}
			if kind == portOut {
				ce = &cedge{from: mcell[m], to: p, fromOff: iconSize / 2, toOff: dummyH / 2}
			}
			l.edges = append(l.edges, ce)
			ports[m] = ce
			return ce
		}
		for _, e := range g.edges {
			a, b := mcell[e.from], mcell[e.to]
			ce := across[e]
			from, to := e.from, e.to // in the direction the outer lay runs the edge
			if ce != nil && ce.rev {
				from, to = to, from
			}
			switch {
			case a != nil && b != nil:
				c := &cedge{from: a, to: b, fromOff: iconSize / 2, toOff: iconSize / 2}
				l.edges = append(l.edges, c)
				within[e] = c
			case ce == nil:
			case mcell[to] != nil:
				in[e] = portEdge(inPort, to, portIn)
			case mcell[from] != nil:
				out[e] = portEdge(outPort, from, portOut)
			}
		}
		l.run()
		c := fcell[f]
		c.inner = l
		c.w, c.h = l.w+2*framePad, l.h+2*framePad+frameHead
	}
	for e, ce := range across {
		if p := in[e]; p != nil {
			ce.toOff = frameHead + framePad + p.from.y + p.fromOff
		}
		if p := out[e]; p != nil {
			ce.fromOff = frameHead + framePad + p.to.y + p.toOff
		}
	}
	outer.run()
	g.dots = append(g.dots, outer.dots...)
	origin := map[*frame]point{} // where each group's lay has its (0,0)
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
		o := point{f.x + framePad, f.y + frameHead + framePad}
		origin[f] = o
		for _, d := range c.inner.dots {
			g.dots = append(g.dots, point{o.x + d.x, o.y + d.y})
		}
		for _, ic := range c.inner.cells {
			if ic.node != nil {
				ic.node.cx = o.x + ic.x + ic.w/2
				ic.node.cy = o.y + ic.y + iconSize/2
			}
		}
	}
	for _, e := range g.edges {
		var path []point
		follow := func(ce *cedge, o point) {
			if ce == nil {
				return
			}
			for _, p := range ce.pieces {
				for _, q := range p.pts {
					path = append(path, point{o.x + q.x, o.y + q.y})
				}
			}
		}
		// The path in the direction the layout ran the edge, then turned
		// round when that was against the edge.
		var first *cedge
		rev := false
		if ce := within[e]; ce != nil {
			follow(ce, origin[member[e.from]])
			first, rev = ce, ce.rev
		} else if ce := across[e]; ce != nil {
			from, to := e.from, e.to
			if ce.rev {
				from, to = to, from
			}
			follow(out[e], origin[member[from]])
			follow(ce, point{})
			follow(in[e], origin[member[to]])
			first, rev = out[e], ce.rev
			if first == nil {
				first = ce
			}
		}
		if len(path) < 2 {
			continue
		}
		if rev {
			for i, j := 0, len(path)-1; i < j; i, j = i+1, j-1 {
				path[i], path[j] = path[j], path[i]
			}
		}
		e.path = dedupe(path)
		e.labelAtEnd = !rev && len(first.pieces) > 0 && first.pieces[0].fanout
	}
	g.outer()
}

// dedupe drops repeated points and the middle of three on one straight line.
func dedupe(pts []point) []point {
	out := pts[:0:0]
	for _, p := range pts {
		if n := len(out); n > 0 && math.Abs(out[n-1].x-p.x) < 0.01 && math.Abs(out[n-1].y-p.y) < 0.01 {
			continue
		}
		if n := len(out); n >= 2 {
			a, b := out[n-2], out[n-1]
			if (math.Abs(a.x-b.x) < 0.01 && math.Abs(b.x-p.x) < 0.01) || (math.Abs(a.y-b.y) < 0.01 && math.Abs(b.y-p.y) < 0.01) {
				out[n-1] = p
				continue
			}
		}
		out = append(out, p)
	}
	return out
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

// run lays the cells out left to right from (0,0) and routes the edges.
func (l *lay) run() {
	for _, e := range l.edges {
		e.from.degree++
		e.to.degree++
	}
	l.breakCycles()
	l.layers()
	l.split()
	l.orderLayers()
	l.spread()
	l.placeY()
	l.placeX()
}

// breakCycles marks the edges that close a cycle, depth first: an edge to a
// cell still on the stack closes one.
func (l *lay) breakCycles() {
	if l.broken {
		return
	}
	l.broken = true
	succ := map[*cell][]*cedge{}
	for _, e := range l.edges {
		succ[e.from] = append(succ[e.from], e)
	}
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
	// Each edge that closes a cycle is laid out the other way round, so it
	// is routed like the others and drawn back in its own direction.
	for _, e := range l.edges {
		if e.back {
			e.from, e.to = e.to, e.from
			e.fromOff, e.toOff = e.toOff, e.fromOff
			e.back, e.rev = false, true
		}
	}
}

// split cuts every edge into pieces from one column to the next, bending an
// edge that skips columns through a dummy cell in each column it crosses, so
// the ordering keeps a lane for it and it runs between the cells there
// instead of over them.
func (l *lay) split() {
	for _, e := range l.edges {
		if e.back || e.to.layer-e.from.layer <= 1 {
			e.pieces = []*cedge{e}
			l.segs = append(l.segs, e)
			continue
		}
		prev, off := e.from, e.fromOff
		for layer := e.from.layer + 1; layer < e.to.layer; layer++ {
			d := &cell{id: "dummy", h: dummyH, layer: layer, dummy: true}
			l.cells = append(l.cells, d)
			p := &cedge{from: prev, to: d, fromOff: off, toOff: dummyH / 2}
			e.pieces = append(e.pieces, p)
			l.segs = append(l.segs, p)
			prev, off = d, dummyH/2
		}
		p := &cedge{from: prev, to: e.to, fromOff: off, toOff: e.toOff}
		e.pieces = append(e.pieces, p)
		l.segs = append(l.segs, p)
	}
}

// layers assigns each cell its column: the longest path from a source,
// sources outside the cloud first, the ports out of a group last, and cells
// without edges beside the busiest cell of their module.
func (l *lay) layers() {
	succ := map[*cell][]*cedge{}
	for _, e := range l.edges {
		succ[e.from] = append(succ[e.from], e)
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
	l.shorten()
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
	last := 0
	for _, c := range l.cells {
		if c.port != portOut {
			last = max(last, c.layer)
		}
	}
	for _, c := range l.cells {
		if c.port == portOut {
			c.layer = last + 1
		}
	}
}

// shorten moves cells between columns to make the edges shorter, the sum
// network simplex minimises: a cell can sit anywhere right of what calls it
// and left of what it calls, and goes to the end of that range nearer the
// side with more edges. A cell that only writes to a store moves up to the
// store instead of waiting in the first column; a store only one cell writes
// moves down to that cell instead of the last column.
func (l *lay) shorten() {
	pred := map[*cell][]*cell{}
	succ := map[*cell][]*cell{}
	for _, e := range l.edges {
		succ[e.from] = append(succ[e.from], e.to)
		pred[e.to] = append(pred[e.to], e.from)
	}
	for range 4 * len(l.cells) {
		moved := false
		for _, c := range l.cells {
			if c.ext || c.port != 0 || len(pred[c]) == len(succ[c]) {
				continue
			}
			lo, hi := 0, math.MaxInt
			for _, p := range pred[c] {
				lo = max(lo, p.layer+1)
			}
			for _, s := range succ[c] {
				hi = min(hi, s.layer-1)
			}
			want := lo
			if len(succ[c]) > len(pred[c]) {
				want = hi
			}
			if want != c.layer && want >= lo && want <= hi {
				c.layer = want
				moved = true
			}
		}
		if !moved {
			break
		}
	}
	low := math.MaxInt
	for _, c := range l.cells {
		low = min(low, c.layer)
	}
	for _, c := range l.cells {
		c.layer -= low
	}
}

// busiest is the cell of a module with the most edges, other than skip.
func (l *lay) busiest(mod string, skip *cell) *cell {
	var best *cell
	for _, c := range l.cells {
		if c == skip || c.degree == 0 || c.port != 0 || module(c.id) != mod {
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

// orderLayers orders each column to keep edges from crossing, the way dot
// does: sweeps that sort a column by the mean position of its neighbours in
// the column beside it, each followed by swapping neighbours in a column
// while that removes crossings, keeping the best order any sweep found.
// Cells without edges go to the bottom of their column.
func (l *lay) orderLayers() {
	for i, c := range l.cells {
		c.order = i
	}
	pred := map[*cell][]*cell{}
	succ := map[*cell][]*cell{}
	for _, e := range l.segs {
		pred[e.to] = append(pred[e.to], e.from)
		succ[e.from] = append(succ[e.from], e.to)
	}
	alone := func(c *cell) bool { return len(pred[c]) == 0 && len(succ[c]) == 0 }
	cols := l.columns()
	renumber := func(col []*cell) {
		for i, c := range col {
			c.order = i
		}
	}
	sweep := func(col []*cell, near map[*cell][]*cell) {
		key := map[*cell]float64{}
		for i, c := range col {
			switch ns := near[c]; {
			case alone(c):
				key[c] = math.Inf(1)
			case len(ns) > 0:
				sum := 0.0
				for _, n := range ns {
					sum += float64(n.order)
				}
				key[c] = sum / float64(len(ns))
			default:
				key[c] = float64(i)
			}
		}
		sort.SliceStable(col, func(i, j int) bool { return key[col[i]] < key[col[j]] })
		renumber(col)
	}
	// cross counts the crossings between v's edges and w's when v is above w.
	cross := func(v, w *cell) int {
		n := 0
		for _, side := range []map[*cell][]*cell{pred, succ} {
			for _, a := range side[v] {
				for _, b := range side[w] {
					if a.order > b.order {
						n++
					}
				}
			}
		}
		return n
	}
	transpose := func() {
		for range 8 {
			better := false
			for _, col := range cols {
				for i := 0; i+1 < len(col); i++ {
					v, w := col[i], col[i+1]
					if alone(v) || alone(w) || cross(w, v) >= cross(v, w) {
						continue
					}
					col[i], col[i+1] = w, v
					renumber(col)
					better = true
				}
			}
			if !better {
				return
			}
		}
	}
	crossings := func() int {
		n := 0
		for _, col := range cols {
			for i, v := range col {
				for _, w := range col[i+1:] {
					for _, a := range succ[v] {
						for _, b := range succ[w] {
							if a.order > b.order {
								n++
							}
						}
					}
				}
			}
		}
		return n
	}
	best, fewest := map[*cell]int{}, math.MaxInt
	keep := func() {
		if n := crossings(); n < fewest {
			fewest = n
			for _, c := range l.cells {
				best[c] = c.order
			}
		}
	}
	keep()
	for round := range 24 {
		if round%2 == 0 {
			for i := 1; i < len(cols); i++ {
				sweep(cols[i], pred)
			}
		} else {
			for i := len(cols) - 2; i >= 0; i-- {
				sweep(cols[i], succ)
			}
		}
		transpose()
		keep()
	}
	for _, c := range l.cells {
		c.order = best[c]
	}
}

// spread gives each edge reaching a node its own point on the side of the
// node's icon, top to bottom in the order of where the edges come from, so
// the arrows arriving at one node do not run into one another. Edges leaving
// a node keep one point and branch on their track.
func (l *lay) spread() {
	in := map[*cell][]*cedge{}
	for _, s := range l.segs {
		if s.to.node != nil {
			in[s.to] = append(in[s.to], s)
		}
	}
	last := map[*cedge]*cedge{} // the last piece of each edge → the edge
	for _, e := range l.edges {
		last[e.pieces[len(e.pieces)-1]] = e
	}
	for _, c := range l.cells {
		ss := in[c]
		if len(ss) < 2 {
			continue
		}
		sort.SliceStable(ss, func(i, j int) bool {
			a, b := ss[i].from, ss[j].from
			if a.layer != b.layer {
				return a.layer > b.layer
			}
			return a.order < b.order
		})
		step := math.Min(spreadGap, (iconSize-8)/float64(len(ss)-1))
		for j, s := range ss {
			s.toOff = iconSize/2 + (float64(j)-float64(len(ss)-1)/2)*step
			if e := last[s]; e != nil {
				e.toOff = s.toOff
			}
		}
	}
}

// gapBetween is the room between two cells stacked in a column: lanes sit
// closer to their neighbours than nodes do.
func gapBetween(a, b *cell) float64 {
	if a.dummy || b.dummy || a.port != 0 || b.port != 0 {
		return laneGap
	}
	return rowGap
}

// placeY sets each cell's height in its column, keeping the order: every cell
// moves toward the cells it is connected to, so an edge runs straight where
// its ends can line up, and cells that would overlap share the move.
func (l *lay) placeY() {
	cols := l.columns()
	for _, col := range cols {
		y := 0.0
		for i, c := range col {
			if i > 0 {
				y += gapBetween(col[i-1], c)
			}
			c.y = y
			y += c.h
		}
	}
	type link struct {
		other        *cell
		mine, theirs float64
	}
	pred := map[*cell][]link{}
	succ := map[*cell][]link{}
	for _, s := range l.segs {
		if s.back {
			continue
		}
		succ[s.from] = append(succ[s.from], link{s.to, s.fromOff, s.toOff})
		pred[s.to] = append(pred[s.to], link{s.from, s.toOff, s.fromOff})
	}
	align := func(col []*cell, sides ...map[*cell][]link) {
		want := make([]float64, len(col))
		weight := make([]float64, len(col))
		for i, c := range col {
			sum, n := 0.0, 0.0
			for _, side := range sides {
				for _, k := range side[c] {
					sum += k.other.y + k.theirs - k.mine
					n++
				}
			}
			switch {
			case n == 0 && i > 0:
				// No edges: right under the cell above.
				want[i], weight[i] = col[i-1].y+col[i-1].h+gapBetween(col[i-1], c), 0.01
			case n == 0:
				want[i], weight[i] = c.y, 0.01
			case c.dummy:
				// A long edge stays straight before the cells beside it.
				want[i], weight[i] = sum/n, 4*n
			default:
				want[i], weight[i] = sum/n, n
			}
		}
		pack(col, want, weight)
	}
	for range 8 {
		for i := 1; i < len(cols); i++ {
			align(cols[i], pred)
		}
		for i := len(cols) - 2; i >= 0; i-- {
			align(cols[i], succ)
		}
	}
	for range 4 {
		for _, col := range cols {
			align(col, pred, succ)
		}
	}
	l.straighten(cols)
	top, bottom := math.Inf(1), math.Inf(-1)
	for _, c := range l.cells {
		top = math.Min(top, c.y)
		bottom = math.Max(bottom, c.y+c.h)
	}
	for _, c := range l.cells {
		c.y -= top
	}
	l.h = bottom - top
}

// straighten lines up the lanes of each long edge, and the port of an edge
// crossing a group's frame, with the height of one end of the edge, where
// every one of them has room there between its neighbours: the edge then runs
// straight instead of stepping a little at every column. The source's height
// is tried first, then the target's.
func (l *lay) straighten(cols [][]*cell) {
	pos := map[*cell]int{}
	for _, col := range cols {
		for i, c := range col {
			pos[c] = i
		}
	}
	fits := func(c *cell, top float64) bool {
		col, i := cols[c.layer], pos[c]
		if i > 0 {
			if p := col[i-1]; p.y+p.h+gapBetween(p, c) > top+0.01 {
				return false
			}
		}
		if i+1 < len(col) {
			if n := col[i+1]; top+c.h+gapBetween(c, n) > n.y+0.01 {
				return false
			}
		}
		return true
	}
	for range 3 {
		for _, e := range l.edges {
			var lanes []*cell
			for _, p := range e.pieces[1:] {
				lanes = append(lanes, p.from)
			}
			heights := []float64{e.from.y + e.fromOff, e.to.y + e.toOff}
			switch {
			case e.from.port != 0:
				lanes, heights = append(lanes, e.from), heights[1:]
			case e.to.port != 0:
				lanes, heights = append(lanes, e.to), heights[:1]
			}
			if len(lanes) == 0 {
				continue
			}
			for _, y := range heights {
				ok := true
				for _, c := range lanes {
					if !fits(c, y-c.h/2) {
						ok = false
						break
					}
				}
				if ok {
					for _, c := range lanes {
						c.y = y - c.h/2
					}
					break
				}
			}
		}
	}
}

// pack places a column's cells as near the heights they want as their order
// and spacing allow, by weighted least squares: cells that would overlap
// merge into a block placed at the weighted mean of what its cells want.
func pack(col []*cell, want, weight []float64) {
	off := make([]float64, len(col)) // each cell's top when the first is at 0
	for j := 1; j < len(col); j++ {
		off[j] = off[j-1] + col[j-1].h + gapBetween(col[j-1], col[j])
	}
	type block struct {
		first, last int
		sw, swd     float64
	}
	base := func(b block) float64 { return b.swd / b.sw }
	var bs []block
	for i := range col {
		bs = append(bs, block{i, i, weight[i], weight[i] * (want[i] - off[i])})
		for len(bs) > 1 {
			p, q := bs[len(bs)-2], bs[len(bs)-1]
			if base(q) >= base(p) {
				break
			}
			bs = append(bs[:len(bs)-2], block{p.first, q.last, p.sw + q.sw, p.swd + q.swd})
		}
	}
	for _, b := range bs {
		for j := b.first; j <= b.last; j++ {
			col[j].y = base(b) + off[j]
		}
	}
}

// outX is where a cell's edges leave it: the right of a node's icon, the
// right edge of a group, the middle of a lane.
func outX(c *cell) float64 {
	switch {
	case c.node != nil:
		return c.x + c.w/2 + iconSize/2
	case c.frame != nil:
		return c.x + c.w
	}
	return c.x + c.w/2
}

// inX is where edges reach a cell.
func inX(c *cell) float64 {
	switch {
	case c.node != nil:
		return c.x + c.w/2 - iconSize/2
	case c.frame != nil:
		return c.x
	}
	return c.x + c.w/2
}

// placeX gives each column the width of its widest cell and each gap between
// two columns room for the vertical stretches of the edges crossing it, then
// routes every piece: across at its source's height, along its track, and
// across again at its target's height. Pieces leaving one cell share a track,
// as do pieces reaching one cell from cells that send nothing else there.
func (l *lay) placeX() {
	cols := l.columns()
	type bundle struct {
		pieces []*cedge
		lo, hi float64
		ref    float64 // the height the bundle leaves from
		down   bool
		track  int
	}
	gaps := make([][]*bundle, len(cols))
	ntracks := make([]int, len(cols))
	type end struct {
		c *cell
		y int
	}
	for i := range cols {
		var pieces []*cedge
		for _, s := range l.segs {
			if !s.back && s.from.layer == i && math.Abs((s.from.y+s.fromOff)-(s.to.y+s.toOff)) >= 0.5 {
				pieces = append(pieces, s)
			}
		}
		from, to := map[end]int{}, map[end]int{}
		key := func(s *cedge) (end, end) {
			return end{s.from, int(math.Round(s.from.y + s.fromOff))}, end{s.to, int(math.Round(s.to.y + s.toOff))}
		}
		for _, s := range pieces {
			a, b := key(s)
			from[a]++
			to[b]++
		}
		byEnd := map[end]*bundle{}
		var bs []*bundle
		for _, s := range pieces {
			a, b := key(s)
			k := end{}
			switch {
			case from[a] >= 2:
				k = a
				s.fanout = true
			case to[b] >= 2:
				k = b
			}
			bd := byEnd[k]
			if bd == nil || k.c == nil {
				bd = &bundle{lo: math.Inf(1), hi: math.Inf(-1)}
				bs = append(bs, bd)
				if k.c != nil {
					byEnd[k] = bd
				}
			}
			bd.pieces = append(bd.pieces, s)
		}
		for _, bd := range bs {
			sumFrom, sumTo := 0.0, 0.0
			for _, s := range bd.pieces {
				y0, y1 := s.from.y+s.fromOff, s.to.y+s.toOff
				bd.lo = math.Min(bd.lo, math.Min(y0, y1))
				bd.hi = math.Max(bd.hi, math.Max(y0, y1))
				sumFrom += y0
				sumTo += y1
			}
			n := float64(len(bd.pieces))
			bd.ref, bd.down = sumFrom/n, sumTo > sumFrom
		}
		// Going down, the bundle leaving higher takes the track further
		// right; going up, the one leaving lower does: then the stretch across
		// to a track does not cross another's vertical stretch.
		sort.SliceStable(bs, func(a, b int) bool {
			x, y := bs[a], bs[b]
			if x.down != y.down {
				return x.down
			}
			if x.down {
				return x.ref > y.ref
			}
			return x.ref < y.ref
		})
		for j, bd := range bs {
			for _, prev := range bs[:j] {
				if prev.lo < bd.hi+laneGap && bd.lo < prev.hi+laneGap {
					bd.track = max(bd.track, prev.track+1)
				}
			}
			ntracks[i] = max(ntracks[i], bd.track+1)
		}
		gaps[i] = bs
	}
	hasExt := len(cols) > 0 && len(cols[0]) > 0 && cols[0][0].ext
	x := 0.0
	for i, col := range cols {
		width := 0.0
		for _, c := range col {
			width = math.Max(width, c.w)
		}
		for _, c := range col {
			c.x = x + (width-c.w)/2
			if c.port == portIn {
				c.x = 0
			}
		}
		x += width
		if i == len(cols)-1 {
			break
		}
		n := float64(ntracks[i])
		gap := math.Max(colGap, (n+1)*trackGap)
		first := x + (gap-(n-1)*trackGap)/2
		if hasExt && i == 0 {
			// The sources outside the cloud keep their tracks clear of its frame.
			gap = extGap + n*trackGap
			first = x + trackGap
		}
		for _, bd := range gaps[i] {
			tx := first + float64(bd.track)*trackGap
			for _, s := range bd.pieces {
				s.pts = []point{{0, s.from.y + s.fromOff}, {tx, s.from.y + s.fromOff}, {tx, s.to.y + s.toOff}, {0, s.to.y + s.toOff}}
			}
			// A dot where three stretches meet on the track, so a branch is
			// told from a crossing.
			type join struct {
				right bool
				y     int
			}
			joins := map[join]bool{}
			for _, s := range bd.pieces {
				joins[join{false, int(math.Round(s.from.y + s.fromOff))}] = true
				joins[join{true, int(math.Round(s.to.y + s.toOff))}] = true
			}
			meet := map[int]int{}
			for j := range joins {
				meet[j.y]++
			}
			ys := make([]int, 0, len(meet))
			for y := range meet {
				ys = append(ys, y)
			}
			sort.Ints(ys)
			for _, y := range ys {
				n := meet[y]
				if float64(y) > bd.lo+0.5 {
					n++
				}
				if float64(y) < bd.hi-0.5 {
					n++
				}
				if n >= 3 {
					l.dots = append(l.dots, point{tx, float64(y)})
				}
			}
		}
		x += gap
	}
	l.w = x
	for _, s := range l.segs {
		if s.back {
			continue
		}
		if s.pts == nil {
			y := s.from.y + s.fromOff
			s.pts = []point{{0, y}, {0, y}}
		}
		s.pts[0].x = outX(s.from)
		s.pts[len(s.pts)-1].x = inX(s.to)
	}
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
		for _, e := range g.edges {
			if e.from.ext {
				continue
			}
			for _, p := range e.path {
				include(p.x, p.y, p.x, p.y)
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
	for _, e := range g.edges {
		for _, p := range e.path {
			include(p.x, p.y, p.x, p.y)
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
		for i := range e.path {
			e.path[i].x += dx
			e.path[i].y += dy
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
	for i := range g.dots {
		g.dots[i].x += dx
		g.dots[i].y += dy
	}
	g.w = x1 + dx + marginX
	g.h = y1 + dy + marginY
}
