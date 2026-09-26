package infer

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/O6lvl4/archgopher/model"
	"github.com/O6lvl4/archgopher/terraform/eval"
)

// boundaryDepth is how many helpers a node's boundary may sit behind: a
// function names a security group, which names the VPC; a database names a
// subnet group, which names subnets, which name the VPC.
const boundaryDepth = 4

// groups places each node in the boundary it sits in and lists the
// boundaries. A node whose helpers lead to two boundaries, or none, stays out.
func (b *builder) groups(nodes []model.Node) []model.Group {
	var out []model.Group
	ids := map[string]string{} // boundary key -> group id
	used := map[string]bool{}
	for i := range nodes {
		r, ok := b.byAddr[nodes[i].Address]
		if !ok {
			continue
		}
		key, br, ok := b.boundaryOf(r)
		if !ok {
			continue
		}
		id, ok := ids[key]
		if !ok {
			label := boundaryLabel(br)
			id = groupID(label, used)
			ids[key] = id
			out = append(out, model.Group{ID: id, Kind: b.rules.Boundaries[br.Type], Label: label, Type: br.Type})
		}
		nodes[i].Group = id
	}
	return out
}

// boundaryOf is the one boundary r sits in. A node whose helpers lead to
// several is warned about, and like one that leads to none sits in none.
func (b *builder) boundaryOf(r *eval.Resource) (string, *eval.Resource, bool) {
	found := b.boundaries(r)
	if len(found) > 1 {
		b.warnings = append(b.warnings, fmt.Sprintf("%s reaches %d boundaries; left out of all of them", r.Address, len(found)))
	}
	if len(found) != 1 {
		return "", nil, false
	}
	for key, br := range found {
		return key, br, true
	}
	return "", nil, false
}

// placement matches the attributes that say where a resource runs
// (vpc_config, subnet_ids, vpc_security_group_ids, network_configuration).
// Other attributes can name subnets without the resource being in them: a
// state machine's definition names the subnets of the tasks it starts.
var placement = regexp.MustCompile(`vpc|subnet|security_group|network`)

// boundaries walks the references of r through resources that are not nodes
// and collects the boundaries it reaches. From the node itself only placement
// attributes are followed. Other nodes are not walked through: a function that
// calls a database is not inside the database's network.
func (b *builder) boundaries(r *eval.Resource) map[string]*eval.Resource {
	w := &boundaryWalk{b: b, found: map[string]*eval.Resource{}, seen: map[string]bool{r.Address: true}}
	frontier := []*eval.Resource{r}
	for depth := 0; depth < boundaryDepth && len(frontier) > 0; depth++ {
		var next []*eval.Resource
		for _, cur := range frontier {
			next = append(next, w.step(cur, cur == r)...)
		}
		frontier = next
	}
	return w.found
}

// boundaryWalk is the state of one boundaries search.
type boundaryWalk struct {
	b     *builder
	found map[string]*eval.Resource // boundary key -> boundary
	seen  map[string]bool
}

// step follows the references of cur, only its placement attributes when
// cur is the node itself, and returns the helpers to walk through next.
func (w *boundaryWalk) step(cur *eval.Resource, placementOnly bool) []*eval.Resource {
	var next []*eval.Resource
	for _, path := range sortedPaths(cur.Refs) {
		if placementOnly && !placement.MatchString(path) {
			continue
		}
		for _, ref := range cur.Refs[path] {
			if t, ok := w.visit(ref); ok {
				next = append(next, t)
			}
		}
	}
	return next
}

// visit looks at a referenced resource once: a boundary is recorded, and a
// helper that is not a node is returned to walk through.
func (w *boundaryWalk) visit(ref string) (*eval.Resource, bool) {
	t, ok := w.b.byAddr[ref]
	if !ok || w.seen[ref] {
		return nil, false
	}
	w.seen[ref] = true
	if _, ok := w.b.rules.Boundaries[t.Type]; ok {
		w.found[boundaryKey(t)] = t
		return nil, false
	}
	return t, !w.b.isNode(ref)
}

// boundaryKey identifies a boundary. A managed one is its address; a data
// source is what it looks up, so modules that each look up the same VPC agree.
func boundaryKey(r *eval.Resource) string {
	if r.Mode != "data" {
		return r.Address
	}
	attrs, _ := json.Marshal(r.Attrs) // map keys are sorted
	return r.Type + " " + string(attrs)
}

// boundaryLabel names a boundary the way its owner does: its Name tag, the
// Name tag it is looked up by, its id or name, else the block's name.
func boundaryLabel(r *eval.Resource) string {
	id, _ := r.Attrs["id"].(string)
	name, _ := r.Attrs["name"].(string)
	for _, s := range []string{nameTag(r.Attrs), filterName(r.Attrs), id, name} {
		if s != "" {
			return s
		}
	}
	return r.Name
}

// nameTag is the Name tag in the attributes, or "".
func nameTag(attrs map[string]any) string {
	tags, _ := attrs["tags"].(map[string]any)
	s, _ := tags["Name"].(string)
	return s
}

// filterName is the first value of the first filter by Name tag or VPC id
// that has one: what a data source looks its boundary up by.
func filterName(attrs map[string]any) string {
	filters, _ := attrs["filter"].([]any)
	for _, f := range filters {
		m, _ := f.(map[string]any)
		if s := filterValue(m); s != "" {
			return s
		}
	}
	return ""
}

func filterValue(filter map[string]any) string {
	if name, _ := filter["name"].(string); name != "tag:Name" && name != "vpc-id" {
		return ""
	}
	vs, _ := filter["values"].([]any)
	if len(vs) == 0 {
		return ""
	}
	s, _ := vs[0].(string)
	return s
}

var notSlug = regexp.MustCompile(`[^a-z0-9]+`)

// groupID derives a short id from the label, unique among the groups.
func groupID(label string, used map[string]bool) string {
	base := strings.Trim(notSlug.ReplaceAllString(strings.ToLower(label), "-"), "-")
	if base == "" {
		base = "group"
	}
	id := base
	for i := 2; used[id]; i++ {
		id = fmt.Sprintf("%s-%d", base, i)
	}
	used[id] = true
	return id
}
