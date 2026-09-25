package meter

import (
	"errors"
	"math"
	"testing"

	"github.com/O6lvl4/archgopher/book"
)

func f(v float64) *float64 { return &v }

// tiered is $10 per million for the first 100 million, $5 for the next
// 400 million and $2 beyond.
func tiered(t *testing.T) book.Book {
	t.Helper()
	b, err := book.Book{"x.requests": {Unit: "request", Per: 1e6, Source: "test", Pool: book.PoolAccount, Tiered: true, Verified: true, Rows: map[string]map[string]*float64{
		"0": {"r1": f(10)}, "100000000": {"r1": f(5)}, "500000000": {"r1": f(2), "r2": nil},
	}}}.Flatten()
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func near(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

func TestBillCountsTiersOverTheWholeQuantity(t *testing.T) {
	b, err := Bill(tiered(t), "x.requests", "r1", 600e6, 0)
	if err != nil {
		t.Fatal(err)
	}
	// 100M × $10/M, 400M × $5/M, 100M × $2/M
	if want := 1000.0 + 2000 + 200; b.USD == nil || !near(*b.USD, want) {
		t.Fatalf("USD = %v, want %v", b.USD, want)
	}
	if len(b.Bands) != 3 || b.Bands[1].From != 100e6 || b.Bands[2].To != 0 {
		t.Fatalf("bands = %+v", b.Bands)
	}
}

func TestBillGivesIncludedUnitsFirst(t *testing.T) {
	b, err := Bill(tiered(t), "x.requests", "r1", 101e6, 1e6)
	if err != nil {
		t.Fatal(err)
	}
	// 1M included, 99M × $10/M, 1M × $5/M
	if b.USD == nil || !near(*b.USD, 995) || !b.Bands[0].Included || b.Bands[1].From != 1e6 {
		t.Fatalf("USD = %v, bands = %+v", b.USD, b.Bands)
	}
}

func TestBillIgnoresAFreeGrantTier(t *testing.T) {
	prices, err := book.Book{"x.ops": {Unit: "op", Source: "test", Pool: book.PoolAccount, Tiered: true, Verified: true, Rows: map[string]map[string]*float64{
		"0": {"r1": f(0), "r2": f(3)}, "100": {"r1": f(2), "r2": f(2)}, "1000": {"r1": f(1), "r2": f(1)},
	}}}.Flatten()
	if err != nil {
		t.Fatal(err)
	}
	// r1 lists the first 100 as a free grant: they are billed at $2.
	// r2 bills the first 100 at $3 of its own.
	for region, want := range map[string]float64{"r1": 2000 + 100, "r2": 300 + 1800 + 100} {
		b, err := Bill(prices, "x.ops", region, 1100, 0)
		if err != nil || !near(*b.USD, want) {
			t.Errorf("%s: %v (%v), want %v", region, *b.USD, err, want)
		}
	}
}

func TestBillATierNotOfferedOnlyWhenReached(t *testing.T) {
	prices := tiered(t)
	for _, row := range []string{"x.requests.0", "x.requests.100000000"} {
		e := prices[row]
		e.Values["r2"] = book.Value{Value: f(1), Verified: true}
		prices[row] = e
	}
	if _, err := Bill(prices, "x.requests", "r2", 1e6, 0); err != nil {
		t.Fatalf("below the missing tier: %v", err)
	}
	var no *NotOfferedError
	if _, err := Bill(prices, "x.requests", "r2", 600e6, 0); !errors.As(err, &no) {
		t.Fatalf("err = %v, want not offered", err)
	}
}

func TestRecorderBillsAPooledLineAsIfAlone(t *testing.T) {
	r := NewRecorder("r1", book.Books{Prices: tiered(t)})
	r.Cost("Requests", 101e6, "request", "x.requests")
	if err := r.Err(); err != nil {
		t.Fatal(err)
	}
	c := r.Costs()[0]
	if c.Pool != "x.requests" || c.MonthlyUSD == nil || !near(*c.MonthlyUSD, 1000+5) || !near(*c.UnitPrice, 1005/101e6) {
		t.Fatalf("cost = %+v", c)
	}
}

func TestRecorderSharesAPoolAmongItsOwnLines(t *testing.T) {
	r := NewRecorder("r1", book.Books{Prices: tiered(t)})
	r.Cost("Reads", 60e6, "request", "x.requests")
	r.Cost("Writes", 60e6, "request", "x.requests")
	// 120M: 100M × $10/M + 20M × $5/M = $1100, half each.
	costs := r.Costs()
	if !near(*costs[0].MonthlyUSD, 550) || !near(*costs[1].MonthlyUSD, 550) {
		t.Fatalf("costs = %v, %v", *costs[0].MonthlyUSD, *costs[1].MonthlyUSD)
	}
}

func TestRecorderChecksTheUnitOfABilledPrice(t *testing.T) {
	r := NewRecorder("r1", book.Books{Prices: tiered(t)})
	r.Cost("Requests", 1, "GB", "x.requests")
	if r.Err() == nil || len(r.Costs()) != 0 {
		t.Fatal("a GB reading of a per-request price was accepted")
	}
}
