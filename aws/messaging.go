package aws

import "github.com/O6lvl4/arch-scouter/scout"

type sqsAttrs struct {
	Fifo bool `scout:"fifo_queue" label:"FIFO" default:"false"`
}

type sqsAssume struct {
	MessageKb          float64 `scout:"messageKb" label:"Message size" unit:"KB" default:"1" hint:"Billed in 64 KB chunks"`
	RequestsPerMessage float64 `scout:"requestsPerMessage" label:"API requests per message" default:"3" hint:"Send, receive and delete; lower with batching"`
}

// SQS bills API requests in 64 KB chunks.
var SQS = scout.Def[sqsAttrs, sqsAssume]{
	Info: scout.Meta{
		Type: "aws_sqs_queue", Label: "SQS", Category: "Messaging", SLA: "aws.sqs",
		Description: "API requests in 64 KB chunks (send, receive, delete). FIFO queues have a per-API rate limit.",
		Kinds:       []string{"send"},
	},
	Run: func(a sqsAttrs, p sqsAssume, d scout.Demand, r *scout.Recorder) {
		in := d.Total()
		kind := "standard"
		if a.Fifo {
			kind = "fifo"
		}
		r.Cost("Requests", in.Monthly*p.RequestsPerMessage*scout.CeilDiv(p.MessageKb, 64), "request", "aws.sqs."+kind+".requests")
		if a.Fifo {
			r.Limit("FIFO send rate", in.PeakPerSecond, "messages/second", "aws.sqs.fifo.tps")
		}
	},
}

type snsAssume struct {
	MessageKb float64 `scout:"messageKb" label:"Message size" unit:"KB" default:"1" hint:"Billed in 64 KB chunks"`
}

// SNS bills publishes; delivery to Lambda and SQS is free.
var SNS = scout.Def[noAttrs, snsAssume]{
	Info: scout.Meta{
		Type: "aws_sns_topic", Label: "SNS", Category: "Messaging", SLA: "aws.sns",
		Description: "Publishes in 64 KB chunks; delivery to Lambda and SQS is free. Headroom is the regional publish rate.",
		Kinds:       []string{"publish"},
	},
	Run: func(_ noAttrs, p snsAssume, d scout.Demand, r *scout.Recorder) {
		in := d.Total()
		r.Cost("Publishes", in.Monthly*scout.CeilDiv(p.MessageKb, 64), "request", "aws.sns.publish")
		r.Limit("Publish rate", in.PeakPerSecond, "requests/second", "aws.sns.publish_rps")
	},
}

// Scheduler (EventBridge Scheduler) bills invocations. Terraform import turns
// its schedule expression into the node's load.
var Scheduler = scout.Def[noAttrs, noAssume]{
	Info: scout.Meta{
		Type: "aws_scheduler_schedule", Label: "EventBridge Scheduler", Category: "Scheduling", SLA: "aws.scheduler",
		Description: "Invocations. Its schedule expression becomes the node's load on Terraform import. Free tier is ignored.",
		Kinds:       []string{"fire"},
	},
	Run: func(_ noAttrs, _ noAssume, d scout.Demand, r *scout.Recorder) {
		r.Cost("Invocations", d.Total().Monthly, "invocation", "aws.scheduler.invocations")
	},
}

// EventRule reads an EventBridge rule. Scheduled rules are free; the schedule
// becomes the node's load on Terraform import.
var EventRule = scout.Def[noAttrs, noAssume]{
	Info: scout.Meta{
		Type: "aws_cloudwatch_event_rule", Label: "EventBridge rule", Category: "Scheduling", SLA: "aws.eventbridge",
		Description: "Scheduled rules cost nothing; the schedule becomes the node's load on Terraform import.",
		Kinds:       []string{"fire"},
	},
	Run: func(_ noAttrs, _ noAssume, _ scout.Demand, _ *scout.Recorder) {},
}
