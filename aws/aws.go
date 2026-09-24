// Package aws holds the AWS scouters and the reference books they read.
package aws

import (
	"embed"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/O6lvl4/arch-scouter/scout"
)

//go:embed books/*.json
var bookFiles embed.FS

// Registry returns every AWS scouter plus the provider-neutral entry.
func Registry() scout.Registry {
	reg := scout.Registry{}
	reg.Register(
		scout.EntryScouter,
		Lambda, StepFunctions, Aurora,
		RestAPI, HTTPAPI, CloudFront,
		DynamoDB, S3,
		SQS, SNS, Scheduler, EventRule,
		BedrockModel,
	)
	return reg
}

// Books loads the bundled reference books.
func Books() (scout.Books, error) {
	var b scout.Books
	for name, dst := range map[string]*scout.Book{"prices": &b.Prices, "quotas": &b.Quotas, "slas": &b.SLAs} {
		data, err := bookFiles.ReadFile("books/" + name + ".json")
		if err != nil {
			return b, err
		}
		if err := json.Unmarshal(data, dst); err != nil {
			return b, fmt.Errorf("books/%s.json: %w", name, err)
		}
	}
	return b, nil
}

// BookBytes returns a bundled book file as stored, for tools that rewrite it.
func BookBytes(name string) ([]byte, error) { return bookFiles.ReadFile("books/" + name + ".json") }

func logs(r *scout.Recorder, ingestGB, retentionDays float64) {
	if ingestGB <= 0 {
		return
	}
	r.Cost("Log ingestion", ingestGB, "GB", "aws.logs.ingest")
	months, name := retentionDays/30.4, "Log storage"
	if retentionDays == 0 {
		months, name = 12, "Log storage (after 12 months, never expires)"
	}
	r.Cost(name, ingestGB*months, "GB-month", "aws.logs.storage")
}

func fmtNum(v float64) string { return strconv.FormatFloat(v, 'f', -1, 64) }
