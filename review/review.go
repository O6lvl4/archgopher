// Package review reads a configuration against practices a provider knows:
// where the shape of the graph, a resource's settings or the resources that
// should stand beside it depart from the usual way of building it. A finding
// is not an error. A design may depart on purpose; the finding asks for the
// reason to be written down. Providers supply the rules; this package knows
// no provider.
package review

import (
	"fmt"
	"sort"
	"strings"

	"github.com/O6lvl4/archgopher/model"
)

// Resource is one configured resource as review sees it: the settings known
// before apply and the resources each attribute path references.
type Resource struct {
	Address string
	Type    string
	// Attrs holds the values known before apply. Nested blocks are lists of maps.
	Attrs map[string]any
	// Refs maps an attribute path ("dimensions.QueueName") to the addresses it references.
	Refs map[string][]string
	// Instances is the count or for_each size; 0 means one, negative unknown.
	Instances int
	// Unknown lists the top-level attributes that are written but not known
	// before apply. A rule never judges them.
	Unknown []string
	// Variants hold the settings of instances that differ from Attrs (for_each
	// over different values): a companion may name a resource in any of them.
	Variants []map[string]any
}

// Input is what review reads: the declaration's graph and, when it came
// from Terraform, the resources. A declaration file alone has no resources,
// and only the graph rules apply.
type Input struct {
	Spec      model.Spec
	Resources []Resource
}

// Rule is one practice. Exactly one of the shapes is set.
type Rule struct {
	ID string `json:"id"`
	// Pillar is the Well-Architected pillar, or the lens, the practice comes from.
	Pillar string `json:"pillar"`
	Title  string `json:"title"`
	// Hint says when departing is fine and what to do otherwise.
	Hint string `json:"hint"`
	// Source is where the practice is written.
	Source string `json:"source"`

	FanIn     *FanIn     `json:"fanIn,omitempty"`
	Setting   *Setting   `json:"setting,omitempty"`
	Companion *Companion `json:"companion,omitempty"`
	Ratio     *Ratio     `json:"ratio,omitempty"`
	Shared    *Shared    `json:"shared,omitempty"`
}

// By names resources of Type that reference another through Path (or a
// path under it).
type By struct {
	Type string `json:"type"`
	Path string `json:"path"`
}

// Scope narrows which resources of a type a rule judges.
type Scope struct {
	// Only: judge a resource only when one of these references it (a queue
	// that is some queue's dead-letter queue).
	Only []By `json:"only,omitempty"`
	// Except: skip a resource one of these references.
	Except []By `json:"except,omitempty"`
	// Via: judge a resource only when, through each path, it references a
	// resource of the given type (an event source mapping reading a queue).
	Via []By `json:"via,omitempty"`
	// UnlessAny: skip every resource when the configuration has one of these
	// types (a registry-wide scanning setting instead of one per repository).
	UnlessAny []string `json:"unlessAny,omitempty"`
}

// FanIn finds a node of type Node fed by more than Max resources of type
// From, counting a node's instances: one function consuming two queues.
type FanIn struct {
	Node string `json:"node"`
	From string `json:"from"`
	Max  int    `json:"max"`
}

// Setting checks one attribute of every resource of Type.
type Setting struct {
	Type  string `json:"type"`
	Scope Scope  `json:"scope"`
	Path  string `json:"path"`
	// Default is the provider's value when the attribute is not written.
	Default any `json:"default,omitempty"`
	// Exactly one check: the attribute is written or referenced, equals a
	// value, is at least a number, or is a list that contains a value.
	Set      bool     `json:"set,omitempty"`
	Equals   any      `json:"equals,omitempty"`
	OneOf    []any    `json:"oneOf,omitempty"`
	AtLeast  *float64 `json:"atLeast,omitempty"`
	Contains string   `json:"contains,omitempty"`
	// When limits the check to resources whose WhenPath is at least WhenAbove
	// (batch_size above 1); WhenDefault stands in when it is not written.
	WhenPath    string  `json:"whenPath,omitempty"`
	WhenAbove   float64 `json:"whenAbove,omitempty"`
	WhenDefault any     `json:"whenDefault,omitempty"`
	// Or: the practice is also met when one of these references the resource
	// (a separate redrive policy resource instead of the attribute).
	Or []By `json:"or,omitempty"`
}

// Companion asks every resource of Type for a resource of one of the By
// types that references it through By.Path, or names it there: the By
// resource's value at Path contains the resource's NameAttr.
type Companion struct {
	Type     string `json:"type"`
	Scope    Scope  `json:"scope"`
	By       []By   `json:"by"`
	NameAttr string `json:"nameAttr,omitempty"`
}

