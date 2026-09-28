package report

import (
	"fmt"
	"html"
	"io"
	"strings"

	"github.com/O6lvl4/archgopher/engine"
)

// HTML writes the result as one page: the diagram first, then the tables of
// the Markdown report. The page is self-contained; the diagram is inline.
func HTML(w io.Writer, r engine.Result, svg []byte) error {
	b := &strings.Builder{}
	b.WriteString("<!doctype html>\n<html lang=\"en\">\n<head>\n<meta charset=\"utf-8\">\n<meta name=\"viewport\" content=\"width=device-width, initial-scale=1\">\n")
	fmt.Fprintf(b, "<title>%s</title>\n<style>%s</style>\n</head>\n<body>\n<main>\n", h(orDash(r.Name)), htmlStyle)
	fmt.Fprintf(b, "<h1>%s <span class=\"muted\">%s</span></h1>\n", h(orDash(r.Name)), h(r.Region))
	fmt.Fprintf(b, "<p class=\"total\">Monthly cost <strong>%s</strong>", h(usd(r.MonthlyUSD)))
	if r.UnpricedCosts > 0 {
		fmt.Fprintf(b, " <span class=\"muted\">plus %d cost lines with unknown prices</span>", r.UnpricedCosts)
	}
	b.WriteString("</p>\n")
	if len(svg) > 0 {
		s := string(svg)
		if i := strings.Index(s, "<svg"); i > 0 {
			s = s[i:] // drop the XML declaration; the page has its own
		}
		b.WriteString("<figure class=\"diagram\">\n" + s + "</figure>\n")
	}
	htmlNodes(b, r)
	htmlLoads(b, r)
	htmlCosts(b, r)
	htmlLimits(b, r)
	htmlPaths(b, r)
	htmlUnverified(b, r)
	htmlProblems(b, r)
	b.WriteString("</main>\n</body>\n</html>\n")
	_, err := io.WriteString(w, b.String())
	return err
}

const htmlStyle = `
body { margin: 0; background: #f7f8f7; color: #16191f; font: 14px/1.6 "Helvetica Neue", Helvetica, Arial, "Hiragino Sans", "Noto Sans JP", sans-serif; }
main { max-width: 1400px; margin: 0 auto; padding: 32px 24px 64px; }
h1 { font-size: 22px; margin: 0 0 4px; } h2 { font-size: 16px; margin: 32px 0 8px; }
.muted { color: #6b7580; font-weight: 400; } .total { margin: 0 0 20px; font-size: 15px; }
figure.diagram { margin: 0; background: #fff; border: 1px solid #d5dad3; border-radius: 8px; padding: 8px; overflow-x: auto; }
figure.diagram svg { display: block; max-width: 100%; height: auto; }
p.note { margin: 0 0 8px; color: #6b7580; }
table { border-collapse: collapse; width: 100%; background: #fff; border: 1px solid #d5dad3; font-size: 13px; }
th, td { padding: 6px 10px; border-top: 1px solid #e3e7e2; text-align: left; vertical-align: top; }
th { background: #eef1ec; font-size: 11px; letter-spacing: .06em; text-transform: uppercase; color: #545b64; border-top: none; }
td.n, th.n { text-align: right; font-variant-numeric: tabular-nums; }
code { font: 12px "SF Mono", Menlo, Consolas, "Liberation Mono", monospace; }
ul.problems { margin: 0; padding-left: 20px; }
`

func h(s string) string { return html.EscapeString(s) }

func htmlNodes(b *strings.Builder, r engine.Result) {
	b.WriteString("<h2>Nodes</h2>\n")
	if len(members(r))-len(r.Groups) < len(r.Nodes) {
		b.WriteString("<p class=\"note\">Pattern rows sum the nodes they expand into.</p>\n")
	}
	b.WriteString("<table><tr><th>Node</th><th>Type</th><th class=\"n\">Monthly</th><th class=\"n\">Tightest headroom</th><th class=\"n\">p99</th><th class=\"n\">SLA</th><th>Status</th></tr>\n")
	for _, n := range nodesAndGroups(r) {
		fmt.Fprintf(b, "<tr><td><code>%s</code></td><td>%s</td><td class=\"n\">%s</td><td class=\"n\">%s</td><td class=\"n\">%s</td><td class=\"n\">%s</td><td>%s</td></tr>\n",
			h(n.ID), h(label(n)), h(usd(n.MonthlyUSD)), h(pct(n.MinHeadroom())), h(latency(n.Latency)), h(sla(n.SLA)), h(status(n)))
	}
	b.WriteString("</table>\n")
}

