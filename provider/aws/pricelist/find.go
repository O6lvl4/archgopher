package pricelist

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Find returns every priced dimension of the products that match filters.
func (o *Offer) Find(filters map[string]string) ([]Match, error) {
	res := map[string]*regexp.Regexp{}
	for k, v := range filters {
		re, err := regexp.Compile("^(?:" + v + ")$")
		if err != nil {
			return nil, fmt.Errorf("filter %s: %w", k, err)
		}
		res[k] = re
	}
	var out []Match
	for sku, p := range o.Products {
		if !matches(p, res) {
			continue
		}
		for _, term := range o.Terms.OnDemand[sku] {
			for _, d := range term.PriceDimensions {
				usd, err := strconv.ParseFloat(d.PricePerUnit["USD"], 64)
				if err != nil {
					continue
				}
				out = append(out, Match{Product: p, Dimension: d, USD: usd})
			}
		}
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.Product.SKU != b.Product.SKU {
			return a.Product.Attributes["usagetype"]+a.Product.SKU < b.Product.Attributes["usagetype"]+b.Product.SKU
		}
		return begin(a.Dimension) < begin(b.Dimension)
	})
	return out, nil
}

func matches(p Product, res map[string]*regexp.Regexp) bool {
	for k, re := range res {
		v := p.Attributes[k]
		if k == "productFamily" {
			v = p.ProductFamily
		}
		if !re.MatchString(v) {
			return false
		}
	}
	return true
}

func begin(d Dimension) float64 {
	v, err := strconv.ParseFloat(d.BeginRange, 64)
	if err != nil {
		return 0
	}
	return v
}

// filtersFor puts the region being resolved in place of "{region}", for
// offers that list prices in both directions (data transfer between regions
// is listed in the offer of either end, so "from this region" needs the name).
func (s Spec) filtersFor(region string) map[string]string {
	out := make(map[string]string, len(s.Filters))
	for k, v := range s.Filters {
		out[k] = strings.ReplaceAll(v, "{region}", regexp.QuoteMeta(region))
	}
	return out
}

// Resolve finds exactly one price for a spec, or explains why it cannot.
func (c *Client) Resolve(spec Spec, region string) (Match, error) {
	filters := spec.filtersFor(region)
	o, err := c.Offer(spec.Service, spec.For(region))
	if err != nil {
		return Match{}, err
	}
	ms, err := o.Find(filters)
	if err != nil {
		return Match{}, err
	}
	// Every match is then a tier of the one product.
	switch usagetypes := productsOf(ms); len(usagetypes) {
	case 0:
		return Match{}, fmt.Errorf("%w: no product matches %v", ErrAbsent, filters)
	case 1:
		return pickTier(ms, spec.Tier)
	default:
		return Match{}, fmt.Errorf("%d products match %v: %s", len(usagetypes), filters, strings.Join(usagetypes, ", "))
	}
}

// productsOf is the usage type of every product among the matches, one per
// SKU, sorted.
func productsOf(ms []Match) []string {
	seen := map[string]bool{}
	var out []string
	for _, m := range ms {
		if !seen[m.Product.SKU] {
			seen[m.Product.SKU] = true
			out = append(out, m.Product.Attributes["usagetype"])
		}
	}
	sort.Strings(out)
	return out
}

func pickTier(list []Match, tier string) (Match, error) {
	switch tier {
	case "", "first":
		return list[0], nil
	case "last":
		return list[len(list)-1], nil
	}
	want, err := strconv.ParseFloat(tier, 64)
	if err != nil {
		return Match{}, fmt.Errorf("tier %q: want first, last or a beginRange", tier)
	}
	for _, m := range list {
		if begin(m.Dimension) == want {
			return m, nil
		}
	}
	return Match{}, fmt.Errorf("no tier begins at %s", tier)
}
