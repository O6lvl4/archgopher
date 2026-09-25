// Package infer turns evaluated Terraform resources into a declaration: which
// resources are nodes, which references are calls, where users enter, and how
// schedules become load. It knows no provider: the provider supplies Rules,
// including edge sources for knowledge like IAM.
package infer

import (
	"fmt"
	"github.com/O6lvl4/archgopher/field"
	"path/filepath"
	"sort"
	"strings"

	"github.com/O6lvl4/archgopher/model"
	"github.com/O6lvl4/archgopher/scouter"
	"github.com/O6lvl4/archgopher/terraform/eval"
	"github.com/O6lvl4/archgopher/traffic"
)

// Rules tell the builder how a provider's resources form a load graph.
// Providers build them in parts (one per service) and Combine them.
type Rules struct {
	// NodeTypes become nodes. Types without a scouter still become nodes and pass load through.
	NodeTypes map[string]bool
	// Links are helper resources that connect two nodes (an API integration, an event source mapping).
	Links []Link
	// Aliases are helper resources that stand for a node (a Lambda alias, an API stage).
	Aliases map[string]string // type -> attribute path pointing at the node
	// Forward maps a node type to the attribute pointing at a node that every
	// call to it also reaches: a Cosmos DB container is called through its
	// account, which bills the request units.
	Forward map[string]string
	// Mentioned types are outermost front doors: nothing inside the system calls
	// them, so references to them (callback URLs, links in emails, CORS origins)
	// are mentions, not edges.
	Mentioned map[string]bool
	// Passive types watch or describe others (alarms): their references are never calls.
	Passive map[string]bool
	// FrontDoors get a shared "users" entry when nothing inside the graph calls them.
	FrontDoors map[string]bool
	// FrontDoorAliases are helper resources that expose a node to users (a Lambda function URL).
	FrontDoorAliases map[string]string
	// Schedules maps a node type to the attribute holding its schedule
	// expression, which becomes the node's traffic.
	Schedules map[string]string
	// IgnoreRefs are attribute path prefixes whose references are not calls (roles, keys, DLQs).
	IgnoreRefs []string
	// Sources add edges from knowledge the builder does not have, such as IAM permissions.
	Sources []EdgeSource
	// Region reads the region from the evaluated configuration: provider blocks
	// (AWS) or resource locations (Azure). Empty when it cannot tell.
	Region func(ev *eval.Evaluated) string
	// Free types cost nothing by themselves (roles, rules, associations).
	// They never become nodes; Cover counts them apart from unpriced ones.
	Free map[string]bool
	// Scouters supply the fields to copy from Terraform and the assumptions to ask for.
	Scouters scouter.Registry
	// Boundaries are types that nodes sit in (a VPC), mapped to what people
	// call them. A node's boundary is found through the helpers it references:
	// security groups, subnets, subnet groups.
	Boundaries map[string]string
}

// Link connects the node(s) referenced by From to the node(s) referenced by To.
// When the link's own type is a node, it sits in between: From calls it, and
// its references (To among them) are its own calls.
type Link struct {
	Type string
	From string
	To   []string
}

// Hint is an edge an EdgeSource proposes. Kind "" means the target's default.
type Hint struct {
	From, To, Kind string
}

// EdgeSource proposes edges by looking at the evaluated resources.
type EdgeSource func(g *Graph) []Hint

// Graph is the read-only view an EdgeSource gets.
type Graph struct{ b *builder }

// Resources lists every evaluated resource.
func (g *Graph) Resources() []*eval.Resource { return g.b.ev.Resources }

// Resource finds a resource by address.
func (g *Graph) Resource(addr string) (*eval.Resource, bool) {
	r, ok := g.b.byAddr[addr]
	return r, ok
}

// IsNode reports whether an address became a node.
func (g *Graph) IsNode(addr string) bool { return g.b.isNode(addr) }

// Targets resolves a reference to node addresses, looking through aliases.
func (g *Graph) Targets(ref string) []string { return g.b.targets(ref) }

