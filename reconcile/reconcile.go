// Package reconcile puts a declaration's reading next to the account's bill,
// service by service. A reading is right when the bill agrees with it line
// by line, not when the totals happen to meet; and an account is billed for
// more than one declaration stands for. The comparison says which part of
// the bill the declaration explains, which part nothing here stands for, and
// where the two disagree.
package reconcile

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/O6lvl4/archgopher/book"
	"github.com/O6lvl4/archgopher/engine"
)

// Bill is what the account paid in a period, by service name.
type Bill struct {
	Period string     `json:"period"`
	Lines  []BillLine `json:"lines"`
}

// BillLine is one service's charge.
type BillLine struct {
	Service string  `json:"service"`
	USD     float64 `json:"usd"`
}

// Names maps a billing service code (the Price List offer code the books
// sync from) to the names the bill carries for it. A * in a name matches any
// text in its place.
type Names map[string][]string

// Row is one service seen from both sides.
type Row struct {
	Service     string  `json:"service"`
	BillUSD     float64 `json:"billUsd"`
	DeclaredUSD float64 `json:"declaredUsd"`
	// Billed lists the bill's own names when several fall under one service
	// (Bedrock bills each model as its own); Service then summarizes them.
	Billed []string `json:"billed,omitempty"`
	Nodes  []string `json:"nodes,omitempty"`
}

// Unmapped is a cost line whose price names no billing service.
type Unmapped struct {
	Node    string  `json:"node"`
	Line    string  `json:"line"`
	PriceID string  `json:"priceId"`
	USD     float64 `json:"usd"`
}

// Report is the comparison.
type Report struct {
	Name   string `json:"name"`
	Period string `json:"period"`
	// BillUSD is the whole bill; TaxUSD the tax lines in it; CoveredUSD the
	// services the declaration has nodes for; OutsideUSD the rest.
	BillUSD     float64 `json:"billUsd"`
	TaxUSD      float64 `json:"taxUsd"`
	CoveredUSD  float64 `json:"coveredUsd"`
	OutsideUSD  float64 `json:"outsideUsd"`
	DeclaredUSD float64 `json:"declaredUsd"`
	// Rows are the services the declaration has nodes for, billed or not.
	Rows []Row `json:"rows"`
	// Outside are billed services no node stands for.
	Outside []Row `json:"outside"`
	// Unmapped are declared lines that could not be placed under a service.
	Unmapped []Unmapped `json:"unmapped,omitempty"`
}

// ParseCostExplorer reads the JSON `aws ce get-cost-and-usage` writes when
// grouped by the SERVICE dimension (the first group key); several periods
// are summed and the period spans them.
func ParseCostExplorer(data []byte) (Bill, error) {
	var doc struct {
		ResultsByTime []struct {
			TimePeriod struct{ Start, End string }
			Groups     []struct {
				Keys    []string
				Metrics map[string]struct{ Amount string }
			}
		}
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		return Bill{}, fmt.Errorf("bill: %w", err)
	}
	if len(doc.ResultsByTime) == 0 {
		return Bill{}, fmt.Errorf("bill: no ResultsByTime; group `aws ce get-cost-and-usage` by SERVICE")
	}
	sums := map[string]float64{}
	var order []string
	for _, r := range doc.ResultsByTime {
		for _, g := range r.Groups {
			if len(g.Keys) == 0 {
				continue
			}
			m, ok := g.Metrics["UnblendedCost"]
			if !ok {
				for _, v := range g.Metrics {
					m = v
					break
				}
			}
			usd, err := strconv.ParseFloat(m.Amount, 64)
			if err != nil {
				return Bill{}, fmt.Errorf("bill: %s: %w", g.Keys[0], err)
			}
			if _, seen := sums[g.Keys[0]]; !seen {
				order = append(order, g.Keys[0])
			}
			sums[g.Keys[0]] += usd
		}
	}
	b := Bill{Period: doc.ResultsByTime[0].TimePeriod.Start + ".." + doc.ResultsByTime[len(doc.ResultsByTime)-1].TimePeriod.End}
	for _, s := range order {
		b.Lines = append(b.Lines, BillLine{Service: s, USD: sums[s]})
	}
	return b, nil
}

