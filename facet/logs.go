package facet

import "github.com/O6lvl4/archgopher/meter"

// LogAssume is embedded in a scouter's assumptions to count the logs each
// call writes.
type LogAssume struct {
	LogKb            float64 `scout:"logKbPerCall" label:"Log per call" unit:"KB" default:"0" hint:"0 means logs are not counted"`
	LogRetentionDays float64 `scout:"logRetentionDays" label:"Log retention" unit:"days" default:"30" hint:"0 means never expire; storage is then shown after 12 months"`
}

// Logs prices ingestion and the storage that retention keeps.
type Logs struct {
	IngestPriceID  string
	StoragePriceID string
}

// Read records the logs of count calls.
func (f Logs) Read(r *meter.Recorder, count float64, a LogAssume) {
	ingestGB := count * a.LogKb / 1024 / 1024
	if ingestGB <= 0 {
		return
	}
	r.Cost("Log ingestion", ingestGB, "GB", f.IngestPriceID)
	months, name := a.LogRetentionDays/30.4, "Log storage"
	if a.LogRetentionDays == 0 {
		months, name = 12, "Log storage (after 12 months, never expires)"
	}
	r.Cost(name, round(ingestGB*months), "GB-month", f.StoragePriceID)
}
