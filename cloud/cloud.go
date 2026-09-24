// Package cloud puts the providers together: one registry, one set of books,
// one set of Terraform rules and the regions of each provider. The engine and
// the Terraform adapter know no provider; this is the one place that lists them.
package cloud

import (
	"github.com/O6lvl4/archgopher/book"
	"github.com/O6lvl4/archgopher/pattern"
	"github.com/O6lvl4/archgopher/provider/aws"
	awspattern "github.com/O6lvl4/archgopher/provider/aws/pattern"
	"github.com/O6lvl4/archgopher/provider/azure"
	"github.com/O6lvl4/archgopher/scouter"
	"github.com/O6lvl4/archgopher/terraform/infer"
)

// Registry holds every provider's resources and the entry.
func Registry() scouter.Registry {
	reg := scouter.Registry{}
	for _, r := range []scouter.Registry{aws.Registry(), azure.Registry()} {
		for t, s := range r {
			reg[t] = s
		}
	}
	return reg
}

// Books merges every provider's books; an id defined twice is an error.
func Books() (book.Books, error) {
	a, err := aws.Books()
	if err != nil {
		return book.Books{}, err
	}
	z, err := azure.Books()
	if err != nil {
		return book.Books{}, err
	}
	return book.Merge(a, z)
}

// TerraformRules combine every provider's rules.
func TerraformRules() infer.Rules {
	return infer.Combine(aws.TerraformRules(), azure.TerraformRules())
}

// Patterns are the L3 patterns.
func Patterns() pattern.Registry { return awspattern.Registry() }

// RegionGroup is one provider's regions.
type RegionGroup struct {
	Provider string   `json:"provider"`
	Label    string   `json:"label"`
	Regions  []string `json:"regions"`
}

// Regions lists the regions each provider's price books cover.
func Regions() ([]RegionGroup, error) {
	a, err := aws.Regions()
	if err != nil {
		return nil, err
	}
	z, err := azure.Regions()
	if err != nil {
		return nil, err
	}
	return []RegionGroup{{Provider: "aws", Label: "AWS", Regions: a}, {Provider: "azure", Label: "Azure", Regions: z}}, nil
}
