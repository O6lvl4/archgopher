package book

import (
	"fmt"
	"sort"
	"strconv"
)

// Flatten turns every table into one entry per row, keyed "<id>.<row>". A
// tiered table also keeps an entry under its own id, which carries the
// pricing rules and the tiers but no values.
func (b Book) Flatten() (Book, error) {
	out := Book{}
	for id, e := range b {
		if err := e.checkRules(id); err != nil {
			return nil, err
		}
		if len(e.Rows) == 0 {
			out[id] = e
			continue
		}
		if err := b.flattenTable(id, e, out); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// flattenTable adds table e's rows to out, and for a tiered table the entry
// that carries its rules.
func (b Book) flattenTable(id string, e Entry, out Book) error {
	if len(e.Values) > 0 {
		return fmt.Errorf("%q has both values and rows", id)
	}
	if e.Tiered {
		tiers, err := tiersOf(id, e.Rows)
		if err != nil {
			return err
		}
		out[id] = Entry{Unit: e.Unit, Per: e.Per, Source: e.Source, Note: e.Note, Pool: e.Pool, Included: e.Included, Combine: e.Combine, Tiered: true, Tiers: tiers, Verified: e.Verified, CheckedAt: e.CheckedAt}
	}
	for key, row := range e.Rows {
		full := id + "." + key
		if _, dup := b[full]; dup {
			return fmt.Errorf("%q is both an entry and a row of %q", full, id)
		}
		out[full] = e.row(row)
	}
	return nil
}

// row is the entry for one row of table e.
func (e Entry) row(prices map[string]*float64) Entry {
	values := map[string]Value{}
	for region, v := range prices {
		values[region] = Value{Value: v, Verified: e.Verified, CheckedAt: e.CheckedAt}
	}
	row := Entry{Unit: e.Unit, Per: e.Per, Source: e.Source, Note: e.Note, Currency: e.Currency, Tax: e.Tax, Values: values}
	if !e.Tiered {
		// The rules hold for each row on its own: one pool per row.
		row.Pool, row.Included, row.Combine = e.Pool, e.Included, e.Combine
	}
	return row
}

func (e Entry) checkRules(id string) error {
	switch {
	case e.Pool != "" && e.Pool != PoolAccount && e.Pool != PoolRegion:
		return fmt.Errorf("%q: pool is %q, not %q or %q", id, e.Pool, PoolAccount, PoolRegion)
	case e.Combine != "" && e.Combine != CombineMax:
		return fmt.Errorf("%q: combine is %q, not %q", id, e.Combine, CombineMax)
	case e.Combine != "" && e.Pool == "":
		return fmt.Errorf("%q: combine needs a pool", id)
	case e.Included < 0:
		return fmt.Errorf("%q: included must not be negative", id)
	case e.Tax < 0 || e.Tax >= 1:
		return fmt.Errorf("%q: tax is a rate from 0 to 1, not %v", id, e.Tax)
	case e.Tax > 0 && e.Currency == "":
		return fmt.Errorf("%q: tax needs the currency it was published in", id)
	case e.Tiered && len(e.Rows) == 0:
		return fmt.Errorf("%q: tiered needs rows", id)
	}
	return nil
}

func tiersOf(id string, rows map[string]map[string]*float64) ([]Tier, error) {
	tiers := make([]Tier, 0, len(rows))
	for key := range rows {
		from, err := strconv.ParseFloat(key, 64)
		if err != nil || from < 0 {
			return nil, fmt.Errorf("%q is tiered, so row %q must be where its tier starts", id, key)
		}
		tiers = append(tiers, Tier{From: from, ID: id + "." + key})
	}
	sort.Slice(tiers, func(i, j int) bool { return tiers[i].From < tiers[j].From })
	if tiers[0].From != 0 {
		return nil, fmt.Errorf("%q is tiered, so its first row must start at 0", id)
	}
	return tiers, nil
}