// Combine merges rule parts. Maps are unioned, lists appended, and the first
// non-nil function wins.
func Combine(parts ...Rules) Rules {
	out := Rules{
		NodeTypes: map[string]bool{}, Aliases: map[string]string{}, Forward: map[string]string{}, Mentioned: map[string]bool{}, Passive: map[string]bool{},
		FrontDoors: map[string]bool{}, FrontDoorAliases: map[string]string{}, Schedules: map[string]string{},
		Scouters: scouter.Registry{}, Boundaries: map[string]string{}, Free: map[string]bool{},
	}
	for _, p := range parts {
		copyMap(out.NodeTypes, p.NodeTypes)
		copyMap(out.Aliases, p.Aliases)
		copyMap(out.Forward, p.Forward)
		copyMap(out.Mentioned, p.Mentioned)
		copyMap(out.Passive, p.Passive)
		copyMap(out.FrontDoors, p.FrontDoors)
		copyMap(out.FrontDoorAliases, p.FrontDoorAliases)
		copyMap(out.Schedules, p.Schedules)
		copyMap(out.Scouters, p.Scouters)
		copyMap(out.Boundaries, p.Boundaries)
		copyMap(out.Free, p.Free)
		out.Links = append(out.Links, p.Links...)
		out.IgnoreRefs = append(out.IgnoreRefs, p.IgnoreRefs...)
		out.Sources = append(out.Sources, p.Sources...)
		out.Region = firstRegion(out.Region, p.Region)
	}
	return out
}

// firstRegion asks a, then b, for the region: a configuration of several
// providers takes the first one that can tell.
func firstRegion(a, b func(*eval.Evaluated) string) func(*eval.Evaluated) string {
	if a == nil {
		return b
	}
	if b == nil {
		return a
	}
	return func(ev *eval.Evaluated) string {
		if r := a(ev); r != "" {
			return r
		}
		return b(ev)
	}
}

func copyMap[K comparable, V any](dst, src map[K]V) {
	for k, v := range src {
		dst[k] = v
	}
}

// UsersID is the ID of the entry created for front doors.
const UsersID = "users"

// Build turns evaluated resources into a declaration.
func Build(ev *eval.Evaluated, rules Rules, name string) (model.Spec, []string) {
	b := &builder{ev: ev, rules: rules, byAddr: map[string]*eval.Resource{}, ids: map[string]string{}, warnings: append([]string(nil), ev.Warnings...)}
	for _, r := range ev.Resources {
		b.byAddr[r.Address] = r
	}
	spec := model.Spec{Name: name}
	if rules.Region != nil {
		spec.Region = rules.Region(ev)
	}
	for _, r := range ev.Resources {
		if b.owned(r) {
			spec.Nodes = append(spec.Nodes, b.node(r))
		}
	}
	var off []string
	for _, r := range ev.Removed {
		if b.owned(r) {
			off = append(off, r.Address)
		}
	}
	if len(off) > 0 {
		b.warnings = append(b.warnings, fmt.Sprintf("off with the current variables (count or for_each is 0), pass --var to include: %s", strings.Join(off, ", ")))
	}
	spec.Groups = b.groups(spec.Nodes)
	edges := b.edges()
	spec.Nodes, edges = b.frontDoors(spec.Nodes, edges)
	spec.Edges = b.breakCycles(spec.Nodes, edges)
	return spec, b.warnings
}

type builder struct {
	ev       *eval.Evaluated
	rules    Rules
	byAddr   map[string]*eval.Resource
	ids      map[string]string // address -> node id
	used     map[string]bool
	warnings []string
}

func (b *builder) mentioned(addr string) bool {
	r, ok := b.byAddr[addr]
	return ok && b.rules.Mentioned[r.Type]
}

func (b *builder) isNode(addr string) bool {
	r, ok := b.byAddr[addr]
	return ok && b.owned(r)
}

// owned reports whether r is a node of this configuration. A data source
// reads something managed elsewhere: its cost belongs there, and several
// modules reading the same thing would otherwise count it once each.
func (b *builder) owned(r *eval.Resource) bool {
	return b.rules.NodeTypes[r.Type] && r.Mode != "data"
}

