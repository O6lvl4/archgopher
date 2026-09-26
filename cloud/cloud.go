// Package cloud puts the providers together: one registry, one set of books,
// one set of Terraform rules and the regions of each provider. The engine and
// the Terraform adapter know no provider; this is the one place that lists them.
package cloud

import (
	"github.com/O6lvl4/archgopher/book"
	"github.com/O6lvl4/archgopher/model"
	"github.com/O6lvl4/archgopher/pattern"
	"github.com/O6lvl4/archgopher/provider/aws"
	awspattern "github.com/O6lvl4/archgopher/provider/aws/pattern"
	"github.com/O6lvl4/archgopher/provider/azure"
	"github.com/O6lvl4/archgopher/provider/cloudflare"
	"github.com/O6lvl4/archgopher/provider/conoha"
	"github.com/O6lvl4/archgopher/provider/gcp"
	"github.com/O6lvl4/archgopher/scouter"
	"github.com/O6lvl4/archgopher/terraform/infer"
)

// Registry holds every provider's resources and the entry.
func Registry() scouter.Registry {
	reg := scouter.Registry{}
	for _, r := range []scouter.Registry{aws.Registry(), azure.Registry(), gcp.Registry(), cloudflare.Registry(), conoha.Registry()} {
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
	g, err := gcp.Books()
	if err != nil {
		return book.Books{}, err
	}
	c, err := cloudflare.Books()
	if err != nil {
		return book.Books{}, err
	}
	h, err := conoha.Books()
	if err != nil {
		return book.Books{}, err
	}
	return book.Merge(a, z, g, c, h)
}

// TerraformRules combine every provider's rules.
func TerraformRules() infer.Rules {
	return infer.Combine(aws.TerraformRules(), azure.TerraformRules(), gcp.TerraformRules(), cloudflare.TerraformRules(), conoha.TerraformRules())
}

// Patterns are the L3 patterns.
func Patterns() pattern.Registry { return awspattern.Registry() }

// RegionGroup is one provider's regions.
type RegionGroup struct {
	Provider string   `json:"provider"`
	Label    string   `json:"label"`
	Regions  []string `json:"regions"`
}

// Regions lists the regions each provider's price books cover. Cloudflare
// and ConoHa have none: their prices are the same wherever the declaration
// is, so their resources price in any region of the others.
func Regions() ([]RegionGroup, error) {
	a, err := aws.Regions()
	if err != nil {
		return nil, err
	}
	z, err := azure.Regions()
	if err != nil {
		return nil, err
	}
	g, err := gcp.Regions()
	if err != nil {
		return nil, err
	}
	return []RegionGroup{
		{Provider: "aws", Label: "AWS", Regions: a},
		{Provider: "azure", Label: "Azure", Regions: z},
		{Provider: "gcp", Label: "Google Cloud", Regions: g},
	}, nil
}

// DefaultRegion is the region a declaration without one gets.
const DefaultRegion = "us-east-1"

// FillRegion gives a declaration without a region the default one. It reports
// whether that choice can change a result: it cannot when every node is of a
// provider whose prices are the same everywhere (Cloudflare, ConoHa) or is an
// entry.
func FillRegion(spec *model.Spec) (warn bool) {
	if spec.Region != "" {
		return false
	}
	spec.Region = DefaultRegion
	cf, ch := cloudflare.Registry(), conoha.Registry()
	for _, n := range spec.Nodes {
		_, inCF := cf[n.Type]
		_, inCH := ch[n.Type]
		if !inCF && !inCH {
			return true
		}
	}
	return false
}
