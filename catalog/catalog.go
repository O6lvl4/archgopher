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
