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
	byKey, keys := byPool(lines)
	pools := make([]Pool, len(keys))
	for i, key := range keys {
		members := byKey[key]
		pools[i] = newPool(key, members, prices, region)
		bill, err := Bill(prices, pools[i].PriceID, region, pools[i].Quantity, prices[pools[i].PriceID].Included)
		if err != nil {
			pools[i].Error = err.Error()
		}
		pools[i].Bands, pools[i].MonthlyUSD = bill.Bands, bill.USD
		shareOut(members, bill.USD)
	}
	return pools
}

// byPool groups the pooled lines by pool key and returns the keys in order.
func byPool(lines []Owned) (map[string][]Owned, []string) {
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
	return byKey, keys
}

// newPool is the pool of members before it is billed: its quantity is their
// sum, or the largest of them for a fee paid once.
func newPool(key string, members []Owned, prices book.Book, region string) Pool {
	first := members[0].Cost
	combine := prices[first.PriceID].Combine
	p := Pool{Key: key, PriceID: first.PriceID, Region: region, Unit: first.Unit}
	for _, l := range members {
		q := l.Cost.Quantity
		if combine == book.CombineMax {
			p.Quantity = max(p.Quantity, q)
		} else {
			p.Quantity += q
		}
		p.Members = append(p.Members, PoolMember{Node: l.Node, Line: l.Cost.Name, Quantity: q})
	}
	return p
}

// shareOut prices each member at its share of the pool's usd by quantity, or
// an equal share when none has quantity. An unknown usd leaves them unpriced.
func shareOut(members []Owned, usd *float64) {
	var sum float64
	for _, l := range members {
		sum += l.Cost.Quantity
	}
	for _, l := range members {
		l.Cost.Bands = nil
		l.Cost.UnitPrice, l.Cost.MonthlyUSD = nil, nil
		if usd == nil {
			continue
		}
		share := 1 / float64(len(members))
		if sum > 0 {
			share = l.Cost.Quantity / sum
		}
		m := *usd * share
		l.Cost.MonthlyUSD = &m
		if l.Cost.Quantity > 0 {
			u := m / l.Cost.Quantity
			l.Cost.UnitPrice = &u
		}
	}
}
