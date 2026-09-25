package book

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestTablesFlattenToRows(t *testing.T) {
	var b Book
	err := json.Unmarshal([]byte(`{
	  "aws.ec2.linux": {"unit": "hour", "source": "s", "verified": true, "checkedAt": "2026-01-02",
	    "rows": {"t3.micro": {"us-east-1": 0.0104, "eu-west-1": null}}},
	  "aws.s3.storage": {"unit": "GB-month", "source": "s", "values": {"*": {"value": 0.023}}}}`), &b)
	if err != nil {
		t.Fatal(err)
	}
	flat, err := b.Flatten()
	if err != nil {
		t.Fatal(err)
	}
	e, v, err := flat.Lookup("aws.ec2.linux.t3.micro", "us-east-1")
	if err != nil || *v.Value != 0.0104 || !v.Verified || v.CheckedAt != "2026-01-02" || e.Unit != "hour" {
		t.Fatalf("row: %+v %+v %v", e, v, err)
	}
	if _, v, err := flat.Lookup("aws.ec2.linux.t3.micro", "eu-west-1"); err != nil || v.Value != nil {
		t.Fatalf("not offered: %+v %v", v, err)
	}
	if _, _, err := flat.Lookup("aws.s3.storage", "us-east-1"); err != nil {
		t.Fatal(err)
	}
	if _, ok := flat["aws.ec2.linux"]; ok {
		t.Fatal("the table itself is not an entry")
	}
}

func TestTableAndValuesConflict(t *testing.T) {
	b := Book{"x": {Values: map[string]Value{"*": {}}, Rows: map[string]map[string]*float64{"a": {}}}}
	if _, err := b.Flatten(); err == nil || !strings.Contains(err.Error(), "both") {
		t.Fatalf("got %v", err)
	}
}