func (b *builder) node(r *eval.Resource) model.Node {
	n := model.Node{ID: b.id(r), Type: r.Type, Address: r.Address, Attributes: map[string]any{}}
	if s, ok := b.rules.Scouters[r.Type]; ok {
		for _, f := range s.Attributes() {
			if v := readAttribute(r, f); v != nil {
				n.Attributes[f.Key] = v
			}
		}
		for _, f := range s.Assumptions() {
			if f.Required {
				if n.Assumptions == nil {
					n.Assumptions = map[string]any{}
				}
				n.Assumptions[f.Key] = nil
			}
		}
	} else {
		n.Note = "No scouter reads " + r.Type + " yet; load passes through."
	}
	if attr, ok := b.rules.Schedules[r.Type]; ok {
		// The schedule itself is the traffic, so the declaration keeps what
		// Terraform says and follows it when it changes.
		expr, _ := r.Attrs[attr].(string)
		if _, err := traffic.Parse(expr); err == nil {
			n.Traffic = &model.Traffic{Schedule: expr}
		} else {
			b.warnings = append(b.warnings, fmt.Sprintf("%s: %v", r.Address, err))
		}
	}
	if r.Instances > 1 {
		n.Note = join(n.Note, fmt.Sprintf("%d instances (count or for_each); one node carries their combined load.", r.Instances))
	} else if r.Instances < 0 {
		n.Note = join(n.Note, "Instance count is unknown before apply; one node carries the combined load.")
	}
	if len(n.Attributes) == 0 {
		n.Attributes = nil
	}
	return n
}

// id derives a short, stable id: the resource name, the module name for
// generic names like "this", and the type as a suffix on collision.
func (b *builder) id(r *eval.Resource) string {
	if b.used == nil {
		b.used = map[string]bool{UsersID: true}
	}
	base := r.Name
	if r.Module != "" {
		mod := r.Module[strings.LastIndex(r.Module, ".")+1:]
		switch r.Name {
		case "this", "main", "default", "self":
			base = mod
		default:
			base = mod + "." + r.Name
		}
	}
	id := base
	if b.used[id] {
		id = base + "-" + shortType(r.Type)
	}
	for i := 2; b.used[id]; i++ {
		id = fmt.Sprintf("%s-%d", base, i)
	}
	b.used[id] = true
	b.ids[r.Address] = id
	return id
}

type edgeKey struct{ from, to, kind string }

