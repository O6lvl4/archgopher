// Package report renders an engine result for people (Markdown) and machines (JSON).
package report

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/O6lvl4/archgopher/engine"
)

// JSON writes the result as indented JSON.
func JSON(w io.Writer, r engine.Result) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(r)
}

// Markdown writes the result as Markdown tables.
func Markdown(w io.Writer, r engine.Result) error {
	b := &strings.Builder{}
	for _, section := range []func(*strings.Builder, engine.Result){
		writeTotal, writeNodes, writeLoads, writeCosts, writePools, writeLimits, writePaths, writeUnverified, writeProblems,
	} {
		section(b, r)
	}
	_, err := io.WriteString(w, b.String())
	return err
}

func writeTotal(b *strings.Builder, r engine.Result) {
	fmt.Fprintf(b, "# %s (%s)\n\n", orDash(r.Name), r.Region)
	fmt.Fprintf(b, "Monthly cost: **%s**", usd(r.MonthlyUSD))
	if r.UnpricedCosts > 0 {
		fmt.Fprintf(b, " (plus %d cost lines with unknown prices)", r.UnpricedCosts)
	}
	b.WriteString("\n\n")
}

func writeNodes(b *strings.Builder, r engine.Result) {
	b.WriteString("## Nodes\n\n")
	if len(members(r))-len(r.Groups) < len(r.Nodes) {
		b.WriteString("Pattern rows sum the nodes they expand into.\n\n")
	}
	b.WriteString("| Node | Type | Monthly | Tightest headroom | p99 | SLA | Status |\n| --- | --- | ---: | ---: | ---: | ---: | --- |\n")
	for _, n := range nodesAndGroups(r) {
		fmt.Fprintf(b, "| %s | %s | %s | %s | %s | %s | %s |\n",
			n.ID, label(n), usd(n.MonthlyUSD), pct(n.MinHeadroom()), latency(n.Latency), sla(n.SLA), status(n))
	}
}

// writeLoads lists the nodes that bring load in, and how it was worked out.
func writeLoads(b *strings.Builder, r engine.Result) {
	var arrivals []engine.NodeResult
	for _, n := range r.Nodes {
		if n.Load != nil {
			arrivals = append(arrivals, n)
		}
	}
	if len(arrivals) == 0 {
		return
	}
	b.WriteString("\n## Load in\n\n| Node | Monthly | Peak / s | How |\n| --- | ---: | ---: | --- |\n")
	for _, n := range arrivals {
		fmt.Fprintf(b, "| %s | %s | %s | %s |\n", n.ID, num(n.Load.Monthly), num(n.Load.PeakPerSecond), orGiven(n.LoadBasis))
	}
}

func writeCosts(b *strings.Builder, r engine.Result) {
	b.WriteString("\n## Cost lines\n\n| Node | Component | Quantity | Unit | Unit price | Monthly |\n| --- | --- | ---: | --- | ---: | ---: |\n")
	for _, n := range members(r) {
		for _, c := range n.Costs {
			fmt.Fprintf(b, "| %s | %s | %s | %s | %s | %s |\n", n.ID, c.Name, num(c.Quantity), c.Unit, price(c.UnitPrice), usdPtr(c.MonthlyUSD))
		}
	}
}

func writePools(b *strings.Builder, r engine.Result) {
	if len(r.Pools) == 0 {
		return
	}
	b.WriteString("\n## Shared across the account\n\nThe provider bills these prices on what the whole account uses: volume tiers and included units count once, and each line above pays the average price of its pool.\n\n| Price | Quantity | Unit | Bands | Monthly | Lines |\n| --- | ---: | --- | --- | ---: | --- |\n")
	for _, p := range r.Pools {
		fmt.Fprintf(b, "| %s | %s | %s | %s | %s | %s |\n", p.PriceID, num(p.Quantity), p.Unit, bands(p.Bands), usdPtr(p.MonthlyUSD), poolLines(p))
	}
}

func writeLimits(b *strings.Builder, r engine.Result) {
	b.WriteString("\n## Limits\n\n| Node | Limit | Peak demand | Capacity | Unit | Headroom |\n| --- | --- | ---: | ---: | --- | ---: |\n")
	for _, n := range members(r) {
		for _, l := range n.Limits {
			fmt.Fprintf(b, "| %s | %s | %s | %s | %s | %s |\n", n.ID, l.Name, num(l.Demand), numPtr(l.Capacity), l.Unit, pct(l.Headroom))
		}
	}
}

func writePaths(b *strings.Builder, r engine.Result) {
	if len(r.Paths) == 0 {
		return
	}
	b.WriteString("\n## Paths\n\np99 adds up the p99 of every hop, so it is an upper bound.\n\n| Path | p50 | p99 | Availability | Missing |\n| --- | ---: | ---: | ---: | --- |\n")
	for _, p := range r.Paths {
		fmt.Fprintf(b, "| %s | %s ms | %s ms | %s | %s |\n", strings.Join(p.Nodes, " → "), num(p.P50Ms), num(p.P99Ms), availability(p.Availability), missing(p))
	}
}

func writeUnverified(b *strings.Builder, r engine.Result) {
	if len(r.Unverified) == 0 {
		return
	}
	b.WriteString("\n## Unverified reference values\n\nNobody has checked these against the source yet, or the value is unknown.\n\n| Book | ID | Region | State | Source |\n| --- | --- | --- | --- | --- |\n")
	for _, u := range r.Unverified {
		state := "unverified"
		if !u.Known {
			state = "unknown"
		}
		fmt.Fprintf(b, "| %s | %s | %s | %s | %s |\n", u.Book, u.ID, u.Region, state, u.Source)
	}
}

func writeProblems(b *strings.Builder, r engine.Result) {
	var problems []string
	for _, n := range nodesAndGroups(r) {
		problems = append(problems, nodeProblems(n)...)
	}
	for _, w := range r.Warnings {
		problems = append(problems, "- "+w)
	}
	if len(problems) > 0 {
		b.WriteString("\n## Problems\n\n" + strings.Join(problems, "\n") + "\n")
	}
}

// nodeProblems are the list items for a node that failed, was skipped or is stale.
func nodeProblems(n engine.NodeResult) []string {
	var out []string
	if n.Error != "" {
		out = append(out, fmt.Sprintf("- **%s**: %s", n.ID, n.Error))
	}
	if n.Skipped != "" {
		out = append(out, fmt.Sprintf("- **%s**: skipped, %s", n.ID, n.Skipped))
	}
	if n.Stale {
		out = append(out, fmt.Sprintf("- **%s**: no longer in Terraform (%s)", n.ID, n.Address))
	}
	return out
}

// nodesAndGroups is every row of the result: the nodes, then the groups.
func nodesAndGroups(r engine.Result) []engine.NodeResult {
	return append(append([]engine.NodeResult(nil), r.Nodes...), r.Groups...)
}

// members skips rolled-up pattern results, whose lines their members already
// list, and adds the groups, which have lines of their own.
func members(r engine.Result) []engine.NodeResult {
	var out []engine.NodeResult
	for _, n := range r.Nodes {
		if len(n.Members) == 0 {
			out = append(out, n)
		}
	}
	return append(out, r.Groups...)
}
