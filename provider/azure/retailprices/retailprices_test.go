package retailprices

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func server(t *testing.T) *Client {
	t.Helper()
	pages := [][]Item{
		{
			{ProductName: "Functions", SkuName: "Standard", MeterName: "Standard Total Executions", MeterID: "m1", Type: "Consumption", TierMinimum: 0, RetailPrice: 0},
			{ProductName: "Functions", SkuName: "Standard", MeterName: "Standard Total Executions", MeterID: "m1", Type: "Consumption", TierMinimum: 100000, RetailPrice: 2e-6},
		},
		{
			{ProductName: "Functions", SkuName: "Standard", MeterName: "Standard Execution Time", MeterID: "m2", Type: "Consumption", TierMinimum: 400000, RetailPrice: 1.6e-5},
			{ProductName: "Blob", SkuName: "Hot LRS", MeterName: "Hot LRS Data Stored", MeterID: "m4", Type: "Consumption", TierMinimum: 0, RetailPrice: 0.02},
			{ProductName: "Blob", SkuName: "Hot LRS", MeterName: "Hot LRS Data Stored", MeterID: "m4", Type: "Consumption", TierMinimum: 51200, RetailPrice: 0.0192},
			{ProductName: "Functions", SkuName: "Standard", MeterName: "Standard Execution Time", MeterID: "m3", Type: "DevTestConsumption", RetailPrice: 1},
		},
	}
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Query().Get("$filter"), "armRegionName eq 'japaneast'") {
			reply(t, w, map[string]any{"Items": []Item{}})
			return
		}
		i := 0
		if r.URL.Query().Get("page") == "2" {
			i = 1
		}
		next := ""
		if i == 0 {
			next = srv.URL + "?page=2&$filter=" + url.QueryEscape(r.URL.Query().Get("$filter"))
		}
		reply(t, w, map[string]any{"Items": pages[i], "NextPageLink": next})
	}))
	t.Cleanup(srv.Close)
	return &Client{HTTP: srv.Client(), CacheDir: t.TempDir(), BaseURL: srv.URL}
}

func TestResolveFollowsPagesAndSkipsTheFreeTier(t *testing.T) {
	c := server(t)
	it, err := c.Resolve(Spec{Service: "Functions", Filters: map[string]string{"meterName": "Standard Total Executions"}, ListPer: 10}, "japaneast")
	if err != nil || it.RetailPrice != 2e-6 {
		t.Fatalf("want the paid tier, got %+v %v", it, err)
	}
	it, err = c.Resolve(Spec{Service: "Functions", Filters: map[string]string{"meterName": "Standard Execution Time"}}, "japaneast")
	if err != nil || it.MeterID != "m2" {
		t.Fatalf("the second page and Consumption only, got %+v %v", it, err)
	}
}

func TestTheListPriceIsNotAVolumeDiscount(t *testing.T) {
	c := server(t)
	it, err := c.Resolve(Spec{Service: "Storage", Filters: map[string]string{"meterName": "Hot LRS Data Stored"}}, "japaneast")
	if err != nil || it.RetailPrice != 0.02 {
		t.Fatalf("want the first priced tier, not the volume discount: %+v %v", it, err)
	}
}

func TestResolveExplainsAbsenceAndAmbiguity(t *testing.T) {
	c := server(t)
	if _, err := c.Resolve(Spec{Service: "Functions", Filters: map[string]string{"meterName": "Nothing"}}, "japaneast"); !errors.Is(err, ErrAbsent) {
		t.Fatalf("want ErrAbsent, got %v", err)
	}
	if _, err := c.Resolve(Spec{Service: "Functions", Filters: map[string]string{"meterName": "Standard.*"}}, "japaneast"); err == nil || !strings.Contains(err.Error(), "2 meters match") {
		t.Fatalf("want an ambiguity error, got %v", err)
	}
	if _, err := c.Resolve(Spec{Service: "Functions", Filters: map[string]string{"meterName": "Standard Total Executions"}}, "westus2"); !errors.Is(err, ErrAbsent) {
		t.Fatalf("a region with no prices is absent, got %v", err)
	}
}

func TestSpecUnits(t *testing.T) {
	if got := (Spec{ListPer: 10}).PerUnit(2e-6); got != 2e-7 {
		t.Fatalf("got %v", got)
	}
	if got := (Spec{Region: "Global"}).For("japaneast"); got != "Global" {
		t.Fatalf("got %v", got)
	}
}

// reply writes a JSON response from a test server.
func reply(t *testing.T, w http.ResponseWriter, v any) {
	if err := json.NewEncoder(w).Encode(v); err != nil {
		t.Errorf("reply: %v", err)
	}
}
