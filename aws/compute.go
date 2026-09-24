package aws

import "github.com/O6lvl4/arch-scouter/scout"

type lambdaAttrs struct {
	MemorySize    float64  `scout:"memory_size" label:"Memory" unit:"MB" default:"128"`
	Architectures []string `scout:"architectures" label:"Architecture" options:"x86_64,arm64" default:"x86_64" hint:"arm64 has a lower GB-second price"`
	Reserved      *float64 `scout:"reserved_concurrent_executions" label:"Reserved concurrency" hint:"Empty or -1 uses the account pool"`
}

type lambdaAssume struct {
	DurationMs       float64 `scout:"durationMs" label:"Duration per invocation" unit:"ms"`
	LogKb            float64 `scout:"logKbPerInvocation" label:"Log per invocation" unit:"KB" default:"0"`
	LogRetentionDays float64 `scout:"logRetentionDays" label:"Log retention" unit:"days" default:"30" hint:"0 means never expire; storage is then shown after 12 months"`
}

// Lambda bills requests and GB-seconds. Concurrency follows Little's law:
// peak rate × duration.
var Lambda = scout.Def[lambdaAttrs, lambdaAssume]{
	Info: scout.Meta{
		Type: "aws_lambda_function", Label: "Lambda", Category: "Compute", SLA: "aws.lambda",
		Description: "Requests and GB-seconds; headroom is concurrency (peak rate × duration). Logs are counted when set.",
		Kinds:       []string{"invoke"},
	},
	Run: func(a lambdaAttrs, p lambdaAssume, d scout.Demand, r *scout.Recorder) {
		in := d.Total()
		arch := "x86_64"
		if len(a.Architectures) > 0 {
			arch = a.Architectures[0]
		}
		seconds := p.DurationMs / 1000
		r.Cost("Requests", in.Monthly, "request", "aws.lambda.requests")
		r.Cost("Duration ("+fmtNum(a.MemorySize)+" MB, "+arch+")", in.Monthly*seconds*a.MemorySize/1024, "GB-second", "aws.lambda."+arch+".gb_second")
		var reserved *float64
		if a.Reserved != nil && *a.Reserved >= 0 {
			reserved = a.Reserved
		}
		r.LimitOverride("Concurrency", in.PeakPerSecond*seconds, "concurrent executions", "aws.lambda.concurrent_executions", reserved)
		logs(r, in.Monthly*p.LogKb/1024/1024, p.LogRetentionDays)
	},
}

type sfnAttrs struct {
	Type string `scout:"type" label:"Workflow type" options:"STANDARD,EXPRESS" default:"STANDARD"`
}

type sfnAssume struct {
	Transitions *float64 `scout:"transitionsPerExecution" label:"State transitions per execution" hint:"Standard workflows bill per transition"`
	DurationMs  *float64 `scout:"durationMs" label:"Duration per execution" unit:"ms" hint:"Express workflows bill GB-seconds"`
	MemoryMb    float64  `scout:"memoryMb" label:"Memory per execution" unit:"MB" default:"64" hint:"Express bills in 64 MB steps"`
}

// StepFunctions bills transitions (standard) or requests and GB-seconds (express).
var StepFunctions = scout.Def[sfnAttrs, sfnAssume]{
	Info: scout.Meta{
		Type: "aws_sfn_state_machine", Label: "Step Functions", Category: "Compute", SLA: "aws.sfn",
		Description: "Standard bills state transitions; express bills requests and GB-seconds. Headroom is the StartExecution rate.",
		Kinds:       []string{"execution"},
	},
	Run: func(a sfnAttrs, p sfnAssume, d scout.Demand, r *scout.Recorder) {
		in := d.Total()
		if a.Type == "EXPRESS" {
			if p.DurationMs == nil {
				r.Fail("durationMs is required for express workflows")
				return
			}
			gb := scout.CeilDiv(p.MemoryMb, 64) * 64 / 1024
			r.Cost("Requests", in.Monthly, "request", "aws.sfn.express.requests")
			r.Cost("Duration", in.Monthly**p.DurationMs/1000*gb, "GB-second", "aws.sfn.express.gb_second")
			return
		}
		if p.Transitions == nil {
			r.Fail("transitionsPerExecution is required for standard workflows")
			return
		}
		r.Cost("State transitions", in.Monthly**p.Transitions, "state transition", "aws.sfn.standard.transitions")
		r.Limit("StartExecution rate", in.PeakPerSecond, "requests/second", "aws.sfn.standard.start_execution")
	},
}

type auroraAttrs struct {
	StorageType string   `scout:"storage_type" label:"Storage type" options:",aurora,aurora-iopt1" default:"" hint:"aurora-iopt1 (I/O-Optimized) has no I/O charge"`
	MinCapacity *float64 `scout:"min_capacity" path:"serverlessv2_scaling_configuration.min_capacity" label:"Min ACU"`
	MaxCapacity *float64 `scout:"max_capacity" path:"serverlessv2_scaling_configuration.max_capacity" label:"Max ACU"`
}

type auroraAssume struct {
	AverageAcu float64  `scout:"averageAcu" label:"Average ACU" hint:"Billed capacity averaged over the month"`
	PeakAcu    *float64 `scout:"peakAcu" label:"Peak ACU" hint:"Compared with max capacity"`
	StorageGb  float64  `scout:"storageGb" label:"Storage" unit:"GB"`
	IoPerQuery *float64 `scout:"ioPerQuery" label:"I/O per query" hint:"Required for standard storage"`
}

// Aurora reads a Serverless v2 cluster: ACU-hours, storage and I/O.
var Aurora = scout.Def[auroraAttrs, auroraAssume]{
	Info: scout.Meta{
		Type: "aws_rds_cluster", Label: "Aurora Serverless v2", Category: "Database", SLA: "aws.aurora",
		Description: "ACU-hours, storage and I/O. Headroom is peak ACU against max capacity.",
		Kinds:       []string{"query"},
	},
	Run: func(a auroraAttrs, p auroraAssume, d scout.Demand, r *scout.Recorder) {
		if a.MaxCapacity == nil {
			r.Fail("only Aurora Serverless v2 is supported: serverlessv2_scaling_configuration is missing")
			return
		}
		if a.MinCapacity != nil && p.AverageAcu < *a.MinCapacity {
			r.Fail("averageAcu %s is below min_capacity %s", fmtNum(p.AverageAcu), fmtNum(*a.MinCapacity))
		}
		iopt := a.StorageType == "aurora-iopt1"
		tier := "serverless_v2"
		storage := "aws.aurora.storage"
		if iopt {
			tier, storage = "iopt", "aws.aurora.iopt.storage"
		}
		r.Cost("Capacity", p.AverageAcu*scout.HoursPerMonth, "ACU-hour", "aws.aurora."+tier+".acu_hour")
		r.Cost("Storage", p.StorageGb, "GB-month", storage)
		if !iopt {
			if p.IoPerQuery == nil {
				r.Fail("ioPerQuery is required for standard storage")
			} else {
				r.Cost("I/O", d.Total().Monthly**p.IoPerQuery, "I/O request", "aws.aurora.io")
			}
		}
		if p.PeakAcu != nil {
			r.LimitOverride("ACU", *p.PeakAcu, "ACU", "", a.MaxCapacity)
		}
	},
}
