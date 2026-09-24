// Package bedrock reads Amazon Bedrock model invocations. A model is not a
// Terraform resource: place it by hand and connect its callers to it.
package bedrock

import (
	"embed"

	"github.com/O6lvl4/arch-scouter/facet"
	"github.com/O6lvl4/arch-scouter/meter"
	"github.com/O6lvl4/arch-scouter/model"
	"github.com/O6lvl4/arch-scouter/provider/aws/kit"
	"github.com/O6lvl4/arch-scouter/scouter"
)

//go:embed books
var books embed.FS

type none struct{}

type assume struct {
	Model string `scout:"model" label:"Model" options:"claude-sonnet-4-5,claude-haiku-4-5,claude-opus-4-5"`
	facet.TokenAssume
}

// Model reads on-demand invocations of one model.
var Model = scouter.Def[none, assume]{
	Info: scouter.Meta{
		Type: "bedrock_model", Label: "Bedrock model", Category: "AI", SLA: "aws.bedrock", External: true,
		Description: "Input, output and prompt-cache tokens. Headroom is tokens and requests per minute: output counts with the burndown multiplier, cache reads do not count.",
		Kinds:       []string{"call"},
	},
	Run: func(_ none, p assume, d model.Demand, r *meter.Recorder) {
		in := d.Total()
		facet.Tokens{Prefix: "aws.bedrock." + p.Model}.Read(r, in.Monthly, in.PeakPerSecond, p.TokenAssume)
	},
}

// Service is Bedrock's contribution to the AWS provider.
var Service = kit.Service{
	Name:     "bedrock",
	Scouters: []scouter.Scouter{Model},
	Books:    books,
}
