package aws

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/O6lvl4/archgopher/book"
	"github.com/O6lvl4/archgopher/definition"
	"github.com/O6lvl4/archgopher/field"
	"github.com/O6lvl4/archgopher/meter"
	"github.com/O6lvl4/archgopher/model"
	"github.com/O6lvl4/archgopher/scouter"
)

var regions = []string{"us-east-1", "ap-northeast-1"}

// sample fills required fields with a plausible value and applies overrides.
func sample(fields []field.Field, over map[string]any) map[string]any {
	out := map[string]any{}
	for _, f := range fields {
		if !f.Required {
			continue
		}
		switch f.Type {
		case field.Choice:
			out[f.Key] = f.Options[0]
		case field.Flag:
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
			n := model.Node{ID: "n", Type: typ, Attributes: attrs[typ], Assumptions: sample(s.Assumptions(), assume[typ])}
			if typ == scouter.EntryType {
				n.Load = &model.Load{Monthly: 1, PeakPerSecond: 1}
			}
			d := model.Demand{}
			for _, k := range s.Meta().Kinds {
				d[k] = model.Load{Monthly: 1e6, PeakPerSecond: 10}
			}
			if err := s.Scout(n, d, meter.NewRecorder(region, books)); err != nil {
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
	for _, name := range []book.Name{book.Prices, book.Quotas, book.SLAs} {
		for id, e := range books.Book(name) {
			if e.Unit == "" || !strings.HasPrefix(e.Source, "https://") || len(e.Values) == 0 {
				t.Errorf("%s %s: needs a unit, an https source and values", name, id)
			}
			if _, any := e.Values[book.AnyRegion]; !any {
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
		got := strings.Join(IAM().Kinds(c.target, c.actions), ",")
		if got != c.want {
			t.Errorf("%s %v: got %q, want %q", c.target, c.actions, got, c.want)
		}
	}
}

var update = flag.Bool("update", false, "rewrite the bundled books in canonical form")

// Every book file must be in the form sync writes, or a sync that changes
// nothing would still produce a diff.
// Fix with: go test ./provider/aws -run TestBooksAreCanonical -update
func TestBooksAreCanonical(t *testing.T) {
	files, err := filepath.Glob("../../catalog/aws/*/books/*.json")
	if err != nil || len(files) == 0 {
		t.Fatalf("no book files: %v", err)
	}
	for _, path := range files {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var b book.Book
		if err := json.Unmarshal(raw, &b); err != nil {
			t.Fatal(err)
		}
		want, err := book.Marshal(b)
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Equal(raw, want) {
			continue
		}
		if *update {
			if err := os.WriteFile(path, want, 0o644); err != nil {
				t.Fatal(err)
			}
			continue
		}
		t.Errorf("%s is not canonical; run: go test ./provider/aws -run TestBooksAreCanonical -update", path)
	}
}

// A row belongs to exactly one unit.
func TestUnitsDoNotShareIDs(t *testing.T) {
	if _, err := Books(); err != nil {
		t.Fatal(err)
	}
}

// Every resource carries worked examples, and every example holds. A case
// never records a missing reference: that would be a broken book, not a result.
// After an intended change, rewrite the expected values with:
//
//	go test ./provider/aws -run TestCases -update
func TestCases(t *testing.T) {
	books, err := Books()
	if err != nil {
		t.Fatal(err)
	}
	for _, u := range mustUnits() {
		if u.Resource == nil {
			continue
		}
		if len(u.Cases) == 0 {
			t.Errorf("%s has no cases.yaml", u.Name)
			continue
		}
		for i, c := range u.Cases {
			if strings.Contains(c.Error, "no reference entry") || strings.Contains(c.Error, "but the reading counts") {
				t.Errorf("%s case %q reads a missing or mismatched reference: %s", u.Name, c.Name, c.Error)
			}
			if *update {
				costs, limits, err := c.Read(u.Resource, books)
				u.Cases[i].Costs, u.Cases[i].Limits, u.Cases[i].Error = costs, limits, ""
				if err != nil {
					u.Cases[i].Error = err.Error()
				}
				continue
			}
			if err := c.Check(u.Resource, books); err != nil {
				t.Errorf("%s: %v", u.Name, err)
			}
		}
		if *update {
			writeCases(t, u)
		}
	}
}

func writeCases(t *testing.T, u definition.Unit) {
	data, err := yaml.Marshal(u.Cases)
	if err != nil {
		t.Fatal(err)
	}
	header := "# Worked examples: given these values and this load, the resource reads this.\n# Monthly USD per cost line, peak demand per limit. After an intended change:\n#   go test ./provider/aws -run TestCases -update\n"
	if err := os.WriteFile(filepath.Join("..", "..", "catalog", "aws", u.Name, "cases.yaml"), append([]byte(header), data...), 0o644); err != nil {
		t.Fatal(err)
	}
}
