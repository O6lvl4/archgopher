package reconcile

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// JSON writes the report as indented JSON.
func JSON(w io.Writer, r Report) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(r)
}

// Markdown writes the report as tables: the split of the bill first, then
// the services side by side, then what each side has alone.
func Markdown(w io.Writer, r Report) error {
	b := &strings.Builder{}
	fmt.Fprintf(b, "# %s against the bill (%s)\n\n", r.Name, r.Period)
	b.WriteString("| | USD |\n| --- | ---: |\n")
	fmt.Fprintf(b, "| Bill | %s |\n", usd(r.BillUSD))
	fmt.Fprintf(b, "| of it, services the declaration has nodes for | %s |\n", usd(r.CoveredUSD))
	fmt.Fprintf(b, "| of it, outside the declaration | %s |\n", usd(r.OutsideUSD))
	if r.TaxUSD != 0 {
		fmt.Fprintf(b, "| of it, tax | %s |\n", usd(r.TaxUSD))
	}
	fmt.Fprintf(b, "| Declaration | %s |\n", usd(r.DeclaredUSD))
	if r.CoveredUSD != 0 {
		fmt.Fprintf(b, "| Declaration against what it covers | %+.1f%% |\n", (r.DeclaredUSD/r.CoveredUSD-1)*100)
	}
	b.WriteString("\n## By service\n\n| Service | Bill | Declaration | Difference | Nodes |\n| --- | ---: | ---: | ---: | --- |\n")
	for _, row := range r.Rows {
		fmt.Fprintf(b, "| %s | %s | %s | %s | %s |\n", row.Service, usd(row.BillUSD), usd(row.DeclaredUSD), diff(row), strings.Join(row.Nodes, ", "))
	}
	if len(r.Outside) > 0 {
		b.WriteString("\n## Billed, no node stands for it\n\nWhat a platform team, another declaration or people's own actions add to the account. Not the declaration's error.\n\n| Service | Bill |\n| --- | ---: |\n")
		var under []string
		for _, row := range r.Outside {
			if row.BillUSD < 0.005 && row.BillUSD > -0.005 {
				under = append(under, row.Service)
				continue
			}
			fmt.Fprintf(b, "| %s | %s |\n", row.Service, usd(row.BillUSD))
		}
		if len(under) > 0 {
			fmt.Fprintf(b, "\nUnder a cent: %s.\n", strings.Join(under, ", "))
		}
	}
	if len(r.Unmapped) > 0 {
		b.WriteString("\n## Lines without a billing service\n\nTheir price names no service code the bill is known by; add one to the provider's billing names.\n\n| Node | Line | Price | Declaration |\n| --- | --- | --- | ---: |\n")
		for _, u := range r.Unmapped {
			fmt.Fprintf(b, "| %s | %s | %s | %s |\n", u.Node, u.Line, u.PriceID, usd(u.USD))
		}
	}
	_, err := io.WriteString(w, b.String())
	return err
}

// diff is the declaration against the bill for one service. Under a cent on
// both sides there is nothing to say; a service the bill does not show
// (covered by a savings plan, or not used in the period) has no percentage.
func diff(row Row) string {
	d := row.DeclaredUSD - row.BillUSD
	switch {
	case cents(row.BillUSD) == 0 && cents(row.DeclaredUSD) == 0:
		return "-"
	case cents(row.BillUSD) == 0:
		return usd(d) + " (not billed)"
	}
	return fmt.Sprintf("%s (%+.0f%%)", usd(d), d/row.BillUSD*100)
}

func cents(v float64) int64 { return int64(v*100 + copysign(0.5, v)) }

func copysign(x, sign float64) float64 {
	if sign < 0 {
		return -x
	}
	return x
}

func usd(v float64) string {
	c := cents(v)
	if c < 0 {
		return fmt.Sprintf("-$%d.%02d", -c/100, -c%100)
	}
	return fmt.Sprintf("$%d.%02d", c/100, c%100)
}
