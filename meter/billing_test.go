package meter

import (
	"errors"
	"math"
	"testing"

	"github.com/O6lvl4/archgopher/book"
)

func f(v float64) *float64 { return &v }

// tiered is $10 per million for the first 100 million, $5 for the next
// 400 million and $2 beyond, with the first million free.
func tiered(t *testing.T) book.Book {
	t.Helper()
	b, err := book.Book{"x.requests": {Unit: "request", Per: 1e6, Source: "test", Pool: book.PoolAccount, Free: 1e6, Tiered: true, Verified: true, Rows: map[string]map[string]*float64{
		"0": {"r1": f(10)}, "100000000": {"r1": f(5)}, "500000000": {"r1": f(2), "r2": nil},
	}}}.Flatten()
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func near(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

func TestBillCountsTiersOverTheWholeQuantity(t *testing.T) {
	b, err := Bill(tiered(t), "x.requests", "r1", 600e6, 1e6, true)
	if err != nil {
		t.Fatal(err)
	}
	// 1M free, 99M × $10/M, 400M × $5/M, 100M × $2/M
	if want := 990.0 + 2000 + 200; b.USD == nil || !near(*b.USD, want) {
		t.Fatalf("USD = %v, want %v", b.USD, want)
	}
	if len(b.Bands) != 4 || !b.Bands[0].Free || b.Bands[0].Quantity != 1e6 || b.Bands[1].From != 1e6 || b.Bands[3].To != 0 {
		t.Fatalf("bands = %+v", b.Bands)
	}
}

func TestBillWithoutFreeUnits(t *testing.T) {
	b, err := Bill(tiered(t), "x.requests", "r1", 2e6, 0, false)
	if err != nil {
		t.Fatal(err)
	}
	if b.USD == nil || !near(*b.USD, 20) || len(b.Bands) != 1 {
		t.Fatalf("USD = %v, bands = %+v", b.USD, b.Bands)
	}
}

func TestBillInsideTheFreeUnits(t *testing.T) {
	b, err := Bill(tiered(t), "x.requests", "r1", 5e5, 1e6, true)
	if err != nil {
		t.Fatal(err)
	}
	if b.USD == nil || *b.USD != 0 || len(b.Bands) != 1 || !b.Bands[0].Free {
		t.Fatalf("USD = %v, bands = %+v", b.USD, b.Bands)
	}
}

func TestBillATierNotOfferedOnlyWhenReached(t *testing.T) {
	prices := tiered(t)
	for _, row := range []string{"x.requests.0", "x.requests.100000000"} {
		e := prices[row]
		e.Values["r2"] = book.Value{Value: f(1), Verified: true}
		prices[row] = e
	}
	if _, err := Bill(prices, "x.requests", "r2", 1e6, 1e6, true); err != nil {
		t.Fatalf("below the missing tier: %v", err)
	}
	var no *NotOfferedError
	if _, err := Bill(prices, "x.requests", "r2", 600e6, 1e6, true); !errors.As(err, &no) {
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
	if c.Pool != "x.requests" || c.MonthlyUSD == nil || !near(*c.MonthlyUSD, 990+5) {
		t.Fatalf("cost = %+v", c)
	}
	if !near(*c.UnitPrice, 995/101e6) {
		t.Fatalf("unit price = %v", *c.UnitPrice)
	}
}

func TestRecorderChecksTheUnitOfABilledPrice(t *testing.T) {
	r := NewRecorder("r1", book.Books{Prices: tiered(t)})
	r.Cost("Requests", 1, "GB", "x.requests")
	if r.Err() == nil || len(r.Costs()) != 0 {
		t.Fatal("a GB reading of a per-request price was accepted")
	}
}

func TestAFreeGroupSplitsItsUnitsByQuantity(t *testing.T) {
	grouped := func(price float64) book.Entry {
		return book.Entry{Unit: "GB-second", Source: "test", Pool: book.PoolRegion, Free: 400000, FreeGroup: "x.gb_second", Values: map[string]book.Value{"r1": {Value: f(price), Verified: true}}}
	}
	prices, err := book.Book{"x.x86": grouped(2), "x.arm": grouped(1)}.Flatten()
	if err != nil {
		t.Fatal(err)
	}
	r := NewRecorder("r1", book.Books{Prices: prices})
	r.Cost("x86", 300000, "GB-second", "x.x86")
	r.Cost("arm", 500000, "GB-second", "x.arm")
	r.Cost("more arm", 200000, "GB-second", "x.arm")
	// 1,000,000 GB-seconds, 400,000 free: x86 gets 3/10 of them, arm 7/10.
	costs := r.Costs()
	if got := *costs[0].MonthlyUSD; !near(got, (300000-120000)*2) {
		t.Errorf("x86 = %v", got)
	}
	if got := *costs[1].MonthlyUSD + *costs[2].MonthlyUSD; !near(got, 700000-280000) {
		t.Errorf("arm = %v", got)
	}
	if !near(*costs[1].MonthlyUSD / *costs[2].MonthlyUSD, 2.5) {
		t.Errorf("arm lines are not shared by quantity: %v, %v", *costs[1].MonthlyUSD, *costs[2].MonthlyUSD)
	}
}

func TestNoFreeBillsEveryUnit(t *testing.T) {
	r := NewRecorder("r1", book.Books{Prices: tiered(t)})
	r.NoFree = true
	r.Cost("Requests", 1e6, "request", "x.requests")
	if got := *r.Costs()[0].MonthlyUSD; !near(got, 10) {
		t.Fatalf("got %v, want 10", got)
	}
}

func TestNoFreeBillsAFreeGrantTier(t *testing.T) {
	prices, err := book.Book{"x.ops": {Unit: "op", Source: "test", Pool: book.PoolAccount, Tiered: true, Verified: true, Rows: map[string]map[string]*float64{
		"0": {"r1": f(0)}, "100": {"r1": f(2)}, "1000": {"r1": f(1)},
	}}}.Flatten()
	if err != nil {
		t.Fatal(err)
	}
	with, _ := Bill(prices, "x.ops", "r1", 200, 0, true)
	without, _ := Bill(prices, "x.ops", "r1", 200, 0, false)
	if *with.USD != 200 || *without.USD != 400 {
		t.Fatalf("with the grant %v, without %v; want 200 and 400", *with.USD, *without.USD)
	}
}
