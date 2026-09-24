package aws

import "github.com/O6lvl4/arch-scouter/scout"

type dynamoAttrs struct {
	BillingMode   string   `scout:"billing_mode" label:"Billing mode" options:"PROVISIONED,PAY_PER_REQUEST" default:"PROVISIONED"`
	ReadCapacity  *float64 `scout:"read_capacity" label:"Read capacity" unit:"RCU"`
	WriteCapacity *float64 `scout:"write_capacity" label:"Write capacity" unit:"WCU"`
}

type dynamoAssume struct {
	ItemSizeKb      float64 `scout:"itemSizeKb" label:"Item size" unit:"KB"`
	StorageGb       float64 `scout:"storageGb" label:"Storage" unit:"GB"`
	Consistency     string  `scout:"consistentRead" label:"Read consistency" options:"eventual,strong,transactional" default:"eventual"`
	TransactionalWr bool    `scout:"transactionalWrites" label:"Transactional writes" default:"false"`
}

// DynamoDB converts reads and writes into request units (4 KB and 1 KB steps).
var DynamoDB = scout.Def[dynamoAttrs, dynamoAssume]{
	Info: scout.Meta{
		Type: "aws_dynamodb_table", Label: "DynamoDB", Category: "Database", SLA: "aws.dynamodb",
		Description: "Read and write units (4 KB / 1 KB steps) and storage. Headroom is provisioned capacity or the on-demand table limit. Auto scaling is not modelled.",
		Kinds:       []string{"read", "write"},
	},
	Run: func(a dynamoAttrs, p dynamoAssume, d scout.Demand, r *scout.Recorder) {
		readFactor := map[string]float64{"eventual": 0.5, "strong": 1, "transactional": 2}[p.Consistency]
		perRead := scout.CeilDiv(p.ItemSizeKb, 4) * readFactor
		perWrite := scout.CeilDiv(p.ItemSizeKb, 1)
		if p.TransactionalWr {
			perWrite *= 2
		}
		reads, writes := d.Of("read"), d.Of("write")
		r.Cost("Storage", p.StorageGb, "GB-month", "aws.dynamodb.storage")
		if a.BillingMode == "PAY_PER_REQUEST" {
			r.Cost("Read request units", reads.Monthly*perRead, "read request unit", "aws.dynamodb.ondemand.read")
			r.Cost("Write request units", writes.Monthly*perWrite, "write request unit", "aws.dynamodb.ondemand.write")
			r.Limit("Table reads", reads.PeakPerSecond*perRead, "read units/second", "aws.dynamodb.ondemand.table_read")
			r.Limit("Table writes", writes.PeakPerSecond*perWrite, "write units/second", "aws.dynamodb.ondemand.table_write")
			return
		}
		if a.ReadCapacity == nil || a.WriteCapacity == nil {
			r.Fail("provisioned tables need read_capacity and write_capacity")
			return
		}
		r.Cost("Provisioned reads", *a.ReadCapacity*scout.HoursPerMonth, "RCU-hour", "aws.dynamodb.provisioned.rcu")
		r.Cost("Provisioned writes", *a.WriteCapacity*scout.HoursPerMonth, "WCU-hour", "aws.dynamodb.provisioned.wcu")
		r.LimitOverride("Provisioned reads", reads.PeakPerSecond*perRead, "read units/second", "", a.ReadCapacity)
		r.LimitOverride("Provisioned writes", writes.PeakPerSecond*perWrite, "write units/second", "", a.WriteCapacity)
	},
}

type s3Assume struct {
	StorageGb float64 `scout:"storageGb" label:"Storage" unit:"GB"`
	Prefixes  float64 `scout:"prefixes" label:"Prefixes the load spreads over" default:"1" hint:"Request rate limits apply per prefix"`
}

// S3 reads S3 Standard: GET, PUT and storage; headroom is the per-prefix rate.
var S3 = scout.Def[noAttrs, s3Assume]{
	Info: scout.Meta{
		Type: "aws_s3_bucket", Label: "S3", Category: "Storage", SLA: "aws.s3",
		Description: "S3 Standard GET, PUT and storage. Headroom is the per-prefix request rate times the prefixes in use.",
		Kinds:       []string{"read", "write"},
	},
	Run: func(_ noAttrs, p s3Assume, d scout.Demand, r *scout.Recorder) {
		reads, writes := d.Of("read"), d.Of("write")
		r.Cost("GET requests", reads.Monthly, "request", "aws.s3.standard.get")
		r.Cost("PUT requests", writes.Monthly, "request", "aws.s3.standard.put")
		r.Cost("Storage", p.StorageGb, "GB-month", "aws.s3.standard.storage")
		r.LimitScaled("GET rate", reads.PeakPerSecond, "requests/second", "aws.s3.prefix_get", p.Prefixes)
		r.LimitScaled("PUT rate", writes.PeakPerSecond, "requests/second", "aws.s3.prefix_put", p.Prefixes)
	},
}
