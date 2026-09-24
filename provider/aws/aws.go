// Package aws is the AWS provider: it combines the service packages into one
// scouter registry, one set of reference books and one set of Terraform rules.
// Adding a service is adding its package to Services.
package aws

import (
	"sort"

	"github.com/O6lvl4/arch-scouter/book"
	"github.com/O6lvl4/arch-scouter/provider/aws/iam"
	"github.com/O6lvl4/arch-scouter/provider/aws/kit"
	"github.com/O6lvl4/arch-scouter/provider/aws/service/apigateway"
	"github.com/O6lvl4/arch-scouter/provider/aws/service/aurora"
	"github.com/O6lvl4/arch-scouter/provider/aws/service/bedrock"
	"github.com/O6lvl4/arch-scouter/provider/aws/service/cloudfront"
	"github.com/O6lvl4/arch-scouter/provider/aws/service/cloudwatch"
	"github.com/O6lvl4/arch-scouter/provider/aws/service/dynamodb"
	"github.com/O6lvl4/arch-scouter/provider/aws/service/eventbridge"
	"github.com/O6lvl4/arch-scouter/provider/aws/service/lambda"
	"github.com/O6lvl4/arch-scouter/provider/aws/service/s3"
	"github.com/O6lvl4/arch-scouter/provider/aws/service/sns"
	"github.com/O6lvl4/arch-scouter/provider/aws/service/sqs"
	"github.com/O6lvl4/arch-scouter/provider/aws/service/stepfunctions"
	"github.com/O6lvl4/arch-scouter/scouter"
	"github.com/O6lvl4/arch-scouter/terraform/infer"
)

// Services are the AWS services arch-scouter reads.
var Services = []kit.Service{
	apigateway.Service, aurora.Service, bedrock.Service, cloudfront.Service, cloudwatch.Service,
	dynamodb.Service, eventbridge.Service, lambda.Service, s3.Service, sns.Service, sqs.Service,
	stepfunctions.Service,
}

// Registry returns every AWS scouter plus the provider-neutral entry.
func Registry() scouter.Registry {
	reg := scouter.Registry{}
	reg.Register(scouter.EntryScouter)
	for _, s := range Services {
		reg.Register(s.Scouters...)
	}
	return reg
}

// Books merges every service's books; an id defined by two services is an error.
func Books() (book.Books, error) {
	parts := make([]book.Books, 0, len(Services))
	for _, s := range Services {
		b, err := s.LoadBooks()
		if err != nil {
			return book.Books{}, err
		}
		parts = append(parts, b)
	}
	return book.Merge(parts...)
}

// Actions merges every service's IAM action table.
func Actions() kit.Actions {
	out := kit.Actions{}
	for _, s := range Services {
		for t, kinds := range s.Actions {
			out[t] = kinds
		}
	}
	return out
}

// kindOrder lists every kind of work, so IAM edges come out in a stable order.
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

// IAM is the IAM configuration the edge source uses.
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

// TerraformRules combine the account-wide rules, every service's rules and
// the IAM edge source.
func TerraformRules() infer.Rules {
	parts := []infer.Rules{common(), {Scouters: Registry(), Sources: []infer.EdgeSource{iam.Source(IAM())}}}
	for _, s := range Services {
		r := s.Terraform
		r.NodeTypes = s.Nodes()
		parts = append(parts, r)
	}
	return infer.Combine(parts...)
}
