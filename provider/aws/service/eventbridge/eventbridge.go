// Package eventbridge reads EventBridge rules and EventBridge Scheduler
// schedules, and turns their rate() and cron() expressions into load.
package eventbridge

import (
	"embed"

	"github.com/O6lvl4/arch-scouter/meter"
	"github.com/O6lvl4/arch-scouter/model"
	"github.com/O6lvl4/arch-scouter/provider/aws/kit"
	"github.com/O6lvl4/arch-scouter/scouter"
	"github.com/O6lvl4/arch-scouter/terraform/infer"
)

//go:embed books
var books embed.FS

type none struct{}

// Schedule bills invocations. Terraform import turns its expression into load.
var Schedule = scouter.Def[none, none]{
	Info: scouter.Meta{
		Type: "aws_scheduler_schedule", Label: "EventBridge Scheduler", Category: "Scheduling", SLA: "aws.scheduler",
		Description: "Invocations. Its schedule expression becomes the node's load on Terraform import. Free tier is ignored.",
		Kinds:       []string{"fire"},
	},
	Run: func(_ none, _ none, d model.Demand, r *meter.Recorder) {
		r.Cost("Invocations", d.Total().Monthly, "invocation", "aws.scheduler.invocations")
	},
}

// Rule reads an EventBridge rule. Scheduled rules are free.
var Rule = scouter.Def[none, none]{
	Info: scouter.Meta{
		Type: "aws_cloudwatch_event_rule", Label: "EventBridge rule", Category: "Scheduling", SLA: "aws.eventbridge",
		Description: "Scheduled rules cost nothing; the schedule becomes the node's load on Terraform import.",
		Kinds:       []string{"fire"},
	},
	Run: func(_ none, _ none, _ model.Demand, _ *meter.Recorder) {},
}

// Service is EventBridge's contribution to the AWS provider.
var Service = kit.Service{
	Name:     "eventbridge",
	Scouters: []scouter.Scouter{Schedule, Rule},
	Books:    books,
	Terraform: infer.Rules{
		Links: []infer.Link{{Type: "aws_cloudwatch_event_target", From: "rule", To: []string{"arn"}}},
		Schedules: map[string]string{
			"aws_scheduler_schedule":    "schedule_expression",
			"aws_cloudwatch_event_rule": "schedule_expression",
		},
		ScheduleLoad: Load,
		IgnoreRefs:   []string{"target.role_arn", "target.dead_letter_config", "target.retry_policy", "event_bus_name"},
	},
}