func (b *builder) edges() []edgeKey {
	var out []edgeKey
	seen := map[edgeKey]bool{}
	add := func(from, to, kind string) {
		if from == to || b.mentioned(to) {
			return
		}
		k := edgeKey{from, to, kind}
		if !seen[k] {
			seen[k] = true
			out = append(out, k)
		}
	}
	for _, r := range b.ev.Resources {
		if !b.isNode(r.Address) || b.rules.Passive[r.Type] {
			continue
		}
		for _, path := range sortedPaths(r.Refs) {
			if b.ignored(path) {
				continue
			}
			for _, ref := range r.Refs[path] {
				for _, to := range b.targets(ref) {
					add(r.Address, to, "")
				}
			}
		}
	}
	for _, src := range b.rules.Sources {
		for _, h := range src(&Graph{b: b}) {
			add(h.From, h.To, h.Kind)
		}
	}
	for _, l := range b.rules.Links {
		for _, r := range b.ev.Resources {
			if r.Type != l.Type {
				continue
			}
			froms := b.targetsAt(r, l.From)
			if b.isNode(r.Address) {
				// A helper with a cost of its own (a Pub/Sub subscription) is a
				// node on the path: from → it, and its own references carry on.
				for _, from := range froms {
					add(from, r.Address, "")
				}
				continue
			}
			for _, p := range l.To {
				for _, to := range b.targetsAt(r, p) {
					for _, from := range froms {
						add(from, to, "")
					}
				}
			}
		}
	}
	// A kindless edge is redundant when IAM already gave the same pair explicit kinds.
	kinded := map[[2]string]bool{}
	for _, e := range out {
		if e.kind != "" {
			kinded[[2]string{e.from, e.to}] = true
		}
	}
	var kept []edgeKey
	for _, e := range out {
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

func (b *builder) frontDoors(nodes []model.Node, edges []edgeKey) ([]model.Node, []edgeKey) {
	incoming := map[string]bool{}
	for _, e := range edges {
		incoming[e.to] = true
	}
	doors := map[string]bool{}
	for _, n := range nodes {
		if b.rules.FrontDoors[n.Type] && !incoming[n.Address] {
			doors[n.Address] = true
		}
	}
	for _, r := range b.ev.Resources {
		if path, ok := b.rules.FrontDoorAliases[r.Type]; ok {
			for _, t := range b.targetsAt(r, path) {
				doors[t] = true
			}
		}
	}
	if len(doors) == 0 {
		return nodes, edges
	}
	entry := model.Node{ID: UsersID, Type: scouter.EntryType, Note: "Users reaching the front doors. Set load: monthly volume and peak per second."}
	nodes = append([]model.Node{entry}, nodes...)
	for _, addr := range sortedKeys(doors) {
		edges = append(edges, edgeKey{from: UsersID, to: addr})
	}
	return nodes, edges
}

// breakCycles drops edges that close a cycle (a callback URL, mutual references)
// and reports them; the engine refuses cyclic graphs.
func (b *builder) breakCycles(nodes []model.Node, edges []edgeKey) []model.Edge {
	adj := map[string][]string{}
	var out []model.Edge
	reaches := func(from, to string) bool {
		seen := map[string]bool{}
		stack := []string{from}
		for len(stack) > 0 {
			n := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if n == to {
				return true
			}
			if seen[n] {
				continue
			}
			seen[n] = true
			stack = append(stack, adj[n]...)
		}
		return false
	}
	idOf := func(addr string) string {
		if addr == UsersID {
			return UsersID
		}
		return b.ids[addr]
	}
	for _, e := range edges {
		from, to := idOf(e.from), idOf(e.to)
		if reaches(to, from) {
			b.warnings = append(b.warnings, fmt.Sprintf("dropped edge %s -> %s: it closes a cycle", from, to))
			continue
		}
		adj[from] = append(adj[from], to)
		out = append(out, model.Edge{From: from, To: to, Kind: e.kind})
	}
	return oneEdgePerPair(out)
}

// oneEdgePerPair folds edges between the same two nodes into one whose
// operations are their kinds: a role that reads and writes a table is one
// edge that does both.
func oneEdgePerPair(edges []model.Edge) []model.Edge {
	var out []model.Edge
	at := map[[2]string]int{}
	for _, e := range edges {
		k := [2]string{e.From, e.To}
		i, seen := at[k]
		if !seen {
			at[k] = len(out)
			out = append(out, e)
			continue
		}
		first := &out[i]
		if len(first.Ops) == 0 {
			first.Ops = []model.Op{{Kind: first.Kind}}
			first.Kind = ""
		}
		first.Ops = append(first.Ops, model.Op{Kind: e.Kind})
	}
	return out
}

// readAttribute reads a field's value from a resource, or nil when it is not
// known before apply.
//
//   - A path ending in ".#" counts what it names across every block
//     ("criteria.dimension.values.#" is every value of every dimension of every
//     criterion); nothing written leaves the field to its default. A list whose
//     elements are known only after apply counts the resources it references.
//   - A boolean that points at a block or at a non-boolean value reads whether
//     it is written, even when the value (an id) is known only after apply.
func readAttribute(r *eval.Resource, f field.Field) any {
	path := f.TerraformPath()
	if base, ok := strings.CutSuffix(path, ".#"); ok {
		if n := max(countPath(r.Attrs, base), len(r.Refs[base])); n > 0 {
			return float64(n)
		}
		return nil
	}
	v := lookupPath(r.Attrs, path)
	if f.Type == field.Number && v == nil {
		// A number that points at a block reads how many are written
		// (replicas, rules).
		if n := blocks(r.Attrs, path); n > 0 {
			return float64(n)
		}
		return nil
	}
	if f.Type != field.Flag {
		return v
	}
	switch x := v.(type) {
	case bool:
		return x
	case nil:
		if blocks(r.Attrs, path) > 0 || len(r.Refs[path]) > 0 {
			return true
		}
		return nil
	case string:
		if x == "true" || x == "false" {
			return x == "true"
		}
		if x == "" {
			return nil // written empty: as good as not written
		}
	}
	return true
}

// countPath counts the elements "a.b" holds, fanning out over every block
// instance on the way: list elements and blocks count one each, a single
// value counts one, nothing written counts zero.
func countPath(m map[string]any, path string) int {
	cur := []any{m}
	for _, part := range strings.Split(path, ".") {
		var next []any
		for _, c := range cur {
			if list, ok := c.([]any); ok {
				for _, e := range list {
					if obj, ok := e.(map[string]any); ok && obj[part] != nil {
						next = append(next, obj[part])
					}
				}
				continue
			}
			if obj, ok := c.(map[string]any); ok && obj[part] != nil {
				next = append(next, obj[part])
			}
		}
		cur = next
	}
	n := 0
	for _, c := range cur {
		if list, ok := c.([]any); ok {
			n += len(list)
		} else {
			n++
		}
	}
	return n
}

// lookupPath reads "a.b" from nested maps, taking the first element of block lists.
// A map key may itself hold dots (annotations such as
// "autoscaling.knative.dev/minScale"): when a part is not a key, the shortest
// run of the following parts that is one is taken.
func lookupPath(m map[string]any, path string) any {
	var cur any = m
	parts := strings.Split(path, ".")
	for i := 0; i < len(parts); {
		if list, ok := cur.([]any); ok {
			if len(list) == 0 {
				return nil
			}
			cur = list[0]
		}
		obj, ok := cur.(map[string]any)
		if !ok {
			return nil
		}
		var next any
		found := false
		for j := i + 1; j <= len(parts); j++ {
			if v, ok := obj[strings.Join(parts[i:j], ".")]; ok {
				next, found, i = v, true, j
				break
			}
		}
		if !found {
			return nil
		}
		cur = next
	}
	if list, ok := cur.([]any); ok && len(list) > 0 {
		if _, isBlock := list[0].(map[string]any); isBlock {
			return nil
		}
	}
	return cur
}

// blocks counts the blocks written at "a.b", empty ones included; the parent
// path is walked through the first instance of each block.
func blocks(m map[string]any, path string) int {
	parts := strings.Split(path, ".")
	parent, last := m, parts[len(parts)-1]
	if len(parts) > 1 {
		p, ok := lookupBlock(m, strings.Join(parts[:len(parts)-1], "."))
		if !ok {
			return 0
		}
		parent = p
	}
	switch v := parent[last].(type) {
	case []any:
		n := 0
		for _, b := range v {
			if _, ok := b.(map[string]any); ok {
				n++
			}
		}
		return n
	case map[string]any:
		return 1
	}
	return 0
}

// lookupBlock walks "a.b" through blocks and returns the first instance.
func lookupBlock(m map[string]any, path string) (map[string]any, bool) {
	cur := m
	for _, part := range strings.Split(path, ".") {
		switch v := cur[part].(type) {
		case []any:
			if len(v) == 0 {
				return nil, false
			}
			next, ok := v[0].(map[string]any)
			if !ok {
				return nil, false
			}
			cur = next
		case map[string]any:
			cur = v
		default:
			return nil, false
		}
	}
	return cur, true
}

func sortedPaths(m map[string][]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func join(a, b string) string {
	if a == "" {
		return b
	}
	return a + " " + b
}

// DefaultName names a declaration after the Terraform directory.
func DefaultName(dir string) string {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return dir
	}
	return filepath.Base(abs)
}

// shortType is the service part of a type: aws_sqs_queue -> sqs.
func shortType(t string) string {
	parts := strings.Split(t, "_")
	if len(parts) > 1 {
		return parts[1]
	}
	return t
}

func sortedKeys(set map[string]bool) []string {
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
