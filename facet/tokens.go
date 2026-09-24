package facet

import (
	"github.com/O6lvl4/archgopher/book"
	"github.com/O6lvl4/archgopher/meter"
)

// TokenAssume is embedded in a model scouter's assumptions.
type TokenAssume struct {
	InputTokens     float64 `scout:"inputTokensPerCall" label:"Input tokens per call" unit:"tokens" hint:"Include the system prompt and history"`
	OutputTokens    float64 `scout:"outputTokensPerCall" label:"Output tokens per call" unit:"tokens"`
	CacheReadShare  float64 `scout:"cacheReadShare" label:"Share of input read from cache" default:"0"`
	CacheWriteShare float64 `scout:"cacheWriteShare" label:"Share of input written to cache" default:"0"`
}

// Tokens prices model calls by token class and checks tokens and requests
// per minute. Ids are Prefix + ".input", ".output", ".cache_read",
// ".cache_write", ".tpm", ".rpm" and ".output_burndown".
type Tokens struct {
	Prefix string
}

// Read records calls at a monthly count and a peak rate.
func (f Tokens) Read(r *meter.Recorder, monthly, peakPerSecond float64, a TokenAssume) {
	if a.CacheReadShare < 0 || a.CacheWriteShare < 0 || a.CacheReadShare+a.CacheWriteShare > 1 {
		r.Fail("cache shares must be between 0 and 1 and add up to at most 1")
		return
	}
	plain := 1 - a.CacheReadShare - a.CacheWriteShare
	r.Cost("Input tokens", monthly*a.InputTokens*plain, "token", f.Prefix+".input")
	if a.CacheWriteShare > 0 {
		r.Cost("Cache write tokens", monthly*a.InputTokens*a.CacheWriteShare, "token", f.Prefix+".cache_write")
	}
	if a.CacheReadShare > 0 {
		r.Cost("Cache read tokens", monthly*a.InputTokens*a.CacheReadShare, "token", f.Prefix+".cache_read")
	}
	r.Cost("Output tokens", monthly*a.OutputTokens, "token", f.Prefix+".output")
	burndown := 1.0
	if b := r.Ref(book.Quotas, f.Prefix+".output_burndown", "multiplier"); b != nil {
		burndown = *b
	}
	perMinute := peakPerSecond * 60
	// Cache reads do not count against tokens per minute; output counts burndown times.
	counted := a.InputTokens*(1-a.CacheReadShare) + a.OutputTokens*burndown
	r.Limit("Tokens per minute", perMinute*counted, "tokens/minute", f.Prefix+".tpm")
	r.Limit("Requests per minute", perMinute, "requests/minute", f.Prefix+".rpm")
}
