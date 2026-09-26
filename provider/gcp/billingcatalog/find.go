package billingcatalog

import (
	"encoding/json"
	"fmt"
	"maps"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Find returns the skus sold in region that match filters (usageType
// defaults to OnDemand).
func Find(skus []Sku, region string, filters map[string]string) ([]Sku, error) {
	f := map[string]string{"usageType": "OnDemand"}
	maps.Copy(f, filters)
	res := map[string]*regexp.Regexp{}
	for k, v := range f {
		re, err := regexp.Compile("^(?:" + v + ")$")
		if err != nil {
			return nil, fmt.Errorf("filter %s: %w", k, err)
		}
		res[k] = re
	}
	var out []Sku
	for _, s := range skus {
		if (region == "" || s.In(region)) && matches(s, res) {
			out = append(out, s)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Description+out[i].SkuID < out[j].Description+out[j].SkuID })
	return out, nil
}

func matches(s Sku, res map[string]*regexp.Regexp) bool {
	for k, re := range res {
		if !re.MatchString(s.Field(k)) {
			return false
		}
	}
	return true
}

// Price is a resolved sku tier.
type Price struct {
	Sku  Sku
	Rate Rate
}

// matching returns the skus of the spec's service that match its filters in
// any region, remembered per service and filters.
func (c *Client) matching(spec Spec) ([]Sku, error) {
	key, err := json.Marshal([]any{spec.Service, spec.Filters})
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if m, ok := c.matched[string(key)]; ok {
		return m, nil
	}
	skus, err := c.service(spec.Service)
	if err != nil {
		return nil, err
	}
	m, err := Find(skus, "", spec.Filters)
	if err != nil {
		return nil, err
	}
	if c.matched == nil {
		c.matched = map[string][]Sku{}
	}
	c.matched[string(key)] = m
	return m, nil
}

// service returns the skus of a service, remembered once read. The caller
// holds c.mu.
func (c *Client) service(service string) ([]Sku, error) {
	if skus, ok := c.skus[service]; ok {
		return skus, nil
	}
	skus, err := c.Skus(service)
	if err != nil {
		return nil, err
	}
	if c.skus == nil {
		c.skus = map[string][]Sku{}
	}
	c.skus[service] = skus
	return skus, nil
}

// Resolve finds exactly one price for a spec, or explains why it cannot.
func (c *Client) Resolve(spec Spec, region string) (Price, error) {
	all, err := c.matching(spec)
	if err != nil {
		return Price{}, err
	}
	var ms []Sku
	for _, s := range all {
		if s.In(spec.For(region)) {
			ms = append(ms, s)
		}
	}
	switch len(ms) {
	case 0:
		return Price{}, fmt.Errorf("%w: no sku matches %v in %s", ErrAbsent, spec.Filters, spec.For(region))
	case 1:
	default:
		var names []string
		for _, m := range ms {
			names = append(names, m.Description)
		}
		return Price{}, fmt.Errorf("%d skus match %v: %s", len(ms), spec.Filters, strings.Join(names, "; "))
	}
	rate, err := pickTier(ms[0].Rates(), spec.Tier, spec.ListPer)
	if err != nil {
		return Price{}, err
	}
	return Price{Sku: ms[0], Rate: rate}, nil
}

func pickTier(rates []Rate, tier string, listPer float64) (Rate, error) {
	if len(rates) == 0 {
		return Rate{}, fmt.Errorf("the sku has no price")
	}
	switch tier {
	case "":
		return firstPriced(rates), nil
	case "first":
		return rates[0], nil
	case "last":
		return rates[len(rates)-1], nil
	}
	want, err := strconv.ParseFloat(tier, 64)
	if err != nil {
		return Rate{}, fmt.Errorf("tier %q: want first, last or where the tier starts", tier)
	}
	if listPer == 0 {
		listPer = 1
	}
	in, ok := rateAt(rates, want, listPer)
	if !ok {
		return Rate{}, fmt.Errorf("no tier is in effect at %s", tier)
	}
	return in, nil
}

// firstPriced is the first tier with a price: a free grant comes first at
// zero. With no priced tier it is the first.
func firstPriced(rates []Rate) Rate {
	for _, r := range rates {
		if r.UnitPrice.USD() > 0 {
			return r
		}
	}
	return rates[0]
}

// rateAt is the tier in effect at a usage in the book's units, whose tier
// starts are startUsageAmount × listPer. A region whose SKU does not break
// exactly there bills the rate in effect at that point.
func rateAt(rates []Rate, usage, listPer float64) (Rate, bool) {
	var in *Rate
	for i, r := range rates {
		if r.Start*listPer <= usage*(1+1e-12) && (in == nil || r.Start >= in.Start) {
			in = &rates[i]
		}
	}
	if in == nil {
		return Rate{}, false
	}
	return *in, true
}
