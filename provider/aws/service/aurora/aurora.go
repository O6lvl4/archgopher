// Package aurora reads Amazon Aurora Serverless v2 clusters.
package aurora

import (
	"embed"
	"strconv"

	"github.com/O6lvl4/arch-scouter/facet"
	"github.com/O6lvl4/arch-scouter/meter"
	"github.com/O6lvl4/arch-scouter/model"
	"github.com/O6lvl4/arch-scouter/provider/aws/kit"
	"github.com/O6lvl4/arch-scouter/scouter"
)

//go:embed books
var books embed.FS

type attrs struct {
	StorageType string   `scout:"storage_type" label:"Storage type" options:",aurora,aurora-iopt1" default:"" hint:"aurora-iopt1 (I/O-Optimized) has no I/O charge"`
	MinCapacity *float64 `scout:"min_capacity" path:"serverlessv2_scaling_configuration.min_capacity" label:"Min ACU"`
	MaxCapacity *float64 `scout:"max_capacity" path:"serverlessv2_scaling_configuration.max_capacity" label:"Max ACU"`
}

type assume struct {
	AverageAcu float64  `scout:"averageAcu" label:"Average ACU" hint:"Billed capacity averaged over the month"`
	PeakAcu    *float64 `scout:"peakAcu" label:"Peak ACU" hint:"Compared with max capacity"`
	StorageGb  float64  `scout:"storageGb" label:"Storage" unit:"GB"`
	IoPerQuery *float64 `scout:"ioPerQuery" label:"I/O per query" hint:"Required for standard storage"`
}

func num(v float64) string { return strconv.FormatFloat(v, 'f', -1, 64) }

// Cluster reads ACU-hours, storage and I/O.
var Cluster = scouter.Def[attrs, assume]{
	Info: scouter.Meta{
		Type: "aws_rds_cluster", Label: "Aurora Serverless v2", Category: "Database", SLA: "aws.aurora",
		Description: "ACU-hours, storage and I/O. Headroom is peak ACU against max capacity.",
		Kinds:       []string{"query"},
	},
	Run: func(a attrs, p assume, d model.Demand, r *meter.Recorder) {
		if a.MaxCapacity == nil {
			r.Fail("only Aurora Serverless v2 is supported: serverlessv2_scaling_configuration is missing")
			return
		}
		if a.MinCapacity != nil && p.AverageAcu < *a.MinCapacity {
			r.Fail("averageAcu %s is below min_capacity %s", num(p.AverageAcu), num(*a.MinCapacity))
		}
		iopt := a.StorageType == "aurora-iopt1"
		tier, storage := "serverless_v2", "aws.aurora.storage"
		if iopt {
			tier, storage = "iopt", "aws.aurora.iopt.storage"
		}
		facet.Capacity{Name: "Capacity", Unit: "ACU-hour", PriceID: "aws.aurora." + tier + ".acu_hour"}.Read(r, p.AverageAcu)
		facet.Storage{Name: "Storage", PriceID: storage}.Read(r, p.StorageGb)
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

// Service is Aurora's contribution to the AWS provider.
var Service = kit.Service{
	Name:     "aurora",
	Scouters: []scouter.Scouter{Cluster},
	Books:    books,
	Actions:  kit.Actions{"aws_rds_cluster": {"query": {"rds-data:ExecuteStatement", "rds-data:BatchExecuteStatement", "rds-db:connect"}}},
}
