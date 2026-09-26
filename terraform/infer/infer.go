// Package infer turns evaluated Terraform resources into a declaration: which
// resources are nodes, which references are calls, where users enter, and how
// schedules become load. It knows no provider: the provider supplies Rules,
// including edge sources for knowledge like IAM.
package infer

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/O6lvl4/archgopher/model"
	"github.com/O6lvl4/archgopher/scouter"
	"github.com/O6lvl4/archgopher/terraform/eval"
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
// its references (To among them) are its own calls. A path of Self names the
// link resource itself, so a resource between two others (a pipe, an SNS
// subscription) can say it receives from its source (To: [self]) and calls
// its target (From: self).
type Link struct {
	Type string
	From string
	To   []string
}

// Self in Link.To stands for the link resource itself when it is a node:
// an endpoint group that names its listener is called through that listener.
const Self = "self"

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
	b.warnOff(ev.Removed)
	spec.Groups = b.groups(spec.Nodes)
	edges := b.edges()
	spec.Nodes, edges = b.frontDoors(spec.Nodes, edges)
	spec.Edges = b.breakCycles(spec.Nodes, edges)
	spec.Nodes, spec.Edges = b.splitVariants(spec.Nodes, spec.Edges)
	return spec, b.warnings
}

// warnOff names the nodes that count or for_each switch off, so people know
// which variables would bring them in.
func (b *builder) warnOff(removed []*eval.Resource) {
	var off []string
	for _, r := range removed {
		if b.owned(r) {
			off = append(off, r.Address)
		}
	}
	if len(off) > 0 {
		b.warnings = append(b.warnings, fmt.Sprintf("off with the current variables (count or for_each is 0), pass --var to include: %s", strings.Join(off, ", ")))
	}
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

func sortedPaths(m map[string][]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// DefaultName names a declaration after the Terraform directory.
func DefaultName(dir string) string {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return dir
	}
	return filepath.Base(abs)
}

func sortedKeys(set map[string]bool) []string {
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
