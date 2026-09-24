package definition

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/O6lvl4/arch-scouter/book"
	"github.com/O6lvl4/arch-scouter/meter"
	"github.com/O6lvl4/arch-scouter/model"
)

// Case is one worked example a resource carries: given these values and this
// load, the readings are these. Cases are the resource's answer key.
type Case struct {
	Name        string              `yaml:"name"`
	Region      string              `yaml:"region"`
	Attributes  map[string]any      `yaml:"attributes,omitempty"`
	Assumptions map[string]any      `yaml:"assumptions,omitempty"`
	Demand      map[string]CaseLoad `yaml:"demand,omitempty"`
	// Costs are the expected monthly USD per cost line; Limits the expected peak demand per limit.
	Costs  map[string]float64 `yaml:"costs,omitempty"`
	Limits map[string]float64 `yaml:"limits,omitempty"`
	// Error is a substring the node's error must contain.
	Error string `yaml:"error,omitempty"`
}

// CaseLoad is a load written in a case.
type CaseLoad struct {
	Monthly float64 `yaml:"monthly"`
	Peak    float64 `yaml:"peak"`
}

// Read runs the case and returns what the resource reads.
func (c Case) Read(r *Resource, books book.Books) (costs, limits map[string]float64, err error) {
	d := model.Demand{}
	for k, l := range c.Demand {
		d[k] = model.Load{Monthly: l.Monthly, PeakPerSecond: l.Peak}
	}
	rec := meter.NewRecorder(c.Region, books)
	err = r.Scout(model.Node{ID: "case", Type: r.File.Type, Attributes: c.Attributes, Assumptions: c.Assumptions}, d, rec)
	costs, limits = map[string]float64{}, map[string]float64{}
	for _, x := range rec.Costs() {
		if x.MonthlyUSD != nil {
			costs[x.Name] = round(*x.MonthlyUSD)
		}
	}
	for _, l := range rec.Limits() {
		limits[l.Name] = round(l.Demand)
	}
	return costs, limits, err
}

// Check compares the case with what the resource reads.
func (c Case) Check(r *Resource, books book.Books) error {
	costs, limits, err := c.Read(r, books)
	var problems []string
	switch {
	case c.Error == "" && err != nil:
		problems = append(problems, "unexpected error: "+err.Error())
	case c.Error != "" && (err == nil || !strings.Contains(err.Error(), c.Error)):
		problems = append(problems, fmt.Sprintf("want an error containing %q, got %v", c.Error, err))
	}
	problems = append(problems, compare("cost", c.Costs, costs)...)
	problems = append(problems, compare("limit", c.Limits, limits)...)
	if len(problems) > 0 {
		return fmt.Errorf("case %q: %s", c.Name, strings.Join(problems, "; "))
	}
	return nil
}

func compare(what string, want, got map[string]float64) []string {
	var out []string
	for _, k := range keys(want, got) {
		w, okW := want[k]
		g, okG := got[k]
		switch {
		case !okW:
			out = append(out, fmt.Sprintf("unexpected %s %q = %v", what, k, g))
		case !okG:
			out = append(out, fmt.Sprintf("missing %s %q", what, k))
		case math.Abs(w-g) > 1e-9*math.Max(1, math.Abs(w)):
			out = append(out, fmt.Sprintf("%s %q: want %v, got %v", what, k, w, g))
		}
	}
	return out
}

func keys(a, b map[string]float64) []string {
	set := map[string]bool{}
	for k := range a {
		set[k] = true
	}
	for k := range b {
		set[k] = true
	}
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func round(v float64) float64 { return math.Round(v*1e9) / 1e9 }
