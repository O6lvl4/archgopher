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
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/O6lvl4/archgopher/book"
	"github.com/O6lvl4/archgopher/engine"
)

// Bill is what the account paid in a period, by service name, and by usage
// type within a service when the export was grouped that finely.
type Bill struct {
	Period string      `json:"period"`
	Lines  []BillLine  `json:"lines"`
	Usage  []UsageLine `json:"usage,omitempty"`
}

// BillLine is one service's charge.
type BillLine struct {
	Service string  `json:"service"`
	USD     float64 `json:"usd"`
}

// UsageLine is one usage type's charge and quantity within a service.
type UsageLine struct {
	Service   string  `json:"service"`
	UsageType string  `json:"usageType"`
	USD       float64 `json:"usd"`
	Quantity  float64 `json:"quantity"`
	Unit      string  `json:"unit"`
	// Note, on an unread line, says why no price could read it when the
	// reason is the book rather than the declaration.
	Note string `json:"note,omitempty"`
}

// UsageRow is one price of the declaration against the usage types it is
// billed under: the same money on both sides, and the quantities each side
// counted, in each side's unit.
type UsageRow struct {
	Service     string   `json:"service"`
	UsageTypes  []string `json:"usageTypes"`
	BillUSD     float64  `json:"billUsd"`
	BillQty     float64  `json:"billQuantity"`
	BillUnit    string   `json:"billUnit"`
	DeclaredUSD float64  `json:"declaredUsd"`
	DeclaredQty float64  `json:"declaredQuantity"`
	Unit        string   `json:"declaredUnit"`
	PriceID     string   `json:"priceId"`
	Lines       []string `json:"lines"`
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
	// UsageRows put each price next to the usage types it is billed under,
	// when the bill was grouped by usage type; UnreadUsage are usage types of
	// covered services that no line of the declaration reads.
	UsageRows   []UsageRow  `json:"usageRows,omitempty"`
	UnreadUsage []UsageLine `json:"unreadUsage,omitempty"`
}

