package retailprices

import (
	"fmt"
	"maps"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Find returns the items that match filters (type defaults to Consumption).
func Find(items []Item, filters map[string]string) ([]Item, error) {
	f := map[string]string{"type": "Consumption"}
	maps.Copy(f, filters)
	res := map[string]*regexp.Regexp{}
	for k, v := range f {
		re, err := regexp.Compile("^(?:" + v + ")$")
		if err != nil {
			return nil, fmt.Errorf("filter %s: %w", k, err)
		}
		res[k] = re
	}
	var out []Item
	for _, it := range items {
		if matches(it, res) {
			out = append(out, it)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.MeterID != b.MeterID {
			return a.MeterName+a.MeterID < b.MeterName+b.MeterID
		}
		return a.TierMinimum < b.TierMinimum
	})
	return out, nil
}

func matches(it Item, res map[string]*regexp.Regexp) bool {
	for k, re := range res {
		if !re.MatchString(it.Field(k)) {
			return false
		}
	}
	return true
}

// Resolve finds exactly one price for a spec, or explains why it cannot.
func (c *Client) Resolve(spec Spec, region string) (Item, error) {
	items, err := c.Items(spec.Service, spec.For(region))
	if err != nil {
		return Item{}, err
	}
	ms, err := Find(items, spec.Filters)
	if err != nil {
		return Item{}, err
	}
	// Every match is then a tier of the one meter.
	switch meters := metersOf(ms); len(meters) {
	case 0:
		return Item{}, fmt.Errorf("%w: no price matches %v", ErrAbsent, spec.Filters)
	case 1:
		return pickTier(ms, spec.Tier, spec.ListPer)
	default:
		return Item{}, fmt.Errorf("%d meters match %v: %s", len(meters), spec.Filters, strings.Join(meters, "; "))
	}
}

// metersOf names every meter among the matches, once each, sorted.
func metersOf(ms []Item) []string {
	seen := map[string]bool{}
	var out []string
	for _, m := range ms {
		if !seen[m.MeterID] {
			seen[m.MeterID] = true
			out = append(out, m.ProductName+" / "+m.SkuName+" / "+m.MeterName)
		}
	}
	sort.Strings(out)
	return out
}

func pickTier(list []Item, tier string, listPer float64) (Item, error) {
	if listPer == 0 {
		listPer = 1
	}
	switch tier {
	case "":
		for _, it := range list {
			if it.RetailPrice > 0 {
				return it, nil
			}
		}
		return list[0], nil
	case "last":
		return list[len(list)-1], nil
	case "first":
		return list[0], nil
	}
	want, err := strconv.ParseFloat(tier, 64)
	if err != nil {
		return Item{}, fmt.Errorf("tier %q: want first, last or where the tier starts", tier)
	}
	for _, it := range list {
		if it.TierMinimum*listPer == want {
			return it, nil
		}
	}
	return Item{}, fmt.Errorf("no tier starts at %s", tier)
}