func htmlLoads(b *strings.Builder, r engine.Result) {
	var arrivals []engine.NodeResult
	for _, n := range r.Nodes {
		if n.Load != nil {
			arrivals = append(arrivals, n)
		}
	}
	if len(arrivals) == 0 {
		return
	}
	b.WriteString("<h2>Load in</h2>\n<table><tr><th>Node</th><th class=\"n\">Monthly</th><th class=\"n\">Peak / s</th><th>How</th></tr>\n")
	for _, n := range arrivals {
		fmt.Fprintf(b, "<tr><td><code>%s</code></td><td class=\"n\">%s</td><td class=\"n\">%s</td><td>%s</td></tr>\n", h(n.ID), h(num(n.Load.Monthly)), h(num(n.Load.PeakPerSecond)), h(orGiven(n.LoadBasis)))
	}
	b.WriteString("</table>\n")
}

func htmlCosts(b *strings.Builder, r engine.Result) {
	b.WriteString("<h2>Cost lines</h2>\n<table><tr><th>Node</th><th>Component</th><th class=\"n\">Quantity</th><th>Unit</th><th class=\"n\">Unit price</th><th class=\"n\">Monthly</th></tr>\n")
	for _, n := range members(r) {
		for _, c := range n.Costs {
			fmt.Fprintf(b, "<tr><td><code>%s</code></td><td>%s</td><td class=\"n\">%s</td><td>%s</td><td class=\"n\">%s</td><td class=\"n\">%s</td></tr>\n", h(n.ID), h(c.Name), h(num(c.Quantity)), h(c.Unit), h(price(c.UnitPrice)), h(usdPtr(c.MonthlyUSD)))
		}
	}
	b.WriteString("</table>\n")
}

func htmlLimits(b *strings.Builder, r engine.Result) {
	b.WriteString("<h2>Limits</h2>\n<p class=\"note\">Capacities are the published defaults, unless marked as the account's own.</p>\n<table><tr><th>Node</th><th>Limit</th><th class=\"n\">Peak demand</th><th class=\"n\">Capacity</th><th>Unit</th><th class=\"n\">Headroom</th></tr>\n")
	for _, n := range members(r) {
		for _, l := range n.Limits {
			fmt.Fprintf(b, "<tr><td><code>%s</code></td><td>%s</td><td class=\"n\">%s</td><td class=\"n\">%s</td><td>%s</td><td class=\"n\">%s</td></tr>\n", h(n.ID), h(l.Name), h(num(l.Demand)), h(capacity(l)), h(l.Unit), h(pct(l.Headroom)))
		}
	}
	b.WriteString("</table>\n")
}

func htmlPaths(b *strings.Builder, r engine.Result) {
	if len(r.Paths) == 0 {
		return
	}
	b.WriteString("<h2>Paths</h2>\n<p class=\"note\">p99 adds up the p99 of every hop, so it is an upper bound.</p>\n<table><tr><th>Path</th><th class=\"n\">p50</th><th class=\"n\">p99</th><th class=\"n\">Availability</th><th>Missing</th></tr>\n")
	for _, p := range r.Paths {
		fmt.Fprintf(b, "<tr><td><code>%s</code></td><td class=\"n\">%s ms</td><td class=\"n\">%s ms</td><td class=\"n\">%s</td><td>%s</td></tr>\n", h(strings.Join(p.Nodes, " → ")), h(num(p.P50Ms)), h(num(p.P99Ms)), h(availability(p.Availability)), h(missing(p)))
	}
	b.WriteString("</table>\n")
}

func htmlUnverified(b *strings.Builder, r engine.Result) {
	if len(r.Unverified) == 0 {
		return
	}
	b.WriteString("<h2>Unverified reference values</h2>\n<p class=\"note\">Nobody has checked these against the source yet, or the value is unknown.</p>\n<table><tr><th>Book</th><th>ID</th><th>Region</th><th>State</th><th>Source</th></tr>\n")
	for _, u := range r.Unverified {
		state := "unverified"
		if !u.Known {
			state = "unknown"
		}
		fmt.Fprintf(b, "<tr><td>%s</td><td><code>%s</code></td><td>%s</td><td>%s</td><td>%s</td></tr>\n", h(string(u.Book)), h(u.ID), h(u.Region), state, h(u.Source))
	}
	b.WriteString("</table>\n")
}

func htmlProblems(b *strings.Builder, r engine.Result) {
	var items []string
	for _, n := range nodesAndGroups(r) {
		if n.Error != "" {
			items = append(items, fmt.Sprintf("<li><code>%s</code>: %s</li>", h(n.ID), h(n.Error)))
		}
		if n.Skipped != "" {
			items = append(items, fmt.Sprintf("<li><code>%s</code>: skipped, %s</li>", h(n.ID), h(n.Skipped)))
		}
		if n.Stale {
			items = append(items, fmt.Sprintf("<li><code>%s</code>: no longer in Terraform (%s)</li>", h(n.ID), h(n.Address)))
		}
	}
	for _, w := range r.Warnings {
		items = append(items, "<li>"+h(w)+"</li>")
	}
	if len(items) > 0 {
		b.WriteString("<h2>Problems</h2>\n<ul class=\"problems\">\n" + strings.Join(items, "\n") + "\n</ul>\n")
	}
}
