package meter

import (
	"sort"

	"github.com/O6lvl4/archgopher/book"
)

// Pool is a price the provider bills on what the whole account uses: its
// tiers are counted and its included units given once, over every line that
// reads it.
type Pool struct {
	Key     string `json:"key"`
	PriceID string `json:"priceId"`
	Region  string `json:"region"`
	Unit    string `json:"unit"`
	// Quantity is what the pool is billed on: the sum of its lines, or the
	// largest of them for a fee paid once.
	Quantity   float64  `json:"quantity"`
	Bands      []Band   `json:"bands"`
	MonthlyUSD *float64 `json:"monthlyUsd"`
	// Error says why the pool could not be billed; its lines are unpriced.
	Error   string       `json:"error,omitempty"`
	Members []PoolMember `json:"members"`
}

// PoolMember is one line of a pool and the quantity it brings.
type PoolMember struct {
	Node     string  `json:"node"`
	Line     string  `json:"line"`
	Quantity float64 `json:"quantity"`
}

// Owned is a cost line and the node it belongs to.
type Owned struct {
	Node string
	Cost *Cost
}

// Share bills every pool among lines once and shares its cost out over its
// lines by quantity, so each pays the pool's average price.
func Share(lines []Owned, prices book.Book, region string) []Pool {
	byKey := map[string][]Owned{}
	var keys []string
	for _, l := range lines {
		if l.Cost.Pool == "" {
			continue
		}
		if _, seen := byKey[l.Cost.Pool]; !seen {
			keys = append(keys, l.Cost.Pool)
		}
		byKey[l.Cost.Pool] = append(byKey[l.Cost.Pool], l)
	}
	sort.Strings(keys)
	pools := make([]Pool, len(keys))
	sums := make([]float64, len(keys))
	for i, key := range keys {
		first := byKey[key][0].Cost
		e := prices[first.PriceID]
		p := Pool{Key: key, PriceID: first.PriceID, Region: region, Unit: first.Unit}
		for _, l := range byKey[key] {
			q := l.Cost.Quantity
			sums[i] += q
			if e.Combine == book.CombineMax {
				p.Quantity = max(p.Quantity, q)
			} else {
				p.Quantity += q
			}
			p.Members = append(p.Members, PoolMember{Node: l.Node, Line: l.Cost.Name, Quantity: q})
		}
		pools[i] = p
	}
	for i := range pools {
		p := &pools[i]
		bill, err := Bill(prices, p.PriceID, region, p.Quantity, prices[p.PriceID].Included)
		if err != nil {
			p.Error = err.Error()
		}
		p.Bands, p.MonthlyUSD = bill.Bands, bill.USD
		for _, l := range byKey[p.Key] {
			l.Cost.Bands = nil
			l.Cost.UnitPrice, l.Cost.MonthlyUSD = nil, nil
			if bill.USD == nil {
				continue
			}
			share := 1 / float64(len(byKey[p.Key]))
			if sums[i] > 0 {
				share = l.Cost.Quantity / sums[i]
			}
			m := *bill.USD * share
			l.Cost.MonthlyUSD = &m
			if l.Cost.Quantity > 0 {
				u := m / l.Cost.Quantity
				l.Cost.UnitPrice = &u
			}
		}
	}
	return pools
}
