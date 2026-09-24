// Package sqs reads Amazon SQS queues.
package sqs

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

type attrs struct {
	Fifo bool `scout:"fifo_queue" label:"FIFO" default:"false"`
}

type assume struct {
	MessageKb          float64 `scout:"messageKb" label:"Message size" unit:"KB" default:"1" hint:"Billed in 64 KB chunks"`
	RequestsPerMessage float64 `scout:"requestsPerMessage" label:"API requests per message" default:"3" hint:"Send, receive and delete; lower with batching"`
}

// Queue bills API requests in 64 KB chunks.
var Queue = scouter.Def[attrs, assume]{
	Info: scouter.Meta{
		Type: "aws_sqs_queue", Label: "SQS", Category: "Messaging", SLA: "aws.sqs",
		Description: "API requests in 64 KB chunks (send, receive, delete). FIFO queues have a per-API rate limit.",
		Kinds:       []string{"send"},
	},
	Run: func(a attrs, p assume, d model.Demand, r *meter.Recorder) {
		in := d.Total()
		kind := "standard"
		if a.Fifo {
			kind = "fifo"
		}
		facet.Requests{Name: "Requests", Unit: "request", PriceID: "aws.sqs." + kind + ".requests", ChunkKB: 64}.Read(r, in.Monthly*p.RequestsPerMessage, p.MessageKb)
		if a.Fifo {
			facet.Rate{Name: "FIFO send rate", Unit: "messages/second", QuotaID: "aws.sqs.fifo.tps"}.Read(r, in.PeakPerSecond)
		}
	},
}

// Service is SQS's contribution to the AWS provider.
var Service = kit.Service{
	Name:      "sqs",
	Scouters:  []scouter.Scouter{Queue},
	Books:     books,
	Terraform: infer.Rules{IgnoreRefs: []string{"redrive_policy", "redrive_allow_policy"}},
	Actions:   kit.Actions{"aws_sqs_queue": {"send": {"sqs:SendMessage"}}},
}
