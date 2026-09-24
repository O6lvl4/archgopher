package iam

import (
	"path"
	"strings"

	"github.com/O6lvl4/arch-scouter/terraform/eval"
	"github.com/O6lvl4/arch-scouter/terraform/infer"
)

// RoleLink says a resource of Type attaches policies to the role referenced
// by its Role attribute. Policy, when set, points at a separate policy resource.
type RoleLink struct {
	Type   string
	Role   string
	Policy string
}

// Config is what the edge source needs to know about AWS IAM.
type Config struct {
	// RoleAttrs are attribute paths through which a node acts as an IAM role.
	RoleAttrs []string
	RoleLinks []RoleLink
	// Actions maps a target type to kinds and the actions that do that work.
	Actions map[string]map[string][]string
	// KindOrder fixes the order kinds are emitted in.
	KindOrder []string
}

// Source returns an edge source: for every node, the permissions of the roles
// it acts as become edges to the resources they name, with kinds of work.
func Source(c Config) infer.EdgeSource {
	return func(g *infer.Graph) []infer.Hint {
		var out []infer.Hint
		for _, n := range g.Resources() {
			if !g.IsNode(n.Address) {
				continue
			}
			for _, st := range c.statementsOf(g, n) {
				for _, ref := range st.Targets {
					for _, to := range g.Targets(ref) {
						target, _ := g.Resource(to)
						for _, kind := range c.Kinds(target.Type, st.Actions) {
							out = append(out, infer.Hint{From: n.Address, To: to, Kind: kind})
						}
					}
				}
			}
		}
		return out
	}
}

func (c Config) statementsOf(g *infer.Graph, n *eval.Resource) []Statement {
	var out []Statement
	for _, attr := range c.RoleAttrs {
		for _, role := range n.Refs[attr] {
			out = append(out, c.roleStatements(g, role)...)
		}
	}
	return out
}

func (c Config) roleStatements(g *infer.Graph, role string) []Statement {
	var out []Statement
	if rr, ok := g.Resource(role); ok {
		out = append(out, expand(g, Statements(rr))...)
	}
	for _, r := range g.Resources() {
		for _, rl := range c.RoleLinks {
			if r.Type != rl.Type || !contains(r.Refs[rl.Role], role) {
				continue
			}
			if rl.Policy == "" {
				out = append(out, expand(g, Statements(r))...)
				continue
			}
			for _, pol := range r.Refs[rl.Policy] {
				if pr, ok := g.Resource(pol); ok {
					out = append(out, expand(g, Statements(pr))...)
				}
			}
		}
	}
	return out
}

// expand replaces references to policy documents with their statements.
func expand(g *infer.Graph, sts []Statement) []Statement {
	var out []Statement
	for _, st := range sts {
		for _, d := range st.Docs {
			if dr, ok := g.Resource(d); ok {
				out = append(out, expand(g, Statements(dr))...)
			}
		}
		if len(st.Targets) > 0 {
			out = append(out, st)
		}
	}
	return out
}

// Kinds returns the kinds a set of actions exercises on a target. Unknown
// actions, or a target type without a table, give the target's default kind;
// a known target with no load-carrying action (DescribeTable) gives no edge.
func (c Config) Kinds(targetType string, actions []string) []string {
	if actions == nil {
		return []string{""}
	}
	table, ok := c.Actions[targetType]
	if !ok {
		return []string{""}
	}
	var out []string
	for _, kind := range c.KindOrder {
		for _, known := range table[kind] {
			if anyMatch(actions, known) {
				out = append(out, kind)
				break
			}
		}
	}
	return out
}

func anyMatch(globs []string, action string) bool {
	for _, g := range globs {
		if ok, _ := path.Match(strings.ToLower(g), strings.ToLower(action)); ok {
			return true
		}
	}
	return false
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}
