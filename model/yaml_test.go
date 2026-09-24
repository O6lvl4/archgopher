package model

import (
	"strings"
	"testing"
)

func TestSpecRoundTrip(t *testing.T) {
	in := Spec{Name: "n", Region: "r1", Nodes: []Node{{ID: "u", Type: "entry", Load: &Load{Monthly: 3e7, PeakPerSecond: 0.25}}}}
	data, err := MarshalSpec(in)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "monthly: 30000000\n") {
		t.Fatalf("numbers should be plain decimals:\n%s", data)
	}
	out, err := ParseSpec(data)
	if err != nil || out.Nodes[0].Load.Monthly != 3e7 {
		t.Fatalf("round trip: %v %+v", err, out)
	}
	if _, err := ParseSpec([]byte("region: r1\nnodes: []\nedgez: []\n")); err == nil {
		t.Fatal("unknown keys must be rejected")
	}
}
