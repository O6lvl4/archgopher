// Package pattern holds AWS architectures as L3 patterns: each takes a few
// parameters and expands into nodes read by the AWS service scouters.
package pattern

import (
	"github.com/O6lvl4/archgopher/model"
	"github.com/O6lvl4/archgopher/pattern"
	"github.com/O6lvl4/archgopher/scouter"
)

func f(v float64) *float64 { return &v }

type serverlessAPI struct {
	DurationMs       float64 `scout:"durationMs" label:"Function duration" unit:"ms"`
	MemoryMb         float64 `scout:"memoryMb" label:"Function memory" unit:"MB" default:"512"`
	ItemSizeKb       float64 `scout:"itemSizeKb" label:"Item size" unit:"KB"`
	StorageGb        float64 `scout:"storageGb" label:"Table storage" unit:"GB"`
	ReadsPerRequest  float64 `scout:"readsPerRequest" label:"Reads per request" default:"1"`
	WritesPerRequest float64 `scout:"writesPerRequest" label:"Writes per request" default:"0.2"`
}

// ServerlessAPI is API Gateway (REST) in front of a Lambda function that
// reads and writes one on-demand DynamoDB table. Load enters at the API;
// outgoing edges leave from the function.
var ServerlessAPI = pattern.Def[serverlessAPI]{
	Info: scouter.Meta{
		Type: "aws.pattern.serverless_api", Label: "Serverless API", Category: "Pattern", External: true,
		Description: "API Gateway (REST) → Lambda (arm64) → DynamoDB (on demand). Load enters at the API; edges out leave from the function.",
		Kinds:       []string{"request"},
	},
	Build: func(p serverlessAPI) pattern.Fragment {
		return pattern.Fragment{
			In: "api", Out: "fn",
			Nodes: []model.Node{
				{ID: "api", Type: "aws_api_gateway_rest_api"},
				{ID: "fn", Type: "aws_lambda_function",
					Attributes:  map[string]any{"memory_size": p.MemoryMb, "architectures": []any{"arm64"}},
					Assumptions: map[string]any{"durationMs": p.DurationMs}},
				{ID: "table", Type: "aws_dynamodb_table",
					Attributes:  map[string]any{"billing_mode": "PAY_PER_REQUEST"},
					Assumptions: map[string]any{"itemSizeKb": p.ItemSizeKb, "storageGb": p.StorageGb}},
			},
			Edges: []model.Edge{
				{From: "api", To: "fn"},
				{From: "fn", To: "table", Kind: "read", PerUnit: f(p.ReadsPerRequest)},
				{From: "fn", To: "table", Kind: "write", PerUnit: f(p.WritesPerRequest)},
			},
		}
	},
}

type queueWorker struct {
	DurationMs float64 `scout:"durationMs" label:"Duration per batch" unit:"ms"`
	MemoryMb   float64 `scout:"memoryMb" label:"Function memory" unit:"MB" default:"1024"`
	BatchSize  float64 `scout:"batchSize" label:"Messages per invocation" default:"10"`
	MessageKb  float64 `scout:"messageKb" label:"Message size" unit:"KB" default:"1"`
}

// QueueWorker is an SQS queue drained by a Lambda function in batches.
// Load enters at the queue; outgoing edges leave from the function.
var QueueWorker = pattern.Def[queueWorker]{
	Info: scouter.Meta{
		Type: "aws.pattern.queue_worker", Label: "Queue worker", Category: "Pattern", External: true,
		Description: "SQS → Lambda in batches. Load enters at the queue; edges out leave from the function, once per batch.",
		Kinds:       []string{"send"},
	},
	Build: func(p queueWorker) pattern.Fragment {
		batch := p.BatchSize
		if batch < 1 {
			batch = 1
		}
		return pattern.Fragment{
			In: "queue", Out: "worker",
			Nodes: []model.Node{
				{ID: "queue", Type: "aws_sqs_queue", Assumptions: map[string]any{"messageKb": p.MessageKb}},
				{ID: "worker", Type: "aws_lambda_function",
					Attributes:  map[string]any{"memory_size": p.MemoryMb},
					Assumptions: map[string]any{"durationMs": p.DurationMs}},
			},
			Edges: []model.Edge{{From: "queue", To: "worker", PerUnit: f(1 / batch), Note: "one invocation per batch"}},
		}
	},
}

// Registry returns the AWS patterns.
func Registry() pattern.Registry {
	reg := pattern.Registry{}
	reg.Register(ServerlessAPI, QueueWorker)
	return reg
}
