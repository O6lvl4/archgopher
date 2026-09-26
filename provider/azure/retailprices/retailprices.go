// Package retailprices reads the Azure Retail Prices API. It needs no
// credentials, so prices can be verified by anyone, including CI.
package retailprices

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Source names this price source in a book's sync spec.
const Source = "azure"

// BaseURL is the public Retail Prices endpoint.
const BaseURL = "https://prices.azure.com/api/retail/prices"

// ErrAbsent means the API lists nothing that matches in the region.
var ErrAbsent = errors.New("not in the Azure Retail Prices API")

// Item is one retail price.
type Item struct {
	ServiceName   string  `json:"serviceName"`
	ProductName   string  `json:"productName"`
	SkuName       string  `json:"skuName"`
	ArmSkuName    string  `json:"armSkuName"`
	MeterName     string  `json:"meterName"`
	MeterID       string  `json:"meterId"`
	Type          string  `json:"type"`
	UnitOfMeasure string  `json:"unitOfMeasure"`
	TierMinimum   float64 `json:"tierMinimumUnits"`
	RetailPrice   float64 `json:"retailPrice"`
	Region        string  `json:"armRegionName"`
}

// Field reads an item attribute by its API name, for filters.
func (i Item) Field(name string) string {
	switch name {
	case "productName":
		return i.ProductName
	case "skuName":
		return i.SkuName
	case "armSkuName":
		return i.ArmSkuName
	case "meterName":
		return i.MeterName
	case "type":
		return i.Type
	case "unitOfMeasure":
		return i.UnitOfMeasure
	}
	return ""
}

// Spec says how to find one price. Filters are regular expressions that must
// match the whole attribute; type defaults to Consumption.
type Spec struct {
	Source  string            `json:"source"`
	Service string            `json:"service"`
	Filters map[string]string `json:"filters"`
	// Tier picks among the tiers of one meter. The default is the first tier
	// with a price: a free grant comes first as a zero-priced tier (ignored,
	// as every free tier), and volume discounts come after the list price.
	// "first", "last" or where the tier starts pick explicitly. The start is
	// counted in the book's units: tierMinimumUnits × listPer, so a tier of
	// 1,000 "1M" units starts at 1000000000.
	Tier string `json:"tier,omitempty"`
	// Region reads a fixed region's list whatever the book region (Global
	// meters); its row is "*".
	Region string `json:"region,omitempty"`
	// ListPer is how many units the API price covers ("1M" is 1e6). 0 is one.
	ListPer float64 `json:"listPer,omitempty"`
}

// PerUnit converts an API price to the price of one unit.
func (s Spec) PerUnit(usd float64) float64 {
	if s.ListPer == 0 {
		return usd
	}
	return usd / s.ListPer
}

// For returns the region list to read for one book region.
func (s Spec) For(region string) string {
	if s.Region != "" {
		return s.Region
	}
	return region
}

// Client downloads and caches the price lists of one service in one region.
// It keeps the last list it read in memory, since a price table resolves
// hundreds of rows against one list.
type Client struct {
	HTTP     *http.Client
	CacheDir string
	MaxAge   time.Duration
	BaseURL  string

	mu   sync.Mutex
	last struct {
		key   string
		items []Item
	}
}

// NewClient caches under the user cache directory for a day.
func NewClient() *Client {
	dir, err := os.UserCacheDir()
	if err != nil {
		dir = os.TempDir()
	}
	return &Client{HTTP: &http.Client{Timeout: 2 * time.Minute}, CacheDir: filepath.Join(dir, "archgopher", "azure-prices"), MaxAge: 24 * time.Hour, BaseURL: BaseURL}
}

// Items returns every retail price of a service in a region.
func (c *Client) Items(service, region string) ([]Item, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	key := service + "/" + region
	if c.last.key == key {
		return c.last.items, nil
	}
	items, err := c.items(service, region)
	if err != nil {
		return nil, err
	}
	c.last.key, c.last.items = key, items
	return items, nil
}

func (c *Client) items(service, region string) ([]Item, error) {
	path := filepath.Join(c.CacheDir, safe(service), safe(region)+".json")
	if items, ok := c.cached(path); ok {
		return items, nil
	}
	items, err := c.download(service, region)
	if err != nil {
		return nil, err
	}
	if err := writeCache(path, items); err != nil {
		return nil, err
	}
	return items, nil
}

// cached returns the cached list when it is recent enough and readable.
func (c *Client) cached(path string) ([]Item, bool) {
	st, err := os.Stat(path)
	if err != nil || time.Since(st.ModTime()) > c.MaxAge {
		return nil, false
	}
	items, err := readCache(path)
	return items, err == nil
}

func (c *Client) download(service, region string) ([]Item, error) {
	filter := fmt.Sprintf("serviceName eq '%s' and armRegionName eq '%s'", service, region)
	next := c.BaseURL + "?api-version=2023-01-01-preview&$filter=" + url.QueryEscape(filter)
	var all []Item
	for next != "" {
		page, link, err := c.page(next)
		if err != nil {
			return nil, err
		}
		all = append(all, page...)
		next = link
	}
	return all, nil
}

func (c *Client) page(u string) ([]Item, string, error) {
	resp, err := c.HTTP.Get(u)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, "", fmt.Errorf("GET %s: %s: %s", u, resp.Status, body)
	}
	var p struct {
		Items        []Item `json:"Items"`
		NextPageLink string `json:"NextPageLink"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&p); err != nil {
		return nil, "", err
	}
	return p.Items, p.NextPageLink, nil
}

func safe(s string) string {
	return strings.Map(func(r rune) rune {
		if r == '/' || r == '\\' || r == ' ' {
			return '_'
		}
		return r
	}, s)
}

func readCache(path string) ([]Item, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var items []Item
	return items, json.Unmarshal(data, &items)
}

func writeCache(path string, items []Item) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.Marshal(items)
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