// Ratio compares two resources a link resource joins: Left at least Min
// times Right (a queue's visibility timeout against its consumer's timeout).
type Ratio struct {
	Type  string  `json:"type"`
	Scope Scope   `json:"scope"`
	Left  End     `json:"left"`
	Right End     `json:"right"`
	Min   float64 `json:"min"`
}

// End is one side of a Ratio: the resource the link references through Ref,
// and the number at Attr on it.
type End struct {
	Ref     string  `json:"ref"`
	Attr    string  `json:"attr"`
	Default float64 `json:"default"`
}

// Shared finds a resource of Type that more resources of type By reference
// through Path than it has instances: one role behind several functions.
type Shared struct {
	Type string `json:"type"`
	By   By     `json:"by"`
}

// Finding is one place the configuration departs from a rule.
type Finding struct {
	Rule    string `json:"rule"`
	Pillar  string `json:"pillar"`
	Node    string `json:"node"`
	Message string `json:"message"`
	// From lists the nodes or resources that make up the shape.
	From   []string `json:"from,omitempty"`
	Hint   string   `json:"hint"`
	Source string   `json:"source"`
}

// Find applies every rule. Graph rules read the declaration with its patterns
// expanded; the others read the resources.
func Find(in Input, rules []Rule) []Finding {
	ix := index(in.Resources)
	var out []Finding
	for _, r := range rules {
		var fs []Finding
		switch {
		case r.FanIn != nil:
			fs = fanIn(in.Spec, r)
		case r.Setting != nil:
			fs = setting(ix, r)
		case r.Companion != nil:
			fs = companion(ix, r)
		case r.Ratio != nil:
			fs = ratio(ix, r)
		case r.Shared != nil:
			fs = shared(ix, r)
		}
		for i := range fs {
			fs[i].Rule, fs[i].Pillar, fs[i].Hint, fs[i].Source = r.ID, r.Pillar, r.Hint, r.Source
			fs[i].Message = r.Title + ": " + fs[i].Message
		}
		out = append(out, fs...)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Node != out[j].Node {
			return out[i].Node < out[j].Node
		}
		return out[i].Rule < out[j].Rule
	})
	return out
}

type resources struct {
	all    []*Resource
	byAddr map[string]*Resource
}

func index(rs []Resource) resources {
	ix := resources{byAddr: map[string]*Resource{}}
	for i := range rs {
		ix.all = append(ix.all, &rs[i])
		ix.byAddr[rs[i].Address] = &rs[i]
	}
	return ix
}

func (ix resources) ofType(t string) []*Resource {
	var out []*Resource
	for _, r := range ix.all {
		if r.Type == t {
			out = append(out, r)
		}
	}
	return out
}

// referrers are the resources of by.Type that reference addr through by.Path.
func (ix resources) referrers(addr string, by By) []*Resource {
	var out []*Resource
	for _, r := range ix.ofType(by.Type) {
		if contains(refsAt(r, by.Path), addr) {
			out = append(out, r)
		}
	}
	return out
}

// targets are the resources r references through path, of type t when set.
func (ix resources) targets(r *Resource, path, t string) []*Resource {
	var out []*Resource
	for _, a := range refsAt(r, path) {
		if x, ok := ix.byAddr[a]; ok && (t == "" || x.Type == t) {
			out = append(out, x)
		}
	}
	return out
}

func (ix resources) inScope(r *Resource, s Scope) bool {
	for _, t := range s.UnlessAny {
		if len(ix.ofType(t)) > 0 {
			return false
		}
	}
	if len(s.Only) > 0 && !ix.referencedByAny(r.Address, s.Only) {
		return false
	}
	if ix.referencedByAny(r.Address, s.Except) {
		return false
	}
	for _, v := range s.Via {
		if len(ix.targets(r, v.Path, v.Type)) == 0 {
			return false
		}
	}
	return true
}

func (ix resources) referencedByAny(addr string, bys []By) bool {
	for _, by := range bys {
		if len(ix.referrers(addr, by)) > 0 {
			return true
		}
	}
	return false
}

func refsAt(r *Resource, path string) []string {
	var out []string
	for p, refs := range r.Refs {
		if p == path || strings.HasPrefix(p, path+".") {
			out = append(out, refs...)
		}
	}
	return out
}

