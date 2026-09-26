// Package pricelist reads the public AWS Price List bulk files. They need no
// credentials, so prices can be verified by anyone, including CI.
package pricelist

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// ErrAbsent means the Price List offers nothing that matches in the region:
// no file for the service there, or no product that fits the filters.
var ErrAbsent = errors.New("not in the Price List")

// BaseURL is the public Price List endpoint.
const BaseURL = "https://pricing.us-east-1.amazonaws.com"

// Offer is the part of an offer file that pricing needs.
type Offer struct {
	Products map[string]Product `json:"products"`
	Terms    struct {
		OnDemand map[string]map[string]Term `json:"OnDemand"`
	} `json:"terms"`
}

// Product is one SKU.
type Product struct {
	SKU           string            `json:"sku"`
	ProductFamily string            `json:"productFamily"`
	Attributes    map[string]string `json:"attributes"`
}

// Term holds the price dimensions of an on-demand offer term.
type Term struct {
	PriceDimensions map[string]Dimension `json:"priceDimensions"`
}

// Dimension is one price, possibly one tier of several.
type Dimension struct {
	Unit         string            `json:"unit"`
	Description  string            `json:"description"`
	BeginRange   string            `json:"beginRange"`
	EndRange     string            `json:"endRange"`
	PricePerUnit map[string]string `json:"pricePerUnit"`
}

// Spec says how to find one price. Filters are regular expressions that must
// match the whole attribute value; productFamily is matched like an attribute.
type Spec struct {
	Service string            `json:"service"`
	Filters map[string]string `json:"filters"`
	// Tier picks a dimension when there are several: "first" (default), "last",
	// or the beginRange of the tier as a number.
	Tier string `json:"tier,omitempty"`
	// OfferRegion reads another section of the service's files (CloudFront's
	// edge prices live in "aws-other" whatever the resource region). A price
	// read there is the same in every region, so its row is "*".
	OfferRegion string `json:"offerRegion,omitempty"`
	// ListPer is how many units the Price List price covers when its unit is a
	// bundle ("1M Input Tokens" is 1e6). 0 means one unit.
	ListPer float64 `json:"listPer,omitempty"`
}

// PerUnit converts a Price List price to the price of one unit.
func (s Spec) PerUnit(usd float64) float64 {
	if s.ListPer == 0 {
		return usd
	}
	return usd / s.ListPer
}

// For returns the offer region to read for one book region.
func (s Spec) For(region string) string {
	if s.OfferRegion != "" {
		return s.OfferRegion
	}
	return region
}

// Match is one priced dimension of a matching product.
type Match struct {
	Product   Product
	Dimension Dimension
	USD       float64
}

// Client downloads and caches offer files. It keeps the last offer it read
// in memory: a price table resolves hundreds of rows against one offer, and
// some offers (EC2) are hundreds of megabytes.
type Client struct {
	HTTP     *http.Client
	CacheDir string
	MaxAge   time.Duration

	mu   sync.Mutex
	last struct {
		key   string
		offer *Offer
	}
}

// NewClient caches under the user cache directory for a day.
func NewClient() *Client {
	dir, err := os.UserCacheDir()
	if err != nil {
		dir = os.TempDir()
	}
	return &Client{HTTP: &http.Client{Timeout: 5 * time.Minute}, CacheDir: filepath.Join(dir, "archgopher", "pricelist"), MaxAge: 24 * time.Hour}
}

// Offer returns the offer file of a service in a region.
func (c *Client) Offer(service, region string) (*Offer, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	key := service + "/" + region
	if c.last.key == key {
		return c.last.offer, nil
	}
	// Let the previous offer go before reading the next; a failed read then
	// leaves nothing cached.
	c.last.key, c.last.offer = "", nil
	o, err := c.read(service, region)
	if err != nil {
		return nil, err
	}
	c.last.key, c.last.offer = key, o
	return o, nil
}

func (c *Client) read(service, region string) (*Offer, error) {
	path := filepath.Join(c.CacheDir, service, region+".json")
	if err := c.refresh(service, region, path); err != nil {
		return nil, err
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var o Offer
	if err := json.NewDecoder(f).Decode(&o); err != nil {
		return nil, fmt.Errorf("%s %s: %w", service, region, err)
	}
	return &o, nil
}

// refresh downloads an offer file unless the cached copy is recent enough.
func (c *Client) refresh(service, region, path string) error {
	if st, err := os.Stat(path); err == nil && time.Since(st.ModTime()) <= c.MaxAge {
		return nil
	}
	return c.download(service, region, path)
}

func (c *Client) download(service, region, path string) error {
	url := fmt.Sprintf("%s/offers/v1.0/aws/%s/current/%s/index.json", BaseURL, service, region)
	resp, err := c.HTTP.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusForbidden {
		return fmt.Errorf("%w: GET %s: %s", ErrAbsent, url, resp.Status)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GET %s: %s", url, resp.Status)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	// A temporary file of its own, so processes downloading the same offer
	// at once never write into one file.
	f, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if _, err := io.Copy(f, resp.Body); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
