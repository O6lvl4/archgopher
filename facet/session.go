package facet

import "github.com/O6lvl4/archgopher/meter"

// SessionAssume is embedded by resources billed as sessions of microVM
// compute (AgentCore Runtime, Code Interpreter, Browser).
type SessionAssume struct {
	SessionSeconds    float64 `scout:"sessionSeconds" label:"Session length" unit:"s" hint:"From start to end, idle time included"`
	ActiveVcpuSeconds float64 `scout:"activeVcpuSeconds" label:"Active CPU per session" unit:"vCPU-s" hint:"CPU actually used; waiting on models and tools is free"`
	PeakMemoryGb      float64 `scout:"peakMemoryGb" label:"Peak memory" unit:"GB" hint:"Billed for the whole session; 0.125 GB minimum"`
}

// Session prices session compute: vCPU-hours for active CPU only, GB-hours
// of peak memory for the whole session (128 MB minimum), and checks
// concurrent sessions with Little's law.
type Session struct {
	VcpuPriceID   string
	MemoryPriceID string
	// ConcurrentQuotaID is the quota on sessions alive at once.
	ConcurrentQuotaID string
}

// minMemoryGb is the smallest memory a session is billed for.
const minMemoryGb = 0.125

// Read records count sessions starting at peak per second.
func (f Session) Read(r *meter.Recorder, count, peak float64, a SessionAssume) {
	mem := a.PeakMemoryGb
	if mem < minMemoryGb {
		mem = minMemoryGb
	}
	r.Cost("Active CPU", count*a.ActiveVcpuSeconds/3600, "vCPU-hour", f.VcpuPriceID)
	r.Cost("Memory", count*a.SessionSeconds/3600*mem, "GB-hour", f.MemoryPriceID)
	r.Limit("Concurrent sessions", peak*a.SessionSeconds, "sessions", f.ConcurrentQuotaID)
}
