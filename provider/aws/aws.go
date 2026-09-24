// Package aws is the AWS provider. Resources live as data in catalog/aws, one
// directory per resource type; this package loads them and adds what is not
// per resource: the IAM edge source, the schedule syntax, account-wide
// Terraform rules and the L3 patterns.
package aws

import (
	"sort"
	"sync"

	"github.com/O6lvl4/arch-scouter/book"
	"github.com/O6lvl4/arch-scouter/catalog"
	"github.com/O6lvl4/arch-scouter/definition"
	"github.com/O6lvl4/arch-scouter/provider/aws/iam"
	"github.com/O6lvl4/arch-scouter/provider/aws/schedule"
	"github.com/O6lvl4/arch-scouter/scouter"
	"github.com/O6lvl4/arch-scouter/terraform/infer"
)

var (
	once   sync.Once
	units  []definition.Unit
	errCat error
)

// Units loads the catalog once.
func Units() ([]definition.Unit, error) {
	once.Do(func() { units, errCat = definition.LoadAll(catalog.AWS, catalog.AWSRoot) })
	return units, errCat
}

func mustUnits() []definition.Unit {
	u, err := Units()
	if err != nil {
		// The catalog is embedded and tested; a broken one is a build defect.
		panic("arch-scouter: AWS catalog: " + err.Error())
	}
	return u
}

// Registry returns every AWS resource plus the provider-neutral entry.
func Registry() scouter.Registry {
	reg := scouter.Registry{}
	reg.Register(scouter.EntryScouter)
	for _, u := range mustUnits() {
		if u.Resource != nil {
			reg.Register(u.Resource)
		}
	}
	return reg
}

// Books merges the books of every unit; an id owned by two units is an error.
func Books() (book.Books, error) {
	us, err := Units()
	if err != nil {
		return book.Books{}, err
	}
	parts := make([]book.Books, 0, len(us))
	for _, u := range us {
		parts = append(parts, u.Books)
	}
	return book.Merge(parts...)
}

// Actions maps each resource type to kinds of work and the IAM actions that do them.
func Actions() map[string]map[string][]string {
	out := map[string]map[string][]string{}
	for _, u := range mustUnits() {
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
// IAM edge source and the EventBridge schedule syntax.
func TerraformRules() infer.Rules {
	parts := []infer.Rules{common(), {
		Scouters:     Registry(),
		Sources:      []infer.EdgeSource{iam.Source(IAM())},
		ScheduleLoad: schedule.Load,
	}}
	for _, u := range mustUnits() {
		if u.Resource != nil {
			parts = append(parts, u.Resource.Rules())
		}
	}
	return infer.Combine(parts...)
}
