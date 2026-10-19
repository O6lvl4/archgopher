package iam

import (
	"fmt"
	"sort"
	"strings"

	"github.com/O6lvl4/archgopher/terraform/infer"
)

// A policy's Resource is usually a reference, and the edge source follows
// it. When it is an ARN written out (or built from variables), or "*", it
// names something the configuration does not own: a foundation model, a
// guardrail or a bucket in another account, a queue another team made. Those
// are the calls the declaration would otherwise read as zero, so they become
// stub nodes placed outside the configuration.

// Stubs returns a node source: for every node, the ARNs its roles are allowed
// to act on that no resource here stands for, as nodes of the catalog type
// whose ARN shape they fit, with a kinded edge from the node.
func Stubs(c Config) infer.NodeSource {
	return func(g *infer.Graph) []infer.Stub {
		var out []infer.Stub
		for _, n := range g.Resources() {
			if !g.IsNode(n.Address) {
				continue
			}
			for _, st := range c.statementsOf(g, n) {
				out = append(out, c.stubs(n.Address, st)...)
			}
		}
		return out
	}
}

// stubs are the stubs one statement proposes for a caller: one per ARN and
// matching type, and for "*" one per external type the actions can only
// mean (a model for bedrock:InvokeModel). A type whose table has no
// load-carrying action among the statement's gives nothing.
func (c Config) stubs(from string, st Statement) []infer.Stub {
	var out []infer.Stub
	for _, lit := range st.Literals {
		if lit == "*" {
			for _, typ := range sortedTypes(c.External) {
				for _, kind := range c.knownKinds(typ, st.Actions) {
					out = append(out, infer.Stub{
						Key: typ + ":*", Type: typ, From: from, Kind: kind,
						Note: fmt.Sprintf("Outside this configuration: the role of %s allows %s on every resource; which one is not in Terraform.", from, actionsText(st.Actions)),
					})
				}
			}
			continue
		}
		a, ok := parseARN(lit)
		if !ok {
			continue
		}
		for _, typ := range sortedTypes(c.ARNs) {
			pattern, ok := a.match(c.ARNs[typ])
			if !ok {
				continue
			}
			// An ARN that names nothing past the pattern's own words
			// (foundation-model/*) is the same stub as "*".
			key, name := lit, a.nameUnder(pattern)
			if name == "" {
				key = typ + ":*"
			}
			for _, kind := range c.arnKinds(typ, st.Actions) {
				out = append(out, infer.Stub{
					Key: key, Type: typ, Name: name, Match: lit, From: from, Kind: kind,
					Note: fmt.Sprintf("Outside this configuration: the role of %s allows %s on %s%s.", from, actionsText(st.Actions), lit, a.accountText()),
				})
			}
		}
	}
	return out
}

// knownKinds are the kinds the actions exercise on a type with a table; nil
// without a table or without known actions. Used for "*", where only the
// actions say what is called.
func (c Config) knownKinds(typ string, actions []string) []string {
	if _, ok := c.Actions[typ]; !ok || actions == nil {
		return nil
	}
	return c.Kinds(typ, actions)
}

// arnKinds are the kinds for a type the ARN names: Kinds, which gives the
// default kind when the type has no table or the actions are unknown.
func (c Config) arnKinds(typ string, actions []string) []string {
	return c.Kinds(typ, actions)
}

// arn is a parsed arn:partition:service:region:account:resource.
type arn struct{ service, account, resource string }

func parseARN(s string) (arn, bool) {
	p := strings.SplitN(s, ":", 6)
	if len(p) != 6 || p[0] != "arn" {
		return arn{}, false
	}
	return arn{service: p[2], account: p[4], resource: p[5]}, true
}

// match returns the first "service:resource" pattern the ARN fits, * standing
// for any text.
func (a arn) match(patterns []string) (string, bool) {
	for _, p := range patterns {
		svc, res, ok := strings.Cut(p, ":")
		if ok && svc == a.service && wildcard(res, a.resource) {
			return p, true
		}
	}
	return "", false
}

// nameUnder is what the ARN names past the pattern's literal lead
// ("foundation-model/" of "bedrock:foundation-model/*"): the bucket for S3,
// otherwise the last path segment that is not a wildcard, without a trailing
// version ("anthropic.claude-haiku-4-5-20251001-v1:0" keeps its text up to
// the colon). "" when only wildcards remain.
func (a arn) nameUnder(pattern string) string {
	_, res, _ := strings.Cut(pattern, ":")
	lead, _, _ := strings.Cut(res, "*")
	rest := strings.TrimPrefix(a.resource, lead)
	var segs []string
	for _, s := range strings.Split(rest, "/") {
		if s != "" && s != "*" {
			segs = append(segs, s)
		}
	}
	if len(segs) == 0 {
		return ""
	}
	n := segs[len(segs)-1]
	if a.service == "s3" {
		n = segs[0]
	}
	if i := strings.Index(n, ":"); i > 0 {
		// lambda's function:name keeps the name; a model's name:version drops the version
		if head, tail, _ := strings.Cut(n, ":"); strings.Contains(tail, ".") || len(head) > len(tail) {
			n = head
		} else {
			n = tail
		}
	}
	return n
}

func (a arn) accountText() string {
	if a.account == "" || a.account == "*" {
		return ""
	}
	return " (account " + a.account + ")"
}

func actionsText(actions []string) string {
	if actions == nil {
		return "actions it could not read"
	}
	return strings.Join(actions, ", ")
}

// wildcard matches s against a pattern where * matches any text.
func wildcard(pattern, s string) bool {
	parts := strings.Split(pattern, "*")
	if len(parts) == 1 {
		return pattern == s
	}
	if !strings.HasPrefix(s, parts[0]) {
		return false
	}
	s = s[len(parts[0]):]
	for i := 1; i < len(parts)-1; i++ {
		j := strings.Index(s, parts[i])
		if j < 0 {
			return false
		}
		s = s[j+len(parts[i]):]
	}
	return strings.HasSuffix(s, parts[len(parts)-1])
}

func sortedTypes[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
