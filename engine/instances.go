package engine

import (
	"github.com/O6lvl4/archgopher/book"
	"github.com/O6lvl4/archgopher/meter"
	"github.com/O6lvl4/archgopher/model"
)

// share is the part of demand d that one of k identical resources carries:
// the node's load spread evenly over them.
func share(d model.Demand, k int) model.Demand {
	if k == 1 {
		return d
	}
	f := 1 / float64(k)
	out := make(model.Demand, len(d))
	for kind, l := range d {
		one := model.Load{Monthly: l.Monthly * f, PeakPerSecond: l.PeakPerSecond * f}
		for _, s := range l.Sizes {
			one.Sizes = append(one.Sizes, model.Sized{KB: s.KB, Monthly: s.Monthly * f, PeakPerSecond: s.PeakPerSecond * f})
		}
		out[kind] = one
	}
	return out
}

// times turns the costs of one resource into the costs of k of them. A fee
// its pool pays once however many need it stays as it is.
func (rd *reader) times(costs []meter.Cost, k int) []meter.Cost {
	if k == 1 {
		return costs
	}
	f := float64(k)
	for i := range costs {
		c := &costs[i]
		if rd.books.Prices[c.PriceID].Combine == book.CombineMax {
			continue
		}
		c.Quantity *= f
		if c.MonthlyUSD != nil {
			m := *c.MonthlyUSD * f
			c.MonthlyUSD = &m
		}
		for j := range c.Bands {
			c.Bands[j].Quantity *= f
		}
	}
	return costs
}