// Compare places every cost line of the reading under the billing service
// its price belongs to, matches the bill's lines to the same services, and
// reports the two side by side.
func Compare(name string, res engine.Result, books book.Books, names Names, bill Bill) Report {
	rep := Report{Name: name, Period: bill.Period}
	declared := map[string]float64{}
	nodes := map[string]map[string]bool{}
	for _, n := range nodeResults(res) {
		for _, c := range n.Costs {
			if c.MonthlyUSD == nil {
				continue
			}
			rep.DeclaredUSD += *c.MonthlyUSD
			code := serviceCode(books, c.PriceID)
			if code == "" {
				rep.Unmapped = append(rep.Unmapped, Unmapped{Node: n.ID, Line: c.Name, PriceID: c.PriceID, USD: *c.MonthlyUSD})
				continue
			}
			declared[code] += *c.MonthlyUSD
			if nodes[code] == nil {
				nodes[code] = map[string]bool{}
			}
			nodes[code][n.ID] = true
		}
	}
	billed := map[string]float64{}
	lines := map[string][]string{}
	for _, l := range bill.Lines {
		rep.BillUSD += l.USD
		if strings.EqualFold(l.Service, "Tax") {
			rep.TaxUSD += l.USD
			continue
		}
		code := codeOf(names, l.Service)
		if code == "" {
			rep.Outside = append(rep.Outside, Row{Service: l.Service, BillUSD: l.USD})
			rep.OutsideUSD += l.USD
			continue
		}
		billed[code] += l.USD
		lines[code] = append(lines[code], l.Service)
	}
	codes := map[string]bool{}
	for c := range declared {
		codes[c] = true
	}
	for c := range billed {
		codes[c] = true
	}
	for _, code := range sortedKeys(codes) {
		row := Row{Service: label(names[code], code, lines[code]), BillUSD: billed[code], DeclaredUSD: declared[code], Nodes: sortedKeys(nodes[code])}
		if len(lines[code]) > 1 {
			row.Billed = lines[code]
		}
		if len(row.Nodes) == 0 {
			// Billed, known to the catalog, but no node here stands for it.
			rep.Outside = append(rep.Outside, row)
			rep.OutsideUSD += row.BillUSD
			continue
		}
		rep.CoveredUSD += row.BillUSD
		rep.Rows = append(rep.Rows, row)
	}
	byMoney := func(rows []Row) {
		sort.SliceStable(rows, func(i, j int) bool {
			a, b := max(rows[i].BillUSD, rows[i].DeclaredUSD), max(rows[j].BillUSD, rows[j].DeclaredUSD)
			if a != b {
				return a > b
			}
			return rows[i].Service < rows[j].Service
		})
	}
	byMoney(rep.Rows)
	byMoney(rep.Outside)
	return rep
}

// nodeResults are the nodes and groups whose costs count once: a rolled-up
// pattern repeats its members and is skipped.
func nodeResults(res engine.Result) []engine.NodeResult {
	var out []engine.NodeResult
	for _, n := range res.Nodes {
		if len(n.Members) == 0 {
			out = append(out, n)
		}
	}
	return append(out, res.Groups...)
}

// serviceCode is the offer code a price syncs from, read from its book
// entry, or from the table it is a row of when the row carries none; ""
// when no entry on the way up names one.
func serviceCode(books book.Books, priceID string) string {
	for id := priceID; id != ""; {
		if e, ok := books.Prices[id]; ok && len(e.Sync) > 0 {
			var sync struct{ Service string }
			if json.Unmarshal(e.Sync, &sync) == nil && sync.Service != "" {
				return sync.Service
			}
		}
		i := strings.LastIndex(id, ".")
		if i < 0 {
			break
		}
		id = id[:i]
	}
	return ""
}

// codeOf finds the service code a bill line's name belongs to.
func codeOf(names Names, service string) string {
	for _, code := range sortedKeys(names) {
		for _, n := range names[code] {
			if matches(n, service) {
				return code
			}
		}
	}
	return ""
}

// matches compares a name pattern, * standing for any text, with a service name.
func matches(pattern, s string) bool {
	parts := strings.Split(pattern, "*")
	if len(parts) == 1 {
		return pattern == s
	}
	if !strings.HasPrefix(s, parts[0]) {
		return false
	}
	s = s[len(parts[0]):]
	for i := 1; i < len(parts)-1; i++ {
		j := strings.Index(s, parts[i])
		if j < 0 {
			return false
		}
		s = s[j+len(parts[i]):]
	}
	return strings.HasSuffix(s, parts[len(parts)-1])
}

// label names a service row: the bill's own name when one line fell under
// the code; otherwise the first name without a wildcard, or the wildcard
// pattern's words with how many lines it gathered ("(Amazon Bedrock
// Edition) ×4"); the code when there is nothing better.
func label(names []string, code string, lines []string) string {
	if len(lines) == 1 {
		return lines[0]
	}
	plain := code
	for _, n := range names {
		if !strings.Contains(n, "*") {
			plain = n
			break
		}
		if plain == code {
			plain = n
		}
	}
	if len(lines) > 1 {
		return fmt.Sprintf("%s ×%d", strings.TrimSpace(strings.ReplaceAll(plain, "*", "")), len(lines))
	}
	return plain
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
