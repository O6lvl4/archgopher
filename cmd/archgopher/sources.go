package main

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/O6lvl4/archgopher/provider/aws/pricelist"
	"github.com/O6lvl4/archgopher/provider/azure/retailprices"
)

// quote is one price read from a public price list, per unit of the book.
type quote struct {
	usdPerUnit float64
	label      string
}

// priceSource reads one book row's price from wherever its sync spec says.
type priceSource interface {
	quote(region string) (quote, error)
	// global reports whether the price is the same whatever the book region,
	// so the "*" row can be verified too.
	global() bool
}

// sources hands out price sources, creating each client on first use.
type sources struct {
	aws   *pricelist.Client
	azure *retailprices.Client
}

// absent reports an error that means the price list offers nothing there.
func absent(err error) bool {
	return errors.Is(err, pricelist.ErrAbsent) || errors.Is(err, retailprices.ErrAbsent)
}

func (s *sources) of(raw json.RawMessage) (priceSource, error) {
	var probe struct {
		Source string `json:"source"`
	}
	if err := json.Unmarshal(raw, &probe); err != nil {
		return nil, err
	}
	switch probe.Source {
	case "", "aws":
		var spec pricelist.Spec
		if err := json.Unmarshal(raw, &spec); err != nil {
			return nil, err
		}
		if s.aws == nil {
			s.aws = pricelist.NewClient()
		}
		return awsSource{s.aws, spec}, nil
	case retailprices.Source:
		var spec retailprices.Spec
		if err := json.Unmarshal(raw, &spec); err != nil {
			return nil, err
		}
		if s.azure == nil {
			s.azure = retailprices.NewClient()
		}
		return azureSource{s.azure, spec}, nil
	}
	return nil, fmt.Errorf("unknown price source %q", probe.Source)
}

type awsSource struct {
	c    *pricelist.Client
	spec pricelist.Spec
}

func (a awsSource) quote(region string) (quote, error) {
	m, err := a.c.Resolve(a.spec, region)
	if err != nil {
		return quote{}, err
	}
	return quote{a.spec.PerUnit(m.USD), m.Product.Attributes["usagetype"] + ", " + m.Dimension.Unit}, nil
}

func (a awsSource) global() bool { return a.spec.OfferRegion != "" }

type azureSource struct {
	c    *retailprices.Client
	spec retailprices.Spec
}

func (a azureSource) quote(region string) (quote, error) {
	it, err := a.c.Resolve(a.spec, region)
	if err != nil {
		return quote{}, err
	}
	return quote{a.spec.PerUnit(it.RetailPrice), it.MeterName + ", " + it.UnitOfMeasure}, nil
}

func (a azureSource) global() bool { return a.spec.Region != "" }
