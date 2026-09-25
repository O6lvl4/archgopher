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

func TestFlattenKeepsTheTiersOfATieredTable(t *testing.T) {
	one := 1.0
	b, err := Book{"x": {Unit: "GB", Tiered: true, Pool: PoolRegion, Rows: map[string]map[string]*float64{"10240": {"r": &one}, "0": {"r": &one}}}}.Flatten()
	if err != nil {
		t.Fatal(err)
	}
	e := b["x"]
	if !e.Tiered || len(e.Tiers) != 2 || e.Tiers[0].ID != "x.0" || e.Tiers[1].From != 10240 || e.Pool != PoolRegion {
		t.Fatalf("x = %+v", e)
	}
	if _, ok := b["x.10240"]; !ok {
		t.Fatal("the rows are gone")
	}
}

func TestFlattenRefusesBadRules(t *testing.T) {
	one := 1.0
	for name, e := range map[string]Entry{
		"no zero tier":      {Tiered: true, Rows: map[string]map[string]*float64{"5": {"r": &one}}},
		"a named tier":      {Tiered: true, Rows: map[string]map[string]*float64{"0": {"r": &one}, "big": {"r": &one}}},
		"tiers, no rows":    {Tiered: true},
		"an unknown pool":   {Pool: "org"},
		"max with no pool":  {Combine: CombineMax},
		"negative included": {Included: -1},
	} {
		if _, err := (Book{"x": e}).Flatten(); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestFlattenGivesEveryRowOfAPlainTableItsRules(t *testing.T) {
	one := 1.0
	b, err := Book{"x": {Unit: "SMS", Pool: PoolAccount, Included: 100, Rows: map[string]map[string]*float64{"1": {"*": &one}, "44": {"*": &one}}}}.Flatten()
	if err != nil {
		t.Fatal(err)
	}
	if e := b["x.44"]; e.Pool != PoolAccount || e.Included != 100 {
		t.Fatalf("x.44 = %+v", e)
	}
}
