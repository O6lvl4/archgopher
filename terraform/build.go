package terraform

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/O6lvl4/arch-scouter/scout"
)

// Rules tell the builder how a provider's resources form a load graph.
type Rules struct {
	// NodeTypes become nodes. Types without a scouter still become nodes and pass load through.
	NodeTypes map[string]bool
	// Links are helper resources that connect two nodes (an API integration, an event source mapping).
	Links []Link
	// Aliases are helper resources that stand for a node (a Lambda alias, an API stage).
	Aliases map[string]string // type -> attribute path pointing at the node
	// Mentioned types are outermost front doors: nothing inside the system calls
	// them, so references to them (callback URLs, links in emails, CORS origins)
	// are mentions, not edges.
	Mentioned map[string]bool
	// FrontDoors get a shared "users" entry when nothing inside the graph calls them.
	FrontDoors map[string]bool
	// FrontDoorAliases are helper resources that expose a node to users (a Lambda function URL).
	FrontDoorAliases map[string]string
	// Schedules maps a node type to the attribute holding its schedule expression.
	Schedules map[string]string
	// RoleAttrs are attribute paths through which a node acts as an IAM role.
	RoleAttrs []string
	// IgnoreRefs are attribute path prefixes whose references are not calls (roles, keys, DLQs).
	IgnoreRefs []string
	// RoleLinks are how policies attach to roles.
	RoleLinks []RoleLink
	// Kinds maps IAM actions on a target type to kinds of work.
	// It returns nil for no edge and [""] for the target's default kind.
	Kinds func(targetType string, actions []string) []string
	// Scouters supply the fields to copy from Terraform and the assumptions to ask for.
	Scouters scout.Registry
}

// Link connects the node(s) referenced by From to the node(s) referenced by To.
type Link struct {
	Type string
	From string
	To   []string
}

// RoleLink says a resource of Type attaches policies to the role referenced by Role.
// Policy, when set, points at a separate policy resource holding the statements.
type RoleLink struct {
	Type   string
	Role   string
	Policy string
}

// UsersID is the ID of the entry created for front doors.
const UsersID = "users"

// Build turns evaluated resources into a declaration.
func Build(ev *Evaluated, rules Rules, name string) (scout.Spec, []string) {
	b := &builder{ev: ev, rules: rules, byAddr: map[string]*Resource{}, ids: map[string]string{}, warnings: append([]string(nil), ev.Warnings...)}
	for _, r := range ev.Resources {
		b.byAddr[r.Address] = r
	}
	spec := scout.Spec{Name: name, Region: ev.Region}
	for _, r := range ev.Resources {
		if rules.NodeTypes[r.Type] {
			spec.Nodes = append(spec.Nodes, b.node(r))
		}
	}
	var off []string
	for _, r := range ev.Removed {
		if rules.NodeTypes[r.Type] {
			off = append(off, r.Address)
		}
	}
	if len(off) > 0 {
		b.warnings = append(b.warnings, fmt.Sprintf("off with the current variables (count or for_each is 0), pass --var to include: %s", strings.Join(off, ", ")))
	}
	edges := b.edges()
	spec.Nodes, edges = b.frontDoors(spec.Nodes, edges)
	spec.Edges = b.breakCycles(spec.Nodes, edges)
	return spec, b.warnings
}

type builder struct {
	ev       *Evaluated
	rules    Rules
	byAddr   map[string]*Resource
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
	return ok && b.rules.NodeTypes[r.Type]
}

