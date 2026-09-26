// Package servicequotas reads AWS Service Quotas: the default value of a
// quota in a region, which verifies the published quotas of the books, and
// the value applied to an account, which is what that account runs against.
// The API needs credentials; they come from the AWS CLI.
package servicequotas

import (
	"errors"
	"fmt"
)

// Source names this source in a sync spec.
const Source = "servicequotas"

// ErrAbsent means the quota is not listed in the region.
var ErrAbsent = errors.New("not in Service Quotas")

// ErrNoCredentials means the environment names no AWS profile or keys.
var ErrNoCredentials = errors.New("no AWS credentials: set ARCHGOPHER_AWS_PROFILE (or AWS_PROFILE) to a profile the AWS CLI can use")

// Spec names one quota.
type Spec struct {
	Source string `json:"source"`
	// Service is the Service Quotas service code ("lambda").
	Service string `json:"service"`
	// Quota is the quota code ("L-B99A9384").
	Quota string `json:"quota"`
	// ListPer is how many book units one listed unit covers, when they
	// differ (a quota per second read into a book per minute is 1/60). 0 is 1.
	ListPer float64 `json:"listPer,omitempty"`
	// Region reads a fixed region whatever the book region (a global quota
	// listed in us-east-1); its row is "*".
	Region string `json:"region,omitempty"`
}

// PerUnit converts a listed value into the book's unit.
func (s Spec) PerUnit(v float64) float64 {
	if s.ListPer == 0 {
		return v
	}
	return v / s.ListPer
}

// Quota is one quota as Service Quotas lists it.
type Quota struct {
	ServiceCode string  `json:"ServiceCode"`
	QuotaCode   string  `json:"QuotaCode"`
	QuotaName   string  `json:"QuotaName"`
	Value       float64 `json:"Value"`
	Unit        string  `json:"Unit"`
	Adjustable  bool    `json:"Adjustable"`
	GlobalQuota bool    `json:"GlobalQuota"`
}

// Label names the quota in a report.
func (q Quota) Label() string {
	if q.Unit == "" || q.Unit == "None" {
		return q.QuotaName
	}
	return fmt.Sprintf("%s, %s", q.QuotaName, q.Unit)
}
