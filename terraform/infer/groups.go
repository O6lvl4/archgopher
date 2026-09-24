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
		found := b.boundaries(r)
		if len(found) != 1 {
			if len(found) > 1 {
				b.warnings = append(b.warnings, fmt.Sprintf("%s reaches %d boundaries; left out of all of them", r.Address, len(found)))
			}
			continue
		}
		var key string
		var br *eval.Resource
		for k, v := range found {
			key, br = k, v
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
	found := map[string]*eval.Resource{}
	seen := map[string]bool{r.Address: true}
	frontier := []*eval.Resource{r}
	for depth := 0; depth < boundaryDepth && len(frontier) > 0; depth++ {
		var next []*eval.Resource
		for _, cur := range frontier {
			for _, path := range sortedPaths(cur.Refs) {
				if cur == r && !placement.MatchString(path) {
					continue
				}
				for _, ref := range cur.Refs[path] {
					t, ok := b.byAddr[ref]
					if !ok || seen[ref] {
						continue
					}
					seen[ref] = true
					if _, ok := b.rules.Boundaries[t.Type]; ok {
						found[boundaryKey(t)] = t
						continue
					}
					if !b.isNode(ref) {
						next = append(next, t)
					}
				}
			}
		}
		frontier = next
	}
	return found
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
	if tags, ok := r.Attrs["tags"].(map[string]any); ok {
		if s, ok := tags["Name"].(string); ok && s != "" {
			return s
		}
	}
	if filters, ok := r.Attrs["filter"].([]any); ok {
		for _, f := range filters {
			m, _ := f.(map[string]any)
			if name, _ := m["name"].(string); name == "tag:Name" || name == "vpc-id" {
				if vs, ok := m["values"].([]any); ok && len(vs) > 0 {
					if s, ok := vs[0].(string); ok && s != "" {
						return s
					}
				}
			}
		}
	}
	for _, k := range []string{"id", "name"} {
		if s, ok := r.Attrs[k].(string); ok && s != "" {
			return s
		}
	}
	return r.Name
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
