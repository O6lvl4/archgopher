package report

import (
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"

	"github.com/O6lvl4/archgopher/engine"
)

// DiffMarkdown writes a diff as a pull request comment.
func DiffMarkdown(w io.Writer, d Diff) error {
	b := &strings.Builder{}
	b.WriteString(Marker + "\n")
	fmt.Fprintf(b, "### archgopher · %s\n\n", orDash(d.Name))
	fmt.Fprintf(b, "Monthly: **%s → %s** (%s)\n\n", usd(d.BeforeUSD), usd(d.AfterUSD), delta(d.BeforeUSD, d.AfterUSD))
	if d.Empty() {
		b.WriteString("No change in cost, headroom or paths.\n")
		_, err := io.WriteString(w, b.String())
		return err
	}
	for _, a := range d.Alerts {
		fmt.Fprintf(b, "> [!WARNING]\n> %s\n\n", a)
	}
	if len(d.Nodes) > 0 {
		b.WriteString("| Node | | Monthly | Change | Tightest headroom |\n| --- | --- | ---: | ---: | ---: |\n")
		for _, n := range d.Nodes {
			fmt.Fprintf(b, "| `%s` %s | %s | %s | %s | %s |\n", n.ID, n.Label, n.Change,
				span(n.Change, usd(n.BeforeUSD), usd(n.AfterUSD)), delta(n.BeforeUSD, n.AfterUSD),
				span(n.Change, pct(n.BeforeHeadroom), pct(n.AfterHeadroom)))
		}
		b.WriteString("\n<details><summary>Cost lines</summary>\n\n| Node | Component | Before | After |\n| --- | --- | ---: | ---: |\n")
		for _, n := range d.Nodes {
			for _, l := range n.Lines {
				fmt.Fprintf(b, "| `%s` | %s | %s | %s |\n", n.ID, l.Name, lineUSD(l.Before), lineUSD(l.After))
			}
		}
		b.WriteString("\n</details>\n")
	}
	if len(d.Paths) > 0 {
		b.WriteString("\n<details><summary>Paths</summary>\n\n| Path | | p99 | Availability |\n| --- | --- | ---: | ---: |\n")
		for _, p := range d.Paths {
			fmt.Fprintf(b, "| %s | %s | %s | %s |\n", p.Path, p.Change, pathSpan(p, func(r engine.PathResult) string { return num(r.P99Ms) + " ms" }),
				pathSpan(p, func(r engine.PathResult) string { return availability(r.Availability) }))
		}
		b.WriteString("\n</details>\n")
	}
	_, err := io.WriteString(w, b.String())
	return err
}

func span(change, before, after string) string {
	switch change {
	case "added":
		return after
	case "removed":
		return before
	}
	if before == after {
		return after
	}
	return before + " → " + after
}

func pathSpan(p PathChange, f func(engine.PathResult) string) string {
	switch {
	case p.Before == nil:
		return f(*p.After)
	case p.After == nil:
		return f(*p.Before)
	}
	return span("changed", f(*p.Before), f(*p.After))
}

func lineUSD(v *float64) string {
	if v == nil {
		return "-"
	}
	return usd(*v)
}

// delta writes a signed change with its percentage when there is a base.
func delta(before, after float64) string {
	d := after - before
	if math.Abs(d) < 0.005 {
		return "±$0.00"
	}
	sign := "+"
	if d < 0 {
		sign = "−"
	}
	s := sign + "$" + strconv.FormatFloat(math.Abs(d), 'f', 2, 64)
	if before >= 0.005 {
		s += fmt.Sprintf(", %s%s%%", sign, strconv.FormatFloat(math.Abs(d)/before*100, 'f', 1, 64))
	}
	return s
}