// value reads path from r: the value, whether it is written (known, or a
// reference), and whether it is known.
func value(r *Resource, path string) (v any, written, known bool) {
	top := strings.SplitN(path, ".", 2)[0]
	if contains(r.Unknown, top) {
		return nil, true, false
	}
	v, ok := lookup(r.Attrs, strings.Split(path, "."))
	if ok {
		return v, true, true
	}
	if len(refsAt(r, path)) > 0 {
		return nil, true, false
	}
	return nil, false, true
}

// lookup walks a path through maps, stepping into the first element of a
// list, which is how nested blocks are held.
func lookup(m map[string]any, path []string) (any, bool) {
	var cur any = m
	for _, k := range path {
		if l, ok := cur.([]any); ok {
			if len(l) == 0 {
				return nil, false
			}
			cur = l[0]
		}
		mm, ok := cur.(map[string]any)
		if !ok {
			return nil, false
		}
		if cur, ok = mm[k]; !ok || cur == nil {
			return nil, false
		}
	}
	return cur, true
}

func number(v any) (float64, bool) {
	switch n := v.(type) {
	case int:
		return float64(n), true
	case float64:
		return n, true
	}
	return 0, false
}

func setting(ix resources, r Rule) []Finding {
	s := r.Setting
	var out []Finding
	for _, res := range ix.ofType(s.Type) {
		if !ix.inScope(res, s.Scope) || ix.referencedByAny(res.Address, s.Or) {
			continue
		}
		if s.WhenPath != "" {
			w, written, known := value(res, s.WhenPath)
			if !known {
				continue
			}
			if !written {
				w = s.WhenDefault
			}
			if n, ok := number(w); !ok || n <= s.WhenAbove {
				continue
			}
		}
		v, written, known := value(res, s.Path)
		if !known {
			continue
		}
		if !written {
			v = s.Default
		}
		if msg, ok := check(s, v, written); !ok {
			out = append(out, Finding{Node: res.Address, Message: msg})
		}
	}
	return out
}

// check reports whether v meets the setting, and what is wrong when not. A
// written value that is only a reference counts as set.
func check(s *Setting, v any, written bool) (string, bool) {
	switch {
	case s.Set:
		return fmt.Sprintf("%s is not set", s.Path), written
	case s.AtLeast != nil:
		n, ok := number(v)
		if v == nil {
			return fmt.Sprintf("%s is not set (want at least %v)", s.Path, *s.AtLeast), false
		}
		return fmt.Sprintf("%s is %v (want at least %v)", s.Path, v, *s.AtLeast), !ok || n >= *s.AtLeast
	case len(s.OneOf) > 0:
		for _, want := range s.OneOf {
			if fmt.Sprint(v) == fmt.Sprint(want) {
				return "", true
			}
		}
		return fmt.Sprintf("%s is %v (want one of %v)", s.Path, v, s.OneOf), false
	case s.Contains != "":
		l, _ := v.([]any)
		for _, x := range l {
			if x == s.Contains {
				return "", true
			}
		}
		return fmt.Sprintf("%s does not contain %s", s.Path, s.Contains), false
	default:
		if v == nil {
			return fmt.Sprintf("%s is not set (want %v)", s.Path, s.Equals), false
		}
		return fmt.Sprintf("%s is %v (want %v)", s.Path, v, s.Equals), fmt.Sprint(v) == fmt.Sprint(s.Equals)
	}
}

func companion(ix resources, r Rule) []Finding {
	c := r.Companion
	var out []Finding
	for _, res := range ix.ofType(c.Type) {
		if !ix.inScope(res, c.Scope) || ix.named(res, c) {
			continue
		}
		types := make([]string, len(c.By))
		for i, by := range c.By {
			types[i] = by.Type
		}
		out = append(out, Finding{Node: res.Address, Message: "no " + strings.Join(types, " or ") + " for it"})
	}
	return out
}

// named reports whether some By resource references res or names it.
func (ix resources) named(res *Resource, c *Companion) bool {
	if ix.referencedByAny(res.Address, c.By) {
		return true
	}
	if c.NameAttr == "" {
		return false
	}
	name, _ := res.Attrs[c.NameAttr].(string)
	if name == "" {
		return false
	}
	for _, by := range c.By {
		for _, x := range ix.ofType(by.Type) {
			for _, attrs := range append([]map[string]any{x.Attrs}, x.Variants...) {
				if v, ok := lookup(attrs, strings.Split(by.Path, ".")); ok && mentions(v, name) {
					return true
				}
			}
		}
	}
	return false
}

