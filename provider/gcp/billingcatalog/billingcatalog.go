// Package billingcatalog reads the Google Cloud Billing Catalog API. Unlike
// the AWS and Azure price lists it needs credentials: an OAuth access token
// (ARCHGOPHER_GCP_TOKEN, or gcloud's for ARCHGOPHER_GCP_ACCOUNT) or an API key
// (ARCHGOPHER_GCP_API_KEY). The prices themselves are public.
package billingcatalog

import (
	"errors"
	"slices"
	"strconv"
)

// Source names this price source in a book's sync spec.
const Source = "gcp"

// BaseURL is the Cloud Billing Catalog endpoint.
const BaseURL = "https://cloudbilling.googleapis.com/v1"

// ErrAbsent means the catalog lists nothing that matches in the region.
var ErrAbsent = errors.New("not in the Google Cloud Billing Catalog")

// ErrNoCredentials means no token or key is configured.
var ErrNoCredentials = errors.New("no Google Cloud credentials: set ARCHGOPHER_GCP_TOKEN, ARCHGOPHER_GCP_ACCOUNT (gcloud) or ARCHGOPHER_GCP_API_KEY")

// Money is the catalog's price: whole units and nanos.
type Money struct {
	Units string `json:"units"`
	Nanos int64  `json:"nanos"`
}

// USD is the price as a float.
func (m Money) USD() float64 {
	u, _ := strconv.ParseFloat(m.Units, 64)
	return u + float64(m.Nanos)/1e9
}

// Rate is one tier of a price.
type Rate struct {
	Start     float64 `json:"startUsageAmount"`
	UnitPrice Money   `json:"unitPrice"`
}

// Sku is one priced item.
type Sku struct {
	SkuID       string `json:"skuId"`
	Description string `json:"description"`
	Category    struct {
		ServiceDisplayName string `json:"serviceDisplayName"`
		ResourceFamily     string `json:"resourceFamily"`
		ResourceGroup      string `json:"resourceGroup"`
		UsageType          string `json:"usageType"`
	} `json:"category"`
	ServiceRegions []string `json:"serviceRegions"`
	// GeoTaxonomy names the regions of a sku listed under "global" whose
	// region is only in its description (Firestore: "Read Ops Tokyo").
	GeoTaxonomy struct {
		Type    string   `json:"type"`
		Regions []string `json:"regions"`
	} `json:"geoTaxonomy"`
	PricingInfo []struct {
		PricingExpression struct {
			UsageUnit            string  `json:"usageUnit"`
			UsageUnitDescription string  `json:"usageUnitDescription"`
			TieredRates          []Rate  `json:"tieredRates"`
			BaseUnitFactor       float64 `json:"baseUnitConversionFactor"`
		} `json:"pricingExpression"`
	} `json:"pricingInfo"`
}

// Field reads an attribute by name, for filters.
func (s Sku) Field(name string) string {
	switch name {
	case "description":
		return s.Description
	case "resourceFamily":
		return s.Category.ResourceFamily
	case "resourceGroup":
		return s.Category.ResourceGroup
	case "usageType":
		return s.Category.UsageType
	case "usageUnit":
		return s.Unit()
	case "skuId":
		return s.SkuID
	}
	return ""
}

// Unit is the usage unit the tiers are priced in ("GiBy.s", "h", "count").
func (s Sku) Unit() string {
	if len(s.PricingInfo) == 0 {
		return ""
	}
	return s.PricingInfo[0].PricingExpression.UsageUnit
}

// Rates are the tiers of the current price.
func (s Sku) Rates() []Rate {
	if len(s.PricingInfo) == 0 {
		return nil
	}
	return s.PricingInfo[0].PricingExpression.TieredRates
}

// In reports whether the sku is sold in a region ("global" counts everywhere
// only when the spec asks for it).
func (s Sku) In(region string) bool {
	return slices.Contains(s.ServiceRegions, region) ||
		(s.GeoTaxonomy.Type == "REGIONAL" && slices.Contains(s.GeoTaxonomy.Regions, region))
}

// Spec says how to find one price. Service is the catalog's service id
// ("152E-C115-5142" is Cloud Run); filters are regular expressions that must
// match the whole attribute; usageType defaults to OnDemand.
type Spec struct {
	Source  string            `json:"source"`
	Service string            `json:"service"`
	Filters map[string]string `json:"filters"`
	// Tier picks a tier: the default is the first with a price (a free grant
	// comes first at zero), or "first", "last" or a startUsageAmount.
	Tier string `json:"tier,omitempty"`
	// Region reads the sku sold in a fixed region ("global") whatever the
	// book region; its row is "*".
	Region string `json:"region,omitempty"`
	// ListPer is how many book units the catalog price covers. 0 is one.
	ListPer float64 `json:"listPer,omitempty"`
}

// PerUnit converts a catalog price to the price of one book unit.
func (s Spec) PerUnit(usd float64) float64 {
	if s.ListPer == 0 {
		return usd
	}
	return usd / s.ListPer
}

// For returns the region to look in for one book region.
func (s Spec) For(region string) string {
	if s.Region != "" {
		return s.Region
	}
	return region
}
