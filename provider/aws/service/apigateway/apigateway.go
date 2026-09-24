// Package apigateway reads Amazon API Gateway REST and HTTP APIs.
package apigateway

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

var throttle = facet.Rate{Name: "Account throttle", Unit: "requests/second", QuotaID: "aws.apigateway.account_rps"}

// REST bills requests; headroom is the account-level throttle.
var REST = scouter.Def[none, none]{
	Info: scouter.Meta{
		Type: "aws_api_gateway_rest_api", Label: "API Gateway (REST)", Category: "Edge", SLA: "aws.apigateway",
		Description: "Requests; headroom is the account throttle shared by every API in the region.",
		Kinds:       []string{"request"},
	},
	Run: func(_ none, _ none, d model.Demand, r *meter.Recorder) {
		in := d.Total()
		facet.Requests{Name: "Requests", Unit: "request", PriceID: "aws.apigateway.rest.requests"}.Read(r, in.Monthly, 0)
		throttle.Read(r, in.PeakPerSecond)
	},
}

type httpAttrs struct {
	Protocol string `scout:"protocol_type" label:"Protocol" options:"HTTP,WEBSOCKET" default:"HTTP"`
}

type httpAssume struct {
	PayloadKb float64 `scout:"payloadKb" label:"Request size" unit:"KB" default:"1" hint:"Billed in 512 KB steps"`
}

// HTTP bills requests in 512 KB steps.
var HTTP = scouter.Def[httpAttrs, httpAssume]{
	Info: scouter.Meta{
		Type: "aws_apigatewayv2_api", Label: "API Gateway (HTTP)", Category: "Edge", SLA: "aws.apigateway",
		Description: "Requests in 512 KB steps; headroom is the account throttle. WebSocket APIs are not read yet.",
		Kinds:       []string{"request"},
	},
	Run: func(a httpAttrs, p httpAssume, d model.Demand, r *meter.Recorder) {
		if a.Protocol != "HTTP" {
			r.Fail("%s APIs are not supported yet", a.Protocol)
			return
		}
		in := d.Total()
		facet.Requests{Name: "Requests", Unit: "request", PriceID: "aws.apigateway.http.requests", ChunkKB: 512}.Read(r, in.Monthly, p.PayloadKb)
		throttle.Read(r, in.PeakPerSecond)
	},
}

// Service is API Gateway's contribution to the AWS provider.
var Service = kit.Service{
	Name:     "apigateway",
	Scouters: []scouter.Scouter{REST, HTTP},
	Books:    books,
	Terraform: infer.Rules{
		Links: []infer.Link{
			{Type: "aws_api_gateway_integration", From: "rest_api_id", To: []string{"uri"}},
			{Type: "aws_apigatewayv2_integration", From: "api_id", To: []string{"integration_uri"}},
		},
		Aliases: map[string]string{
			"aws_api_gateway_stage":      "rest_api_id",
			"aws_api_gateway_deployment": "rest_api_id",
			"aws_apigatewayv2_stage":     "api_id",
		},
		FrontDoors: map[string]bool{"aws_api_gateway_rest_api": true, "aws_apigatewayv2_api": true},
	},
}
