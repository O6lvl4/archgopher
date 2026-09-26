package azure

import (
	"slices"
	"sort"
	"strings"

	"github.com/O6lvl4/archgopher/terraform/eval"
	"github.com/O6lvl4/archgopher/terraform/infer"
)

// Roles maps a resource type to the kinds of work each Azure role grants on
// it: {"azurerm_storage_account": {"read": ["Storage Blob Data Reader", ...]}}.
// It comes from each unit's iam map, which for Azure holds role names.
type Roles map[string]map[string][]string

// Kinds lists the kinds a role grants on a target type, in a stable order.
func (r Roles) Kinds(targetType, role string) []string {
	var out []string
	for kind, names := range r[targetType] {
		if slices.ContainsFunc(names, func(n string) bool { return strings.EqualFold(n, role) }) {
			out = append(out, kind)
		}
	}
	sort.Strings(out)
	return out
}

// RBAC proposes an edge for each role assignment: from the resource whose
// managed identity holds the role to the resource the role is scoped to, of
// the kinds the role grants. A system-assigned identity is its resource; a
// user-assigned identity stands for every node that lists it in identity.
func RBAC(roles Roles) infer.EdgeSource {
	return func(g *infer.Graph) []infer.Hint {
		var out []infer.Hint
		for _, a := range g.Resources() {
			out = append(out, roles.granted(g, a)...)
		}
		return out
	}
}

// granted is the edges one role assignment grants; a resource that is not a
// role assignment, or names no role, grants none.
func (r Roles) granted(g *infer.Graph, a *eval.Resource) []infer.Hint {
	role, _ := a.Attrs["role_definition_name"].(string)
	if a.Type != "azurerm_role_assignment" || role == "" {
		return nil
	}
	scopes := scopes(g, a)
	var out []infer.Hint
	for _, from := range principals(g, refsAt(a, "principal_id")) {
		for _, to := range scopes {
			target, _ := g.Resource(to)
			for _, kind := range r.Kinds(target.Type, role) {
				out = append(out, infer.Hint{From: from, To: to, Kind: kind})
			}
		}
	}
	return out
}

// scopes resolves the scope of a role assignment to the nodes it covers.
func scopes(g *infer.Graph, a *eval.Resource) []string {
	var out []string
	for _, ref := range refsAt(a, "scope") {
		out = append(out, g.Targets(ref)...)
	}
	return out
}

// principals resolves principal references to the nodes that act with them.
func principals(g *infer.Graph, refs []string) []string {
	var out []string
	for _, ref := range refs {
		if g.IsNode(ref) {
			out = append(out, ref)
			continue
		}
		if r, ok := g.Resource(ref); ok && r.Type == "azurerm_user_assigned_identity" {
			out = append(out, holders(g, ref)...)
		}
	}
	return out
}

// holders are the nodes that list a user-assigned identity in identity.
func holders(g *infer.Graph, identity string) []string {
	var out []string
	for _, n := range g.Resources() {
		if g.IsNode(n.Address) && slices.Contains(refsAt(n, "identity"), identity) {
			out = append(out, n.Address)
		}
	}
	return out
}

// refsAt lists the resources referenced at an attribute path or below it.
func refsAt(r *eval.Resource, path string) []string {
	var out []string
	for p, refs := range r.Refs {
		if p == path || strings.HasPrefix(p, path+".") {
			out = append(out, refs...)
		}
	}
	sort.Strings(out)
	return out
}

// roles collects every unit's iam map.
func roles() Roles {
	out := Roles{}
	for _, u := range cat.MustUnits() {
		if u.Resource != nil && len(u.Resource.File.IAM) > 0 {
			out[u.Resource.File.Type] = u.Resource.File.IAM
		}
	}
	return out
}
