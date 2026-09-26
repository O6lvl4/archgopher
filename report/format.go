package report

import (
	"strconv"
	"strings"

	"github.com/O6lvl4/archgopher/engine"
	"github.com/O6lvl4/archgopher/meter"
)

func bands(list []meter.Band) string {
	parts := make([]string, 0, len(list))
	for _, x := range list {
		if x.Included {
			parts = append(parts, num(x.Quantity)+" included")
			continue
		}
		parts = append(parts, num(x.Quantity)+" at "+price(x.UnitPrice))
	}
	return orDash(strings.Join(parts, ", "))
}

func poolLines(p meter.Pool) string {
	parts := make([]string, 0, len(p.Members))
	for _, m := range p.Members {
		parts = append(parts, m.Node+" ("+m.Line+")")
	}
	return strings.Join(parts, ", ")
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

// orGiven names how a load was worked out; a load written as is was given.
func orGiven(basis string) string {
	if basis == "" {
		return "given"
	}
	return basis
}

// usd writes dollars to the cent; an amount above zero that rounds to $0.00
// is written <$0.01, so it never reads as free.
func usd(v float64) string {
	if v > 0 && v < 0.005 {
		return "<$0.01"
	}
	return "$" + strconv.FormatFloat(v, 'f', 2, 64)
}

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

// capacity writes a limit's capacity, marking the account's own value.
func capacity(l meter.Limit) string {
	if l.From == "account" {
		return numPtr(l.Capacity) + " (account)"
	}
	return numPtr(l.Capacity)
}
