package cloud

import (
	"testing"

	"github.com/O6lvl4/archgopher/model"
)

func TestFillRegion(t *testing.T) {
	cases := []struct {
		name   string
		spec   model.Spec
		region string
		warn   bool
	}{
		{"kept", model.Spec{Region: "eu-west-1", Nodes: []model.Node{{Type: "aws_s3_bucket"}}}, "eu-west-1", false},
		{"regional node", model.Spec{Nodes: []model.Node{{Type: "entry"}, {Type: "aws_s3_bucket"}}}, DefaultRegion, true},
		{"Cloudflare only", model.Spec{Nodes: []model.Node{{Type: "entry"}, {Type: "cloudflare_workers_script"}}}, DefaultRegion, false},
		{"empty", model.Spec{}, DefaultRegion, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			warn := FillRegion(&c.spec)
			if c.spec.Region != c.region || warn != c.warn {
				t.Errorf("got region %q warn %v, want %q %v", c.spec.Region, warn, c.region, c.warn)
			}
		})
	}
}
