package billingcatalog

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func sku(id, desc string, regions []string, unit string, rates ...Rate) Sku {
	s := Sku{SkuID: id, Description: desc, ServiceRegions: regions}
	s.Category.UsageType = "OnDemand"
	s.PricingInfo = make([]struct {
		PricingExpression struct {
			UsageUnit            string  `json:"usageUnit"`
			UsageUnitDescription string  `json:"usageUnitDescription"`
			TieredRates          []Rate  `json:"tieredRates"`
			BaseUnitFactor       float64 `json:"baseUnitConversionFactor"`
		} `json:"pricingExpression"`
	}, 1)
	s.PricingInfo[0].PricingExpression.UsageUnit = unit
	s.PricingInfo[0].PricingExpression.TieredRates = rates
	return s
}

func server(t *testing.T) *Client {
	t.Helper()
	pages := map[string][]Sku{
		"": {sku("A", "CPU Allocation Time", []string{"asia-northeast1"}, "s",
			Rate{Start: 0, UnitPrice: Money{Units: "0"}}, Rate{Start: 180000, UnitPrice: Money{Nanos: 24000}})},
		"p2": {
			sku("B", "Memory Allocation Time", []string{"asia-northeast1", "us-east4"}, "GiBy.s", Rate{UnitPrice: Money{Nanos: 2500}}),
			sku("C", "Requests", []string{"global"}, "count", Rate{UnitPrice: Money{Units: "0", Nanos: 400}}),
		},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer t" {
			http.Error(w, "unauthenticated", http.StatusUnauthorized)
			return
		}
		if !strings.HasSuffix(r.URL.Path, "/services/RUN/skus") {
			http.NotFound(w, r)
			return
		}
		tok := r.URL.Query().Get("pageToken")
		next := ""
		if tok == "" {
			next = "p2"
		}
		reply(t, w, map[string]any{"skus": pages[tok], "nextPageToken": next})
	}))
	t.Cleanup(srv.Close)
	return &Client{HTTP: srv.Client(), CacheDir: t.TempDir(), BaseURL: srv.URL, Auth: func(r *http.Request) error {
		r.Header.Set("Authorization", "Bearer t")
		return nil
	}}
}

func TestResolveReadsPagesRegionsAndThePaidTier(t *testing.T) {
	c := server(t)
	p, err := c.Resolve(Spec{Service: "RUN", Filters: map[string]string{"description": "CPU Allocation Time"}}, "asia-northeast1")
	if err != nil || p.Rate.UnitPrice.USD() != 2.4e-5 {
		t.Fatalf("want the paid tier 2.4e-5, got %+v %v", p.Rate, err)
	}
	p, err = c.Resolve(Spec{Service: "RUN", Filters: map[string]string{"description": "Memory.*"}}, "us-east4")
	if err != nil || p.Sku.SkuID != "B" {
		t.Fatalf("want the second page's sku in us-east4, got %+v %v", p, err)
	}
	p, err = c.Resolve(Spec{Service: "RUN", Region: "global", Filters: map[string]string{"description": "Requests"}, ListPer: 1}, "asia-northeast1")
	if err != nil || p.Sku.SkuID != "C" {
		t.Fatalf("a global sku read for any region, got %+v %v", p, err)
	}
}

func TestResolveExplainsAbsenceAmbiguityAndMissingCredentials(t *testing.T) {
	c := server(t)
	if _, err := c.Resolve(Spec{Service: "RUN", Filters: map[string]string{"description": "CPU Allocation Time"}}, "europe-west3"); !errors.Is(err, ErrAbsent) {
		t.Fatalf("a region without the sku is absent, got %v", err)
	}
	if _, err := c.Resolve(Spec{Service: "RUN", Filters: map[string]string{"description": ".*Allocation Time"}}, "asia-northeast1"); err == nil || !strings.Contains(err.Error(), "2 skus match") {
		t.Fatalf("want an ambiguity error, got %v", err)
	}
	t.Setenv("ARCHGOPHER_GCP_API_KEY", "")
	t.Setenv("ARCHGOPHER_GCP_TOKEN", "")
	t.Setenv("ARCHGOPHER_GCP_ACCOUNT", "")
	if _, err := EnvAuth(); !errors.Is(err, ErrNoCredentials) {
		t.Fatalf("want ErrNoCredentials, got %v", err)
	}
}

func TestMoney(t *testing.T) {
	if got := (Money{Units: "1", Nanos: 500000000}).USD(); got != 1.5 {
		t.Fatalf("got %v", got)
	}
}

// reply writes a JSON response from a test server.
func reply(t *testing.T, w http.ResponseWriter, v any) {
	if err := json.NewEncoder(w).Encode(v); err != nil {
		t.Errorf("reply: %v", err)
	}
}
