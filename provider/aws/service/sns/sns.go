// Package sns reads Amazon SNS topics.
package sns

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
	MessageKb float64 `scout:"messageKb" label:"Message size" unit:"KB" default:"1" hint:"Billed in 64 KB chunks"`
}

// Topic bills publishes; delivery to Lambda and SQS is free.
var Topic = scouter.Def[none, assume]{
	Info: scouter.Meta{
		Type: "aws_sns_topic", Label: "SNS", Category: "Messaging", SLA: "aws.sns",
		Description: "Publishes in 64 KB chunks; delivery to Lambda and SQS is free. Headroom is the regional publish rate.",
		Kinds:       []string{"publish"},
	},
	Run: func(_ none, p assume, d model.Demand, r *meter.Recorder) {
		in := d.Total()
		facet.Requests{Name: "Publishes", Unit: "request", PriceID: "aws.sns.publish", ChunkKB: 64}.Read(r, in.Monthly, p.MessageKb)
		facet.Rate{Name: "Publish rate", Unit: "requests/second", QuotaID: "aws.sns.publish_rps"}.Read(r, in.PeakPerSecond)
	},
}

// Service is SNS's contribution to the AWS provider.
var Service = kit.Service{
	Name:      "sns",
	Scouters:  []scouter.Scouter{Topic},
	Books:     books,
	Terraform: infer.Rules{Links: []infer.Link{{Type: "aws_sns_topic_subscription", From: "topic_arn", To: []string{"endpoint"}}}},
	Actions:   kit.Actions{"aws_sns_topic": {"publish": {"sns:Publish"}}},
}
