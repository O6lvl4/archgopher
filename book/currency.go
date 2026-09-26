package book

import (
	"fmt"
	"strings"
)

// RatePrefix starts the id of a currency's rate: "fx.jpy" is how many US
// dollars one yen buys. A rate is a price entry like any other, with a
// source and the day someone checked it.
const RatePrefix = "fx."

// InUSD states every price in US dollars. A value published in another
// currency is taken out of the tax it includes and converted at the book's
// rate for that currency; it stays verified only when the rate is. Entries
// in US dollars are returned as they are.
func (b Book) InUSD() (Book, error) {
	out := make(Book, len(b))
	for id, e := range b {
		if e.Currency == "" || e.Currency == "USD" {
			out[id] = e
			continue
		}
		rate, err := b.rate(e.Currency)
		if err != nil {
			return nil, fmt.Errorf("%q: %w", id, err)
		}
		out[id] = e.converted(rate)
	}
	return out, nil
}

// rate is the value of the currency's rate entry.
func (b Book) rate(currency string) (Value, error) {
	id := RatePrefix + strings.ToLower(currency)
	e, v, err := b.Lookup(id, AnyRegion)
	switch {
	case err != nil:
		return Value{}, fmt.Errorf("priced in %s, but %w", currency, err)
	case e.Unit != currency:
		return Value{}, fmt.Errorf("rate %q is per %q, not per %s", id, e.Unit, currency)
	case v.Value == nil || *v.Value <= 0:
		return Value{}, fmt.Errorf("rate %q has no value", id)
	}
	return v, nil
}

// converted is e with its values in US dollars before tax.
func (e Entry) converted(rate Value) Entry {
	values := make(map[string]Value, len(e.Values))
	for region, v := range e.Values {
		if v.Value != nil {
			usd := *v.Value / (1 + e.Tax) * *rate.Value
			v.Value = &usd
		}
		v.Verified = v.Verified && rate.Verified
		values[region] = v
	}
	e.Values, e.Currency, e.Tax = values, "", 0
	return e
}