func (b *builder) node(r *Resource) scout.Node {
	n := scout.Node{ID: b.id(r), Type: r.Type, Address: r.Address, Attributes: map[string]any{}}
	if s, ok := b.rules.Scouters[r.Type]; ok {
		for _, f := range s.Attributes() {
			if v := lookupPath(r.Attrs, f.TerraformPath()); v != nil {
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
		expr, _ := r.Attrs[attr].(string)
		if load, err := ScheduleLoad(expr); err == nil {
			n.Load = &load
			n.Note = join(n.Note, "Load from "+expr+".")
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
func (b *builder) id(r *Resource) string {
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
		if !b.isNode(r.Address) {
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
		for _, st := range b.nodeStatements(r) {
			for _, ref := range st.Targets {
				for _, to := range b.targets(ref) {
					for _, kind := range b.rules.Kinds(b.byAddr[to].Type, st.Actions) {
						add(r.Address, to, kind)
					}
				}
			}
		}
	}
	for _, l := range b.rules.Links {
		for _, r := range b.ev.Resources {
			if r.Type != l.Type {
				continue
			}
			froms := b.targetsAt(r, l.From)
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
	for _, p := range append(append([]string(nil), b.rules.IgnoreRefs...), b.rules.RoleAttrs...) {
		if path == p || strings.HasPrefix(path, p+".") {
			return true
		}
	}
	return false
}

// targets resolves a reference to node addresses, looking through aliases.
func (b *builder) targets(ref string) []string {
	if b.isNode(ref) {
		return []string{ref}
	}
	r, ok := b.byAddr[ref]
	if !ok {
		return nil
	}
	if path, ok := b.rules.Aliases[r.Type]; ok {
		return b.targetsAt(r, path)
	}
	return nil
}

func (b *builder) targetsAt(r *Resource, path string) []string {
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

// nodeStatements gathers the IAM statements of every role the node acts as.
func (b *builder) nodeStatements(n *Resource) []Statement {
	var roles []string
	for _, attr := range b.rules.RoleAttrs {
		for p, refs := range n.Refs {
			if p == attr {
				roles = append(roles, refs...)
			}
		}
	}
	var out []Statement
	for _, role := range roles {
		if rr, ok := b.byAddr[role]; ok {
			out = append(out, b.expand(rr.Statements)...)
		}
		for _, r := range b.ev.Resources {
			for _, rl := range b.rules.RoleLinks {
				if r.Type != rl.Type || !contains(r.Refs[rl.Role], role) {
					continue
				}
				if rl.Policy == "" {
					out = append(out, b.expand(r.Statements)...)
					continue
				}
				for _, pol := range r.Refs[rl.Policy] {
					if pr, ok := b.byAddr[pol]; ok {
						out = append(out, b.expand(pr.Statements)...)
					}
				}
			}
		}
	}
	return out
}

func (b *builder) expand(sts []Statement) []Statement {
	var out []Statement
	for _, st := range sts {
		for _, d := range st.Docs {
			if dr, ok := b.byAddr[d]; ok {
				out = append(out, b.expand(dr.Statements)...)
			}
		}
		if len(st.Targets) > 0 {
			out = append(out, st)
		}
	}
	return out
}

func (b *builder) frontDoors(nodes []scout.Node, edges []edgeKey) ([]scout.Node, []edgeKey) {
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
	entry := scout.Node{ID: UsersID, Type: scout.EntryType, Note: "Users reaching the front doors. Set load: monthly volume and peak per second."}
	nodes = append([]scout.Node{entry}, nodes...)
	for _, addr := range sortedKeys(doors) {
		edges = append(edges, edgeKey{from: UsersID, to: addr})
	}
	return nodes, edges
}

// breakCycles drops edges that close a cycle (a callback URL, mutual references)
// and reports them; the engine refuses cyclic graphs.
func (b *builder) breakCycles(nodes []scout.Node, edges []edgeKey) []scout.Edge {
	adj := map[string][]string{}
	var out []scout.Edge
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
		out = append(out, scout.Edge{From: from, To: to, Kind: e.kind})
	}
	return out
}

// lookupPath reads "a.b" from nested maps, taking the first element of block lists.
func lookupPath(m map[string]any, path string) any {
	var cur any = m
	for _, part := range strings.Split(path, ".") {
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
		cur = obj[part]
	}
	if list, ok := cur.([]any); ok && len(list) > 0 {
		if _, isBlock := list[0].(map[string]any); isBlock {
			return nil
		}
	}
	return cur
}

func sortedPaths(m map[string][]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
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
