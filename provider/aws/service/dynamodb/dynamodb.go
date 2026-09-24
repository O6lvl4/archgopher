// Package dynamodb reads Amazon DynamoDB tables.
package dynamodb

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

type attrs struct {
	BillingMode   string   `scout:"billing_mode" label:"Billing mode" options:"PROVISIONED,PAY_PER_REQUEST" default:"PROVISIONED"`
	ReadCapacity  *float64 `scout:"read_capacity" label:"Read capacity" unit:"RCU"`
	WriteCapacity *float64 `scout:"write_capacity" label:"Write capacity" unit:"WCU"`
}

type assume struct {
	ItemSizeKb      float64 `scout:"itemSizeKb" label:"Item size" unit:"KB"`
	StorageGb       float64 `scout:"storageGb" label:"Storage" unit:"GB"`
	Consistency     string  `scout:"consistentRead" label:"Read consistency" options:"eventual,strong,transactional" default:"eventual"`
	TransactionalWr bool    `scout:"transactionalWrites" label:"Transactional writes" default:"false"`
}

var readFactor = map[string]float64{"eventual": 0.5, "strong": 1, "transactional": 2}

// Table converts reads and writes into request units (4 KB and 1 KB steps).
var Table = scouter.Def[attrs, assume]{
	Info: scouter.Meta{
		Type: "aws_dynamodb_table", Label: "DynamoDB", Category: "Database", SLA: "aws.dynamodb",
		Description: "Read and write units (4 KB / 1 KB steps) and storage. Headroom is provisioned capacity or the on-demand table limit. Auto scaling is not modelled.",
		Kinds:       []string{"read", "write"},
	},
	Run: func(a attrs, p assume, d model.Demand, r *meter.Recorder) {
		perRead := meter.CeilDiv(p.ItemSizeKb, 4) * readFactor[p.Consistency]
		perWrite := meter.CeilDiv(p.ItemSizeKb, 1)
		if p.TransactionalWr {
			perWrite *= 2
		}
		reads, writes := d.Of("read"), d.Of("write")
		facet.Storage{Name: "Storage", PriceID: "aws.dynamodb.storage"}.Read(r, p.StorageGb)
		if a.BillingMode == "PAY_PER_REQUEST" {
			r.Cost("Read request units", reads.Monthly*perRead, "read request unit", "aws.dynamodb.ondemand.read")
			r.Cost("Write request units", writes.Monthly*perWrite, "write request unit", "aws.dynamodb.ondemand.write")
			facet.Rate{Name: "Table reads", Unit: "read units/second", QuotaID: "aws.dynamodb.ondemand.table_read"}.Read(r, reads.PeakPerSecond*perRead)
			facet.Rate{Name: "Table writes", Unit: "write units/second", QuotaID: "aws.dynamodb.ondemand.table_write"}.Read(r, writes.PeakPerSecond*perWrite)
			return
		}
		if a.ReadCapacity == nil || a.WriteCapacity == nil {
			r.Fail("provisioned tables need read_capacity and write_capacity")
			return
		}
		facet.Capacity{Name: "Provisioned reads", Unit: "RCU-hour", PriceID: "aws.dynamodb.provisioned.rcu"}.Read(r, *a.ReadCapacity)
		facet.Capacity{Name: "Provisioned writes", Unit: "WCU-hour", PriceID: "aws.dynamodb.provisioned.wcu"}.Read(r, *a.WriteCapacity)
		r.LimitOverride("Provisioned reads", reads.PeakPerSecond*perRead, "read units/second", "", a.ReadCapacity)
		r.LimitOverride("Provisioned writes", writes.PeakPerSecond*perWrite, "write units/second", "", a.WriteCapacity)
	},
}

// Service is DynamoDB's contribution to the AWS provider.
var Service = kit.Service{
	Name:     "dynamodb",
	Scouters: []scouter.Scouter{Table},
	Books:    books,
	Terraform: infer.Rules{
		IgnoreRefs: []string{"replica"},
	},
	Actions: kit.Actions{"aws_dynamodb_table": {
		"read":  {"dynamodb:GetItem", "dynamodb:BatchGetItem", "dynamodb:Query", "dynamodb:Scan", "dynamodb:ConditionCheckItem"},
		"write": {"dynamodb:PutItem", "dynamodb:UpdateItem", "dynamodb:DeleteItem", "dynamodb:BatchWriteItem"},
	}},
}
