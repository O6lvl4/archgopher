// Package stepfunctions reads AWS Step Functions state machines.
package stepfunctions

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

type attrs struct {
	Type string `scout:"type" label:"Workflow type" options:"STANDARD,EXPRESS" default:"STANDARD"`
}

type assume struct {
	Transitions *float64 `scout:"transitionsPerExecution" label:"State transitions per execution" hint:"Standard workflows bill per transition"`
	DurationMs  *float64 `scout:"durationMs" label:"Duration per execution" unit:"ms" hint:"Express workflows bill GB-seconds"`
	MemoryMb    float64  `scout:"memoryMb" label:"Memory per execution" unit:"MB" default:"64" hint:"Express bills in 64 MB steps"`
}

// StateMachine bills transitions (standard) or requests and GB-seconds (express).
var StateMachine = scouter.Def[attrs, assume]{
	Info: scouter.Meta{
		Type: "aws_sfn_state_machine", Label: "Step Functions", Category: "Compute", SLA: "aws.sfn",
		Description: "Standard bills state transitions; express bills requests and GB-seconds. Headroom is the StartExecution rate.",
		Kinds:       []string{"execution"},
	},
	Run: func(a attrs, p assume, d model.Demand, r *meter.Recorder) {
		in := d.Total()
		if a.Type == "EXPRESS" {
			if p.DurationMs == nil {
				r.Fail("durationMs is required for express workflows")
				return
			}
			facet.Requests{Name: "Requests", Unit: "request", PriceID: "aws.sfn.express.requests"}.Read(r, in.Monthly, 0)
			facet.Compute{Name: "Duration", PriceID: "aws.sfn.express.gb_second", StepMB: 64}.Read(r, in.Monthly, *p.DurationMs/1000, p.MemoryMb)
			return
		}
		if p.Transitions == nil {
			r.Fail("transitionsPerExecution is required for standard workflows")
			return
		}
		r.Cost("State transitions", in.Monthly**p.Transitions, "state transition", "aws.sfn.standard.transitions")
		facet.Rate{Name: "StartExecution rate", Unit: "requests/second", QuotaID: "aws.sfn.standard.start_execution"}.Read(r, in.PeakPerSecond)
	},
}

// Service is Step Functions' contribution to the AWS provider.
var Service = kit.Service{
	Name:     "stepfunctions",
	Scouters: []scouter.Scouter{StateMachine},
	Books:    books,
	Actions:  kit.Actions{"aws_sfn_state_machine": {"execution": {"states:StartExecution", "states:StartSyncExecution"}}},
}
