// Package catalog holds cloud resources as data, one directory per resource
// type: resource.yaml, books/ and cases.yaml. Adding a resource is adding a
// directory; no Go code changes. Directories without resource.yaml share
// books that several resources read.
package catalog

import "embed"

// AWS is the AWS catalog.
//
//go:embed aws
var AWS embed.FS

// AWSRoot is the directory of the AWS catalog inside AWS.
const AWSRoot = "aws"

// Azure is the Azure catalog.
//
//go:embed azure
var Azure embed.FS

// AzureRoot is the directory of the Azure catalog inside Azure.
const AzureRoot = "azure"

// GCP is the Google Cloud catalog.
//
//go:embed gcp
var GCP embed.FS

// GCPRoot is the directory of the Google Cloud catalog inside GCP.
const GCPRoot = "gcp"

// Cloudflare is the Cloudflare catalog.
//
//go:embed cloudflare
var Cloudflare embed.FS

// CloudflareRoot is the directory of the Cloudflare catalog inside Cloudflare.
const CloudflareRoot = "cloudflare"

// ConoHa is the ConoHa VPS catalog.
//
//go:embed conoha
var ConoHa embed.FS

// ConoHaRoot is the directory of the ConoHa catalog inside ConoHa.
const ConoHaRoot = "conoha"
