package aws

import "github.com/O6lvl4/arch-scouter/scout"

type bedrockAssume struct {
	Model           string  `scout:"model" label:"Model" options:"claude-sonnet-4-5,claude-haiku-4-5,claude-opus-4-5"`
	InputTokens     float64 `scout:"inputTokensPerCall" label:"Input tokens per call" unit:"tokens" hint:"Include the system prompt and history"`
	OutputTokens    float64 `scout:"outputTokensPerCall" label:"Output tokens per call" unit:"tokens"`
	CacheReadShare  float64 `scout:"cacheReadShare" label:"Share of input read from cache" default:"0"`
	CacheWriteShare float64 `scout:"cacheWriteShare" label:"Share of input written to cache" default:"0"`
}

// BedrockModel reads on-demand model invocations. It is not a Terraform
// resource: place it by hand and connect callers to it.
var BedrockModel = scout.Def[noAttrs, bedrockAssume]{
	Info: scout.Meta{
		Type: "bedrock_model", Label: "Bedrock model", Category: "AI", SLA: "aws.bedrock", External: true,
		Description: "Input, output and prompt-cache tokens. Headroom is tokens and requests per minute: output counts with the burndown multiplier, cache reads do not count.",
		Kinds:       []string{"call"},
	},
	Run: func(_ noAttrs, p bedrockAssume, d scout.Demand, r *scout.Recorder) {
		if p.CacheReadShare < 0 || p.CacheWriteShare < 0 || p.CacheReadShare+p.CacheWriteShare > 1 {
			r.Fail("cache shares must be between 0 and 1 and add up to at most 1")
			return
		}
		in := d.Total()
		id := "aws.bedrock." + p.Model
		plain := 1 - p.CacheReadShare - p.CacheWriteShare
		r.Cost("Input tokens", in.Monthly*p.InputTokens*plain, "token", id+".input")
		if p.CacheWriteShare > 0 {
			r.Cost("Cache write tokens", in.Monthly*p.InputTokens*p.CacheWriteShare, "token", id+".cache_write")
		}
		if p.CacheReadShare > 0 {
			r.Cost("Cache read tokens", in.Monthly*p.InputTokens*p.CacheReadShare, "token", id+".cache_read")
		}
		r.Cost("Output tokens", in.Monthly*p.OutputTokens, "token", id+".output")
		burndown := 1.0
		if b := r.Ref(scout.Quotas, id+".output_burndown", "multiplier"); b != nil {
			burndown = *b
		}
		perMinute := in.PeakPerSecond * 60
		// Cache reads do not count against tokens per minute; output counts burndown times.
		counted := p.InputTokens*(1-p.CacheReadShare) + p.OutputTokens*burndown
		r.Limit("Tokens per minute", perMinute*counted, "tokens/minute", id+".tpm")
		r.Limit("Requests per minute", perMinute, "requests/minute", id+".rpm")
	},
}