// ParseCostExplorer reads the JSON `aws ce get-cost-and-usage` writes when
// grouped by the SERVICE dimension, alone or with USAGE_TYPE as the second
// group; several periods are summed and the period spans them. With usage
// types, the UsageQuantity metric gives each line its quantity.
func ParseCostExplorer(data []byte) (Bill, error) {
	var doc struct {
		GroupDefinitions []struct{ Key string }
		ResultsByTime    []struct {
			TimePeriod struct{ Start, End string }
			Groups     []struct {
				Keys    []string
				Metrics map[string]struct{ Amount, Unit string }
			}
		}
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		return Bill{}, fmt.Errorf("bill: %w", err)
	}
	if len(doc.ResultsByTime) == 0 {
		return Bill{}, fmt.Errorf("bill: no ResultsByTime; group `aws ce get-cost-and-usage` by SERVICE")
	}
	byUsage := len(doc.GroupDefinitions) == 2 && doc.GroupDefinitions[0].Key == "SERVICE" && doc.GroupDefinitions[1].Key == "USAGE_TYPE"
	sums := map[string]float64{}
	usage := map[[2]string]*UsageLine{}
	var order []string
	var usageOrder [][2]string
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
			if !byUsage || len(g.Keys) < 2 {
				continue
			}
			k := [2]string{g.Keys[0], g.Keys[1]}
			u, seen := usage[k]
			if !seen {
				u = &UsageLine{Service: k[0], UsageType: k[1]}
				usage[k] = u
				usageOrder = append(usageOrder, k)
			}
			u.USD += usd
			if q, ok := g.Metrics["UsageQuantity"]; ok {
				qty, _ := strconv.ParseFloat(q.Amount, 64)
				u.Quantity += qty
				u.Unit = q.Unit
			}
		}
	}
	b := Bill{Period: doc.ResultsByTime[0].TimePeriod.Start + ".." + doc.ResultsByTime[len(doc.ResultsByTime)-1].TimePeriod.End}
	for _, s := range order {
		b.Lines = append(b.Lines, BillLine{Service: s, USD: sums[s]})
	}
	for _, k := range usageOrder {
		b.Usage = append(b.Usage, *usage[k])
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
	rep.UsageRows, rep.UnreadUsage = compareUsage(res, books, names, bill, nodes)
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

// syncOf is how a price syncs: the service it comes from and the filters
// that pick it out of the price list, read from its book entry or from the
// table it is a row of. A "{row}" in a filter stands for the row's key.
type syncOf struct {
	Service string
	Filters map[string]string
}

func syncFor(books book.Books, priceID string) (syncOf, bool) {
	for id := priceID; id != ""; {
		if e, ok := books.Prices[id]; ok && len(e.Sync) > 0 {
			var sync syncOf
			if json.Unmarshal(e.Sync, &sync) == nil && sync.Service != "" {
				if id != priceID {
					row := priceID[len(id)+1:]
					for k, v := range sync.Filters {
						sync.Filters[k] = strings.ReplaceAll(v, "{row}", regexp.QuoteMeta(row))
					}
				}
				return sync, true
			}
		}
		i := strings.LastIndex(id, ".")
		if i < 0 {
			break
		}
		id = id[:i]
	}
	return syncOf{}, false
}

// serviceCode is the offer code a price syncs from; "" when none is known.
func serviceCode(books book.Books, priceID string) string {
	s, _ := syncFor(books, priceID)
	return s.Service
}

// compareUsage puts each price of the declaration next to the bill's usage
// types that its sync filter names, within the price's service, and lists
// the usage types of covered services that no price reads.
func compareUsage(res engine.Result, books book.Books, names Names, bill Bill, nodes map[string]map[string]bool) ([]UsageRow, []UsageLine) {
	if len(bill.Usage) == 0 {
		return nil, nil
	}
	rows := map[string]*UsageRow{}
	var order []string
	read := map[[2]string]bool{}
	// unfiltered are the codes with a price the book picks out by something
	// other than usage type: the bill's usage types cannot be read against it.
	unfiltered := map[string]bool{}
	for _, n := range nodeResults(res) {
		for _, c := range n.Costs {
			if c.MonthlyUSD == nil {
				continue
			}
			sync, ok := syncFor(books, c.PriceID)
			if !ok {
				continue
			}
			if sync.Filters["usagetype"] == "" {
				unfiltered[sync.Service] = true
				continue
			}
			re, err := regexp.Compile("^(?:" + strings.ReplaceAll(sync.Filters["usagetype"], "{region}", "[A-Za-z0-9-]+") + ")$")
			if err != nil {
				continue
			}
			row, seen := rows[c.PriceID]
			if !seen {
				row = &UsageRow{PriceID: c.PriceID, Unit: c.Unit}
				services := narrow(servicesOf(bill, names, sync.Service), c.PriceID)
				types := map[string]bool{}
				for _, u := range bill.Usage {
					if !services[u.Service] || !re.MatchString(u.UsageType) {
						continue
					}
					row.Service = u.Service
					if !types[u.UsageType] {
						types[u.UsageType] = true
						row.UsageTypes = append(row.UsageTypes, u.UsageType)
					}
					row.BillUSD += u.USD
					row.BillQty += u.Quantity
					row.BillUnit = u.Unit
					read[[2]string{u.Service, u.UsageType}] = true
				}
				if row.Service == "" {
					row.Service = plain(names[sync.Service], sync.Service)
				}
				rows[c.PriceID] = row
				order = append(order, c.PriceID)
			}
			row.DeclaredUSD += *c.MonthlyUSD
			row.DeclaredQty += c.Quantity
			row.Lines = append(row.Lines, n.ID+": "+c.Name)
		}
	}
	var out []UsageRow
	for _, id := range order {
		out = append(out, *rows[id])
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := max(out[i].BillUSD, out[i].DeclaredUSD), max(out[j].BillUSD, out[j].DeclaredUSD)
		if a != b {
			return a > b
		}
		return out[i].PriceID < out[j].PriceID
	})
	var unread []UsageLine
	for _, u := range bill.Usage {
		code := codeOf(names, u.Service)
		if code == "" || len(nodes[code]) == 0 || read[[2]string{u.Service, u.UsageType}] {
			continue
		}
		if unfiltered[code] {
			u.Note = "a price of this service is picked out of the price list by something other than usage type, so the bill's line cannot be read against it"
		}
		unread = append(unread, u)
	}
	sort.SliceStable(unread, func(i, j int) bool { return unread[i].USD > unread[j].USD })
	return out, unread
}

// servicesOf lists the bill's service names that fall under one code.
func servicesOf(bill Bill, names Names, code string) []string {
	seen := map[string]bool{}
	var out []string
	for _, u := range bill.Usage {
		if !seen[u.Service] && codeOf(names, u.Service) == code {
			seen[u.Service] = true
			out = append(out, u.Service)
		}
	}
	return out
}

// narrow picks, among the bill's services under one code, the ones a price
// is about when the code gathers several: Bedrock bills each model as its
// own service, and the price id names the model ("claude-haiku-4-5" is in
// "Claude Haiku 4.5 (Amazon Bedrock Edition)" once both lose their
// punctuation). A price that names none of them is read against all.
func narrow(services []string, priceID string) map[string]bool {
	out := map[string]bool{}
	if len(services) > 1 {
		for _, seg := range strings.Split(priceID, ".") {
			key := squash(seg)
			if len(key) < 4 {
				continue
			}
			// A segment every service carries ("bedrock") tells nothing.
			hits := map[string]bool{}
			for _, s := range services {
				if strings.Contains(squash(s), key) {
					hits[s] = true
				}
			}
			if len(hits) == len(services) {
				continue
			}
			for s := range hits {
				out[s] = true
			}
		}
	}
	if len(out) == 0 {
		for _, s := range services {
			out[s] = true
		}
	}
	return out
}

// squash lowers a name and drops everything but letters and digits.
func squash(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// plain is the first name without a wildcard, else the code.
func plain(names []string, code string) string {
	for _, n := range names {
		if !strings.Contains(n, "*") {
			return n
		}
	}
	return code
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
