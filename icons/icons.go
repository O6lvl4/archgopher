// Package icons holds the providers' architecture icons, one SVG per
// service, named "<provider>/<name>" by the catalog. They are embedded so the
// CLI can draw a declaration; the web UI reads the same files.
package icons

import "embed"

// FS holds every icon as "<provider>/<name>.svg".
//
//go:embed aws azure gcp cloudflare conoha general
var FS embed.FS

// Read returns the SVG of "<provider>/<name>".
func Read(name string) ([]byte, error) { return FS.ReadFile(name + ".svg") }
