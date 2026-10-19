package infer

import (
	"strings"

	"github.com/O6lvl4/archgopher/model"
)

// A configuration calls things it does not own: a foundation model, a
// guardrail or a bucket in another account, a queue another team made. Its
// code says so where it grants itself permission on them, and nowhere else.
// A NodeSource reads such a place and proposes a Stub for each: a node with
// no address, its numbers left for gaps to ask, so the call is not a silent
// zero. The builder adds the stubs after the configuration's own nodes and
// gives each caller an edge to them.

// Stub is a node a NodeSource proposes for something outside the configuration.
type Stub struct {
	// Key identifies what the stub stands for (an ARN); one node per key.
	// Stubs whose choice assumption resolves to the same option are one node.
	Key string
	// Type is the node type; it must have a scouter.
	Type string
	// Name is the id's tail when no choice option names it better, taken
	// from the ARN's last segment.
	Name string
	// Match is text a choice assumption's option may appear in (the ARN):
	// a model id inside a foundation-model ARN selects the model.
	Match string
	Note  string
	// From is the address of the node that calls it, and Kind the work.
	From, Kind string
}

// NodeSource proposes nodes for what the configuration calls but does not own.
type NodeSource func(g *Graph) []Stub

// stubs adds the nodes the sources propose, one per key, and remembers the
// edges to them for edges().
func (b *builder) stubs() []model.Node {
	var nodes []model.Node
	at := map[string]int{}
	for _, src := range b.rules.NodeSources {
		for _, s := range src(&Graph{b: b}) {
			n, key := b.stubNode(s)
			if _, ok := at[key]; !ok {
				at[key] = len(nodes)
				nodes = append(nodes, n)
				b.stubIDs[key] = n.ID
			}
			b.stubEdges = append(b.stubEdges, edgeKey{from: s.From, to: key, kind: s.Kind})
		}
	}
	return nodes
}

// stubNode builds the node of a stub and the key it is one node under: a
// choice option found in the match text (a model id in an ARN) both fills
// the assumption and merges stubs for the same option under one node.
func (b *builder) stubNode(s Stub) (model.Node, string) {
	n := model.Node{Type: s.Type, Note: s.Note}
	tail, key := idTail(s.Name), s.Key
	if sc, ok := b.rules.Scouters[s.Type]; ok {
		for _, f := range sc.Assumptions() {
			if f.Required && f.Default == nil {
				if n.Assumptions == nil {
					n.Assumptions = map[string]any{}
				}
				n.Assumptions[f.Key] = nil
			}
			if opt := longestOption(f.Options, s.Match); opt != "" {
				if n.Assumptions == nil {
					n.Assumptions = map[string]any{}
				}
				n.Assumptions[f.Key] = opt
				tail, key = idTail(opt), s.Type+"="+opt
			}
		}
	}
	if id, ok := b.stubIDs[key]; ok {
		n.ID = id
		return n, key
	}
	n.ID = b.claim(shortType(s.Type) + "." + tail)
	return n, key
}

// longestOption is the longest of options found in text, or "".
func longestOption(options []string, text string) string {
	best := ""
	for _, o := range options {
		if len(o) > len(best) && strings.Contains(text, o) {
			best = o
		}
	}
	return best
}

// idTail makes a name safe as the tail of a node id.
func idTail(name string) string {
	if name == "" {
		return "any"
	}
	var b strings.Builder
	for _, r := range strings.ToLower(name) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-', r == '_', r == '.':
			b.WriteRune(r)
		default:
			b.WriteRune('-')
		}
	}
	return strings.Trim(b.String(), "-.")
}

// claim takes id, numbered when it is in use.
func (b *builder) claim(id string) string {
	if b.used == nil {
		b.used = map[string]bool{UsersID: true}
	}
	base := id
	for i := 2; b.used[id]; i++ {
		id = base + "-" + itoa(i)
	}
	b.used[id] = true
	return id
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var d []byte
	for ; i > 0; i /= 10 {
		d = append([]byte{byte('0' + i%10)}, d...)
	}
	return string(d)
}
