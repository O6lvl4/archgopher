// Package lambda reads AWS Lambda: requests, GB-seconds, concurrency and logs.
package lambda

import (
	"embed"
	"strconv"

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
	MemorySize    float64  `scout:"memory_size" label:"Memory" unit:"MB" default:"128"`
	Architectures []string `scout:"architectures" label:"Architecture" options:"x86_64,arm64" default:"x86_64" hint:"arm64 has a lower GB-second price"`
	Reserved      *float64 `scout:"reserved_concurrent_executions" label:"Reserved concurrency" hint:"Empty or -1 uses the account pool"`
}

type assume struct {
	DurationMs float64 `scout:"durationMs" label:"Duration per invocation" unit:"ms"`
	facet.LogAssume
}

var (
	requests    = facet.Requests{Name: "Requests", Unit: "request", PriceID: "aws.lambda.requests"}
	concurrency = facet.Concurrency{Name: "Concurrency", Unit: "concurrent executions", QuotaID: "aws.lambda.concurrent_executions"}
	logs        = facet.Logs{IngestPriceID: "aws.logs.ingest", StoragePriceID: "aws.logs.storage"}
)

// Function bills requests and GB-seconds; concurrency is peak rate × duration.
var Function = scouter.Def[attrs, assume]{
	Info: scouter.Meta{
		Type: "aws_lambda_function", Label: "Lambda", Category: "Compute", SLA: "aws.lambda",
		Description: "Requests and GB-seconds; headroom is concurrency (peak rate × duration). Logs are counted when set.",
		Kinds:       []string{"invoke"},
	},
	Run: func(a attrs, p assume, d model.Demand, r *meter.Recorder) {
		in := d.Total()
		arch := "x86_64"
		if len(a.Architectures) > 0 {
			arch = a.Architectures[0]
		}
		seconds := p.DurationMs / 1000
		requests.Read(r, in.Monthly, 0)
		compute := facet.Compute{Name: "Duration (" + strconv.FormatFloat(a.MemorySize, 'f', -1, 64) + " MB, " + arch + ")", PriceID: "aws.lambda." + arch + ".gb_second"}
		compute.Read(r, in.Monthly, seconds, a.MemorySize)
		var reserved *float64
		if a.Reserved != nil && *a.Reserved >= 0 {
			reserved = a.Reserved
		}
		concurrency.Read(r, in.PeakPerSecond, seconds, reserved)
		logs.Read(r, in.Monthly, p.LogAssume)
	},
}

// Service is Lambda's contribution to the AWS provider.
var Service = kit.Service{
	Name:     "lambda",
	Scouters: []scouter.Scouter{Function},
	Books:    books,
	Terraform: infer.Rules{
		Links:            []infer.Link{{Type: "aws_lambda_event_source_mapping", From: "event_source_arn", To: []string{"function_name"}}},
		Aliases:          map[string]string{"aws_lambda_alias": "function_name"},
		FrontDoorAliases: map[string]string{"aws_lambda_function_url": "function_name"},
		IgnoreRefs:       []string{"layers", "vpc_config", "dead_letter_config", "logging_config", "tracing_config"},
	},
	Actions: kit.Actions{"aws_lambda_function": {"invoke": {"lambda:InvokeFunction"}}},
}
