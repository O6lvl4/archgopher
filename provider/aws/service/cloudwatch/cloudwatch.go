// Package cloudwatch holds CloudWatch Logs prices. It has no scouter of its
// own: other services count their logs with facet.Logs against these rows.
package cloudwatch

import (
	"embed"

	"github.com/O6lvl4/arch-scouter/provider/aws/kit"
)

//go:embed books
var books embed.FS

// Service is CloudWatch's contribution to the AWS provider.
var Service = kit.Service{Name: "cloudwatch", Books: books}
