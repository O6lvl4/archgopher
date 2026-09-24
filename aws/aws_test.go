package aws

import (
	"strings"
	"testing"

	"github.com/O6lvl4/arch-scouter/scout"
)

var regions = []string{"us-east-1", "ap-northeast-1"}

// sample fills required fields with a plausible value and applies overrides.
func sample(fields []scout.Field, over map[string]any) map[string]any {
	out := map[string]any{}
	for _, f := range fields {
		if !f.Required {
			continue
		}
		switch f.Type {
		case scout.Choice:
			out[f.Key] = f.Options[0]
		case scout.Flag:
			out[f.Key] = false
		default:
			out[f.Key] = 1.0
		}
	}
	for k, v := range over {
		out[k] = v
	}
	return out
}

// attrs makes each scouter take its main code path.
var attrs = map[string]map[string]any{
	"aws_rds_cluster":       {"max_capacity": 16.0, "min_capacity": 0.5},
	"aws_dynamodb_table":    {"billing_mode": "PAY_PER_REQUEST"},
	"aws_sfn_state_machine": {"type": "STANDARD"},
}

var assume = map[string]map[string]any{
	"aws_rds_cluster":       {"ioPerQuery": 2.0, "peakAcu": 4.0},
	"aws_sfn_state_machine": {"transitionsPerExecution": 5.0},
}

// Every scouter must find every reference it reads, in the units it counts,
// in each bundled region. Unknown values are allowed; missing rows are not.
func TestEveryScouterReadsTheBooks(t *testing.T) {
	books, err := Books()
	if err != nil {
		t.Fatal(err)
	}
	reg := Registry()
	for _, typ := range reg.Types() {
		s := reg[typ]
		for _, region := range regions {
			n := scout.Node{ID: "n", Type: typ, Attributes: attrs[typ], Assumptions: sample(s.Assumptions(), assume[typ])}
			if typ == scout.EntryType {
				n.Load = &scout.Load{Monthly: 1, PeakPerSecond: 1}
			}
			d := scout.Demand{}
			for _, k := range s.Meta().Kinds {
				d[k] = scout.Load{Monthly: 1e6, PeakPerSecond: 10}
			}
			if err := s.Scout(n, d, scout.NewRecorder(region, books)); err != nil {
				t.Errorf("%s in %s: %v", typ, region, err)
			}
		}
	}
}

func TestBooksAreWellFormed(t *testing.T) {
	books, err := Books()
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []scout.BookName{scout.Prices, scout.Quotas, scout.SLAs} {
		for id, e := range books.Book(name) {
			if e.Unit == "" || !strings.HasPrefix(e.Source, "https://") || len(e.Values) == 0 {
				t.Errorf("%s %s: needs a unit, an https source and values", name, id)
			}
			if _, any := e.Values[scout.AnyRegion]; !any {
				for _, r := range regions {
					if _, ok := e.Values[r]; !ok {
						t.Errorf("%s %s: no value for %s", name, id, r)
					}
				}
			}
		}
	}
}

func TestIAMKinds(t *testing.T) {
	cases := []struct {
		target  string
		actions []string
		want    string
	}{
		{"aws_dynamodb_table", []string{"dynamodb:GetItem", "dynamodb:PutItem"}, "read,write"},
		{"aws_dynamodb_table", []string{"dynamodb:*"}, "read,write"},
		{"aws_dynamodb_table", []string{"dynamodb:Get*"}, "read"},
		{"aws_dynamodb_table", []string{"dynamodb:DescribeTable"}, ""},
		{"aws_s3_bucket", []string{"s3:PutObject"}, "write"},
		{"aws_sqs_queue", []string{"sqs:ReceiveMessage"}, ""},
		{"aws_sqs_queue", nil, ""},
		{"aws_kinesis_stream", []string{"kinesis:PutRecord"}, ""},
	}
	for _, c := range cases {
		got := strings.Join(iamKinds(c.target, c.actions), ",")
		if got != c.want {
			t.Errorf("%s %v: got %q, want %q", c.target, c.actions, got, c.want)
		}
	}
}
