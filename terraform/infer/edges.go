package infer

import (
	"sort"
	"strings"

	"github.com/O6lvl4/archgopher/terraform/eval"
)

type edgeKey struct{ from, to, kind string }

// edgeSet collects edges in the order they are found, each once. Self
// edges and edges to mentioned types are not calls and are left out.
type edgeSet struct {
	b    *builder
	out  []edgeKey
	seen map[edgeKey]bool
}

func (s *edgeSet) add(from, to, kind string) {
	if from == to || s.b.mentioned(to) {
		return
	}
	k := edgeKey{from, to, kind}
	if !s.seen[k] {
		s.seen[k] = true
		s.out = append(s.out, k)
	}
}

// addAll adds a kindless edge from every one of froms to every one of tos.
func (s *edgeSet) addAll(froms, tos []string) {
	for _, to := range tos {
		for _, from := range froms {
			s.add(from, to, "")
		}
	}
}

// edges gathers the calls between nodes: references, then what edge sources
// know, then links.
func (b *builder) edges() []edgeKey {
	s := &edgeSet{b: b, seen: map[edgeKey]bool{}}
	for _, r := range b.ev.Resources {
		if b.isNode(r.Address) && !b.rules.Passive[r.Type] {
			b.refEdges(s, r)
		}
	}
	for _, src := range b.rules.Sources {
		for _, h := range src(&Graph{b: b}) {
			s.add(h.From, h.To, h.Kind)
		}
	}
	for _, l := range b.rules.Links {
		for _, r := range b.ev.Resources {
			if r.Type == l.Type {
				b.linkEdges(s, l, r)
			}
		}
	}
	return dropKindless(s.out)
}

// refEdges adds a call from node r to every node its attributes reference,
// except through the paths the rules ignore.
func (b *builder) refEdges(s *edgeSet, r *eval.Resource) {
	from := []string{r.Address}
	for _, path := range sortedPaths(r.Refs) {
		if b.ignored(path) {
			continue
		}
		for _, ref := range r.Refs[path] {
			s.addAll(from, b.targets(ref))
		}
	}
}

// linkEdges adds the calls one link resource makes.
func (b *builder) linkEdges(s *edgeSet, l Link, r *eval.Resource) {
	froms := b.linkEnds(r, l.From)
	if b.isNode(r.Address) && l.From != Self {
		// A helper with a cost of its own (a Pub/Sub subscription) is a
		// node on the path: from → it, and its own references carry on.
		s.addAll(froms, []string{r.Address})
		return
	}
	for _, p := range l.To {
		s.addAll(froms, b.linkEnds(r, p))
	}
}

// dropKindless drops a kindless edge when IAM already gave the same pair
// explicit kinds: it is redundant.
func dropKindless(edges []edgeKey) []edgeKey {
	kinded := map[[2]string]bool{}
	for _, e := range edges {
		if e.kind != "" {
			kinded[[2]string{e.from, e.to}] = true
		}
	}
	var kept []edgeKey
	for _, e := range edges {
		if e.kind == "" && kinded[[2]string{e.from, e.to}] {
			continue
		}
		kept = append(kept, e)
	}
	return kept
}

func (b *builder) ignored(path string) bool {
	for _, p := range b.rules.IgnoreRefs {
		if path == p || strings.HasPrefix(path, p+".") {
			return true
		}
	}
	return false
}

// targets resolves a reference to node addresses, looking through aliases
// and on to the nodes a node forwards its calls to.
func (b *builder) targets(ref string) []string {
	r, ok := b.byAddr[ref]
	if !ok {
		return nil
	}
	if b.isNode(ref) {
		if path, ok := b.rules.Forward[r.Type]; ok {
			return append([]string{ref}, b.targetsAt(r, path)...)
		}
		return []string{ref}
	}
	// A data source of a forwarding type is not a node, but a call to it
	// still reaches the node it names.
	if path, ok := b.rules.Forward[r.Type]; ok {
		return b.targetsAt(r, path)
	}
	if path, ok := b.rules.Aliases[r.Type]; ok {
		return b.targetsAt(r, path)
	}
	return nil
}

// linkEnds resolves one end of a link: the nodes a path references, or the
// link resource itself for Self.
func (b *builder) linkEnds(r *eval.Resource, path string) []string {
	if path == Self {
		if b.isNode(r.Address) {
			return []string{r.Address}
		}
		return nil
	}
	return b.targetsAt(r, path)
}

func (b *builder) targetsAt(r *eval.Resource, path string) []string {
	var out []string
	for p, refs := range r.Refs {
		if p != path && !strings.HasPrefix(p, path+".") {
			continue
		}
		for _, ref := range refs {
			out = append(out, b.targets(ref)...)
		}
	}
	sort.Strings(out)
	return out
}
