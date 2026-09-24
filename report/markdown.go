// Package report renders an engine result for people (Markdown) and machines (JSON).
package report

import (
	"encoding/json"
	"fmt"
	"io"
	"strconv"
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
	fmt.Fprintf(b, "# %s (%s)\n\n", orDash(r.Name), r.Region)
	fmt.Fprintf(b, "Monthly cost: **%s**", usd(r.MonthlyUSD))
	if r.UnpricedCosts > 0 {
		fmt.Fprintf(b, " (plus %d cost lines with unknown prices)", r.UnpricedCosts)
	}
	b.WriteString("\n\n")

	b.WriteString("## Nodes\n\n")
	if len(members(r)) < len(r.Nodes) {
		b.WriteString("Pattern rows sum the nodes they expand into.\n\n")
	}
	b.WriteString("| Node | Type | Monthly | Tightest headroom | p99 | SLA | Status |\n| --- | --- | ---: | ---: | ---: | ---: | --- |\n")
	for _, n := range r.Nodes {
		fmt.Fprintf(b, "| %s | %s | %s | %s | %s | %s | %s |\n",
			n.ID, label(n), usd(n.MonthlyUSD), pct(n.MinHeadroom()), latency(n.Latency), sla(n.SLA), status(n))
	}

	b.WriteString("\n## Cost lines\n\n| Node | Component | Quantity | Unit | Unit price | Monthly |\n| --- | --- | ---: | --- | ---: | ---: |\n")
	for _, n := range members(r) {
		for _, c := range n.Costs {
			fmt.Fprintf(b, "| %s | %s | %s | %s | %s | %s |\n", n.ID, c.Name, num(c.Quantity), c.Unit, price(c.UnitPrice), usdPtr(c.MonthlyUSD))
		}
	}

	b.WriteString("\n## Limits\n\n| Node | Limit | Peak demand | Capacity | Unit | Headroom |\n| --- | --- | ---: | ---: | --- | ---: |\n")
	for _, n := range members(r) {
		for _, l := range n.Limits {
			fmt.Fprintf(b, "| %s | %s | %s | %s | %s | %s |\n", n.ID, l.Name, num(l.Demand), numPtr(l.Capacity), l.Unit, pct(l.Headroom))
		}
	}

	if len(r.Paths) > 0 {
		b.WriteString("\n## Paths\n\np99 adds up the p99 of every hop, so it is an upper bound.\n\n| Path | p50 | p99 | Availability | Missing |\n| --- | ---: | ---: | ---: | --- |\n")
		for _, p := range r.Paths {
			fmt.Fprintf(b, "| %s | %s ms | %s ms | %s | %s |\n", strings.Join(p.Nodes, " → "), num(p.P50Ms), num(p.P99Ms), availability(p.Availability), missing(p))
		}
	}

	if len(r.Unverified) > 0 {
		b.WriteString("\n## Unverified reference values\n\nNobody has checked these against the source yet, or the value is unknown.\n\n| Book | ID | Region | State | Source |\n| --- | --- | --- | --- | --- |\n")
		for _, u := range r.Unverified {
			state := "unverified"
			if !u.Known {
				state = "unknown"
			}
			fmt.Fprintf(b, "| %s | %s | %s | %s | %s |\n", u.Book, u.ID, u.Region, state, u.Source)
		}
	}

	var problems []string
	for _, n := range r.Nodes {
		if n.Error != "" {
			problems = append(problems, fmt.Sprintf("- **%s**: %s", n.ID, n.Error))
		}
		if n.Skipped != "" {
			problems = append(problems, fmt.Sprintf("- **%s**: skipped, %s", n.ID, n.Skipped))
		}
		if n.Stale {
			problems = append(problems, fmt.Sprintf("- **%s**: no longer in Terraform (%s)", n.ID, n.Address))
		}
	}
	for _, w := range r.Warnings {
		problems = append(problems, "- "+w)
	}
	if len(problems) > 0 {
		b.WriteString("\n## Problems\n\n" + strings.Join(problems, "\n") + "\n")
	}
	_, err := io.WriteString(w, b.String())
	return err
}

// members skips rolled-up pattern results, whose lines their members already list.
func members(r engine.Result) []engine.NodeResult {
	var out []engine.NodeResult
	for _, n := range r.Nodes {
		if len(n.Members) == 0 {
			out = append(out, n)
		}
	}
	return out
}

func status(n engine.NodeResult) string {
	switch {
	case n.Error != "":
		return "error"
	case n.Skipped != "":
		return "skipped"
	case n.Stale:
		return "stale"
	}
	if h := n.MinHeadroom(); h != nil && *h < 0 {
		return "over limit"
	}
	return "ok"
}

func missing(p engine.PathResult) string {
	var parts []string
	if len(p.MissingLatency) > 0 {
		parts = append(parts, "latency: "+strings.Join(p.MissingLatency, ", "))
	}
	if len(p.MissingSLA) > 0 {
		parts = append(parts, "SLA: "+strings.Join(p.MissingSLA, ", "))
	}
	return orDash(strings.Join(parts, "; "))
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func usd(v float64) string { return "$" + strconv.FormatFloat(v, 'f', 2, 64) }

func usdPtr(v *float64) string {
	if v == nil {
		return "unknown"
	}
	return usd(*v)
}

func price(v *float64) string {
	if v == nil {
		return "unknown"
	}
	return "$" + strconv.FormatFloat(*v, 'g', 4, 64)
}

func num(v float64) string {
	switch {
	case v == 0:
		return "0"
	case v >= 1000:
		return group(strconv.FormatFloat(v, 'f', 0, 64))
	case v >= 1:
		return strconv.FormatFloat(v, 'f', 2, 64)
	}
	return strconv.FormatFloat(v, 'g', 3, 64)
}

func group(s string) string {
	n := len(s)
	if n <= 3 {
		return s
	}
	var b strings.Builder
	for i, c := range s {
		if i > 0 && (n-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(c)
	}
	return b.String()
}

func numPtr(v *float64) string {
	if v == nil {
		return "unknown"
	}
	return num(*v)
}

func pct(v *float64) string {
	if v == nil {
		return "-"
	}
	return strconv.FormatFloat(*v*100, 'f', 1, 64) + "%"
}

func latency(l *engine.Latency) string {
	if l == nil {
		return "-"
	}
	return num(l.P99Ms) + " ms"
}

func sla(a *engine.Availability) string {
	if a == nil {
		return "-"
	}
	if a.Value == nil {
		return "unknown"
	}
	return availability(*a.Value)
}

func availability(v float64) string { return strconv.FormatFloat(v*100, 'f', 3, 64) + "%" }

func label(n engine.NodeResult) string {
	if len(n.Members) > 0 {
		return n.Label + " (pattern)"
	}
	return n.Label
}
