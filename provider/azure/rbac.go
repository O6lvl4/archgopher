package azure

import (
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
		for _, n := range names {
			if strings.EqualFold(n, role) {
				out = append(out, kind)
				break
			}
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
			if a.Type != "azurerm_role_assignment" {
				continue
			}
			role, _ := a.Attrs["role_definition_name"].(string)
			if role == "" {
				continue
			}
			for _, from := range principals(g, refsAt(a, "principal_id")) {
				for _, ref := range refsAt(a, "scope") {
					for _, to := range g.Targets(ref) {
						target, _ := g.Resource(to)
						for _, kind := range roles.Kinds(target.Type, role) {
							out = append(out, infer.Hint{From: from, To: to, Kind: kind})
						}
					}
				}
			}
		}
		return out
	}
}

// principals resolves principal references to the nodes that act with them.
func principals(g *infer.Graph, refs []string) []string {
	var out []string
	for _, ref := range refs {
		if g.IsNode(ref) {
			out = append(out, ref)
			continue
		}
		r, ok := g.Resource(ref)
		if !ok || r.Type != "azurerm_user_assigned_identity" {
			continue
		}
		for _, n := range g.Resources() {
			if !g.IsNode(n.Address) {
				continue
			}
			for _, u := range refsAt(n, "identity") {
				if u == ref {
					out = append(out, n.Address)
					break
				}
			}
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
	for _, u := range mustUnits() {
		if u.Resource != nil && len(u.Resource.File.IAM) > 0 {
			out[u.Resource.File.Type] = u.Resource.File.IAM
		}
	}
	return out
}
