// Package aws is the AWS provider. Resources live as data in catalog/aws, one
// directory per resource type; this package loads them and adds what is not
// per resource: the IAM edge source, account-wide
// Terraform rules and the L3 patterns.
package aws

import (
	"sort"

	"github.com/O6lvl4/archgopher/book"
	"github.com/O6lvl4/archgopher/catalog"
	"github.com/O6lvl4/archgopher/definition"
	"github.com/O6lvl4/archgopher/provider/aws/iam"
	"github.com/O6lvl4/archgopher/provider/internal/embedded"
	"github.com/O6lvl4/archgopher/scouter"
	"github.com/O6lvl4/archgopher/terraform/infer"
)

var cat = embedded.New("AWS", catalog.AWS, catalog.AWSRoot)

// Units loads the catalog once.
func Units() ([]definition.Unit, error) { return cat.Units() }

// Registry returns every AWS resource plus the provider-neutral entry.
func Registry() scouter.Registry { return cat.Registry() }

// Books merges the books of every unit; an id owned by two units is an error.
func Books() (book.Books, error) { return cat.Books() }

// Regions lists every region the price books cover, sorted. A region is
// supported when every row that varies by region has a value for it, which
// TestEveryRegionIsComplete checks.
func Regions() ([]string, error) { return cat.Regions() }

// Actions maps each resource type to kinds of work and the IAM actions that do them.
func Actions() map[string]map[string][]string {
	out := map[string]map[string][]string{}
	for _, u := range cat.MustUnits() {
		if u.Resource != nil && len(u.Resource.File.IAM) > 0 {
			out[u.Resource.File.Type] = u.Resource.File.IAM
		}
	}
	return out
}

func kindOrder(reg scouter.Registry) []string {
	seen := map[string]bool{}
	var out []string
	for _, t := range reg.Types() {
		for _, k := range reg[t].Meta().Kinds {
			if !seen[k] {
				seen[k] = true
				out = append(out, k)
			}
		}
	}
	sort.Strings(out)
	return out
}

// IAM is the configuration of the IAM edge source.
func IAM() iam.Config {
	return iam.Config{
		RoleAttrs: roleAttrs,
		RoleLinks: []iam.RoleLink{
			{Type: "aws_iam_role_policy", Role: "role"},
			{Type: "aws_iam_role_policy_attachment", Role: "role", Policy: "policy_arn"},
			{Type: "aws_iam_policy_attachment", Role: "roles", Policy: "policy_arn"},
		},
		Actions:   Actions(),
		KindOrder: kindOrder(Registry()),
	}
}

// TerraformRules combine the account-wide rules, every resource's rules, the
// IAM edge source.
func TerraformRules() infer.Rules {
	return cat.Rules(common(), infer.Rules{
		Scouters: Registry(),
		Free:     cat.FreeTypes(),
		Sources:  []infer.EdgeSource{iam.Source(IAM())},
	})
}
