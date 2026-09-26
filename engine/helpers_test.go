package engine

import (
	"math"
	"testing"
)

func f(v float64) *float64 { return &v }

func resultsByID(nodes []NodeResult) map[string]NodeResult {
	byID := map[string]NodeResult{}
	for _, n := range nodes {
		byID[n.ID] = n
	}
	return byID
}

// wantOnePath fails unless there is exactly one path, with the given p99 and availability.
func wantOnePath(t *testing.T, paths []PathResult, p99, availability float64) {
	t.Helper()
	if len(paths) != 1 || paths[0].P99Ms != p99 || math.Abs(paths[0].Availability-availability) > 1e-12 {
		t.Fatalf("paths %+v", paths)
	}
}
