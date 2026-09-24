package aws

import "github.com/O6lvl4/arch-scouter/scout"

type noAttrs struct{}
type noAssume struct{}

// RestAPI bills requests; headroom is the account-level throttle.
var RestAPI = scout.Def[noAttrs, noAssume]{
	Info: scout.Meta{
		Type: "aws_api_gateway_rest_api", Label: "API Gateway (REST)", Category: "Edge", SLA: "aws.apigateway",
		Description: "Requests; headroom is the account throttle shared by every API in the region.",
		Kinds:       []string{"request"},
	},
	Run: func(_ noAttrs, _ noAssume, d scout.Demand, r *scout.Recorder) {
		in := d.Total()
		r.Cost("Requests", in.Monthly, "request", "aws.apigateway.rest.requests")
		r.Limit("Account throttle", in.PeakPerSecond, "requests/second", "aws.apigateway.account_rps")
	},
}

type httpAPIAttrs struct {
	Protocol string `scout:"protocol_type" label:"Protocol" options:"HTTP,WEBSOCKET" default:"HTTP"`
}

type httpAPIAssume struct {
	PayloadKb float64 `scout:"payloadKb" label:"Request size" unit:"KB" default:"1" hint:"Billed in 512 KB steps"`
}

// HTTPAPI bills requests in 512 KB steps.
var HTTPAPI = scout.Def[httpAPIAttrs, httpAPIAssume]{
	Info: scout.Meta{
		Type: "aws_apigatewayv2_api", Label: "API Gateway (HTTP)", Category: "Edge", SLA: "aws.apigateway",
		Description: "Requests in 512 KB steps; headroom is the account throttle. WebSocket APIs are not read yet.",
		Kinds:       []string{"request"},
	},
	Run: func(a httpAPIAttrs, p httpAPIAssume, d scout.Demand, r *scout.Recorder) {
		if a.Protocol != "HTTP" {
			r.Fail("%s APIs are not supported yet", a.Protocol)
			return
		}
		in := d.Total()
		r.Cost("Requests", in.Monthly*scout.CeilDiv(p.PayloadKb, 512), "request", "aws.apigateway.http.requests")
		r.Limit("Account throttle", in.PeakPerSecond, "requests/second", "aws.apigateway.account_rps")
	},
}

type cloudFrontAssume struct {
	ResponseKb float64 `scout:"responseKb" label:"Response size" unit:"KB"`
}

// CloudFront bills HTTPS requests and data transfer out. Prices follow the
// viewer's location; the region's own edge location is assumed.
var CloudFront = scout.Def[noAttrs, cloudFrontAssume]{
	Info: scout.Meta{
		Type: "aws_cloudfront_distribution", Label: "CloudFront", Category: "Edge", SLA: "aws.cloudfront",
		Description: "HTTPS requests and transfer out, priced at the edge nearest the spec region. Free tier is ignored.",
		Kinds:       []string{"request"},
	},
	Run: func(_ noAttrs, p cloudFrontAssume, d scout.Demand, r *scout.Recorder) {
		in := d.Total()
		r.Cost("HTTPS requests", in.Monthly, "request", "aws.cloudfront.https_requests")
		r.Cost("Transfer out", in.Monthly*p.ResponseKb/1024/1024, "GB", "aws.cloudfront.transfer_out")
		r.Limit("Requests per distribution", in.PeakPerSecond, "requests/second", "aws.cloudfront.distribution_rps")
	},
}