// mentions reports whether name appears as a string anywhere in v.
func mentions(v any, name string) bool {
	switch x := v.(type) {
	case string:
		return x == name || strings.Contains(x, name)
	case []any:
		for _, e := range x {
			if mentions(e, name) {
				return true
			}
		}
	case map[string]any:
		for _, e := range x {
			if mentions(e, name) {
				return true
			}
		}
	}
	return false
}

func ratio(ix resources, r Rule) []Finding {
	q := r.Ratio
	var out []Finding
	for _, link := range ix.ofType(q.Type) {
		if !ix.inScope(link, q.Scope) {
			continue
		}
		for _, left := range ix.targets(link, q.Left.Ref, "") {
			for _, right := range ix.targets(link, q.Right.Ref, "") {
				l, lok := endValue(left, q.Left)
				rv, rok := endValue(right, q.Right)
				if !lok || !rok || l >= q.Min*rv {
					continue
				}
				out = append(out, Finding{
					Node:    link.Address,
					Message: fmt.Sprintf("%s %s is %v, %v × %s %s (%v) is %v", left.Address, q.Left.Attr, l, q.Min, right.Address, q.Right.Attr, rv, q.Min*rv),
					From:    []string{left.Address, right.Address},
				})
			}
		}
	}
	return out
}

func endValue(r *Resource, e End) (float64, bool) {
	v, written, known := value(r, e.Attr)
	if !known {
		return 0, false
	}
	if !written {
		return e.Default, true
	}
	return number(v)
}

func shared(ix resources, r Rule) []Finding {
	s := r.Shared
	var out []Finding
	for _, res := range ix.ofType(s.Type) {
		users := ix.referrers(res.Address, s.By)
		count := 0
		names := make([]string, len(users))
		for i, u := range users {
			count += max(1, u.Instances)
			names[i] = u.Address
		}
		if count <= max(1, res.Instances) {
			continue
		}
		sort.Strings(names)
		out = append(out, Finding{Node: res.Address, Message: fmt.Sprintf("used by %d %s (%s)", count, s.By.Type, strings.Join(names, ", ")), From: names})
	}
	return out
}

func fanIn(spec model.Spec, r Rule) []Finding {
	f := r.FanIn
	byID := map[string]model.Node{}
	for _, n := range spec.Nodes {
		byID[n.ID] = n
	}
	feeds := map[string][]string{}
	for _, e := range spec.Edges {
		to, from := byID[e.To], byID[e.From]
		if to.Type == f.Node && from.Type == f.From && !contains(feeds[e.To], e.From) {
			feeds[e.To] = append(feeds[e.To], e.From)
		}
	}
	var out []Finding
	for _, id := range sortedKeys(feeds) {
		count := 0
		for _, from := range feeds[id] {
			count += byID[from].Count()
		}
		if count <= f.Max {
			continue
		}
		sort.Strings(feeds[id])
		out = append(out, Finding{
			Node:    id,
			Message: fmt.Sprintf("fed by %d %s (%s)", count, f.From, describe(feeds[id], byID)),
			From:    feeds[id],
		})
	}
	return out
}

// describe names the feeding nodes, with their instances when more than one.
func describe(ids []string, byID map[string]model.Node) string {
	parts := make([]string, len(ids))
	for i, id := range ids {
		parts[i] = id
		if c := byID[id].Count(); c > 1 {
			parts[i] = fmt.Sprintf("%s ×%d", id, c)
		}
	}
	return strings.Join(parts, ", ")
}

// Text renders findings for a terminal, grouped by rule.
func Text(fs []Finding) string {
	if len(fs) == 0 {
		return "No departures from the known practices.\n"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%d departure(s) from the known practices. Fix them, or write down why the design departs.\n", len(fs))
	byRule := map[string][]Finding{}
	var order []string
	for _, f := range fs {
		if _, ok := byRule[f.Rule]; !ok {
			order = append(order, f.Rule)
		}
		byRule[f.Rule] = append(byRule[f.Rule], f)
	}
	sort.Strings(order)
	for _, id := range order {
		group := byRule[id]
		fmt.Fprintf(&b, "\n## %s [%s] ×%d\nhint: %s\nsource: %s\n", id, group[0].Pillar, len(group), group[0].Hint, group[0].Source)
		for _, f := range group {
			fmt.Fprintf(&b, "- %s\n  %s\n", f.Node, f.Message)
		}
	}
	return b.String()
}

func contains(xs []string, x string) bool {
	for _, y := range xs {
		if y == x {
			return true
		}
	}
	return false
}

func sortedKeys(m map[string][]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
