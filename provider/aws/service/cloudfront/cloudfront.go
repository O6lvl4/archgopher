// Package cloudfront reads Amazon CloudFront distributions.
package cloudfront

import (
	"embed"

	"github.com/O6lvl4/arch-scouter/facet"
	"github.com/O6lvl4/arch-scouter/meter"
	"github.com/O6lvl4/arch-scouter/model"
	"github.com/O6lvl4/arch-scouter/provider/aws/kit"
	"github.com/O6lvl4/arch-scouter/scouter"
	"github.com/O6lvl4/arch-scouter/terraform/infer"
)

//go:embed books
var books embed.FS

type none struct{}

type assume struct {
	ResponseKb float64 `scout:"responseKb" label:"Response size" unit:"KB"`
}

// Distribution bills HTTPS requests and data transfer out. Prices follow the
// viewer's location; the book keys them by the edge nearest the spec region.
var Distribution = scouter.Def[none, assume]{
	Info: scouter.Meta{
		Type: "aws_cloudfront_distribution", Label: "CloudFront", Category: "Edge", SLA: "aws.cloudfront",
		Description: "HTTPS requests and transfer out, priced at the edge nearest the spec region. Free tier is ignored.",
		Kinds:       []string{"request"},
	},
	Run: func(_ none, p assume, d model.Demand, r *meter.Recorder) {
		in := d.Total()
		facet.Requests{Name: "HTTPS requests", Unit: "request", PriceID: "aws.cloudfront.https_requests"}.Read(r, in.Monthly, 0)
		r.Cost("Transfer out", in.Monthly*p.ResponseKb/1024/1024, "GB", "aws.cloudfront.transfer_out")
		facet.Rate{Name: "Requests per distribution", Unit: "requests/second", QuotaID: "aws.cloudfront.distribution_rps"}.Read(r, in.PeakPerSecond)
	},
}

// Service is CloudFront's contribution. A distribution is the outermost front
// door: references to it (callback URLs, links in emails) are mentions.
var Service = kit.Service{
	Name:     "cloudfront",
	Scouters: []scouter.Scouter{Distribution},
	Books:    books,
	Terraform: infer.Rules{
		FrontDoors: map[string]bool{"aws_cloudfront_distribution": true},
		Mentioned:  map[string]bool{"aws_cloudfront_distribution": true},
		IgnoreRefs: []string{"web_acl_id", "viewer_certificate"},
	},
}
