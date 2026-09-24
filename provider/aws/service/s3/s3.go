// Package s3 reads Amazon S3 buckets (S3 Standard).
package s3

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
	StorageGb float64 `scout:"storageGb" label:"Storage" unit:"GB"`
	Prefixes  float64 `scout:"prefixes" label:"Prefixes the load spreads over" default:"1" hint:"Request rate limits apply per prefix"`
}

// Bucket reads GET, PUT and storage; headroom is the per-prefix rate.
var Bucket = scouter.Def[none, assume]{
	Info: scouter.Meta{
		Type: "aws_s3_bucket", Label: "S3", Category: "Storage", SLA: "aws.s3",
		Description: "S3 Standard GET, PUT and storage. Headroom is the per-prefix request rate times the prefixes in use.",
		Kinds:       []string{"read", "write"},
	},
	Run: func(_ none, p assume, d model.Demand, r *meter.Recorder) {
		reads, writes := d.Of("read"), d.Of("write")
		facet.Requests{Name: "GET requests", Unit: "request", PriceID: "aws.s3.standard.get"}.Read(r, reads.Monthly, 0)
		facet.Requests{Name: "PUT requests", Unit: "request", PriceID: "aws.s3.standard.put"}.Read(r, writes.Monthly, 0)
		facet.Storage{Name: "Storage", PriceID: "aws.s3.standard.storage"}.Read(r, p.StorageGb)
		facet.Rate{Name: "GET rate", Unit: "requests/second", QuotaID: "aws.s3.prefix_get"}.ReadScaled(r, reads.PeakPerSecond, p.Prefixes)
		facet.Rate{Name: "PUT rate", Unit: "requests/second", QuotaID: "aws.s3.prefix_put"}.ReadScaled(r, writes.PeakPerSecond, p.Prefixes)
	},
}

// Service is S3's contribution to the AWS provider.
var Service = kit.Service{
	Name:     "s3",
	Scouters: []scouter.Scouter{Bucket},
	Books:    books,
	Terraform: infer.Rules{
		Links:   []infer.Link{{Type: "aws_s3_bucket_notification", From: "bucket", To: []string{"lambda_function.lambda_function_arn", "queue.queue_arn", "topic.topic_arn"}}},
		Aliases: map[string]string{"aws_s3_bucket_website_configuration": "bucket"},
	},
	Actions: kit.Actions{"aws_s3_bucket": {
		"read":  {"s3:GetObject"},
		"write": {"s3:PutObject", "s3:DeleteObject"},
	}},
}
