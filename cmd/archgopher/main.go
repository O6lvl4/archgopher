// Command archgopher reads an architecture on cost, headroom, latency and availability.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/O6lvl4/archgopher/api"
	"github.com/O6lvl4/archgopher/gaps"
	"github.com/O6lvl4/archgopher/model"
	"github.com/O6lvl4/archgopher/report"
)

const usage = `archgopher reads an architecture on cost, headroom, latency and availability.

Usage:
  archgopher scout <spec.yaml> [--json]     Read a declaration
  archgopher gaps <spec.yaml> [--json]      List what the declaration does not know yet
  archgopher tf <dir> [flags]               Build a declaration from Terraform
  archgopher diff <before> <after> [flags]  What changed: declarations or Terraform directories
  archgopher catalog                        List scouters and their fields as JSON
  archgopher sync [--check]                 Verify the price book against the AWS Price List
  archgopher explore <service> <region> [attr=regex...]
                                              Search the Price List to write sync filters

Run a subcommand with -h for its flags.
`

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		if !errors.Is(err, flag.ErrHelp) {
			fmt.Fprintln(os.Stderr, "archgopher:", err)
		}
		os.Exit(1)
	}
}

func run(args []string, out io.Writer) error {
	if len(args) == 0 {
		fmt.Fprint(os.Stderr, usage)
		return flag.ErrHelp
	}
	switch args[0] {
	case "scout":
		return cmdScout(args[1:], out)
	case "gaps":
		return cmdGaps(args[1:], out)
	case "diff":
		return cmdDiff(args[1:], out)
	case "tf":
		return cmdTerraform(args[1:], out)
	case "catalog":
		return cmdCatalog(out)
	case "sync":
		return cmdSync(args[1:], out)
	case "explore":
		return cmdExplore(args[1:], out)
	case "-h", "--help", "help":
		fmt.Fprint(out, usage)
		return nil
	}
	return fmt.Errorf("unknown command %q\n\n%s", args[0], usage)
}

func cmdScout(args []string, out io.Writer) error {
	spec, asJSON, err := declaration("scout", args)
	if err != nil {
		return err
	}
	res, err := api.Scout(spec)
	if err != nil {
		return err
	}
	if asJSON {
		return report.JSON(out, res)
	}
	return report.Markdown(out, res)
}

func cmdGaps(args []string, out io.Writer) error {
	spec, asJSON, err := declaration("gaps", args)
	if err != nil {
		return err
	}
	gs, err := api.Gaps(spec)
	if err != nil {
		return err
	}
	if asJSON {
		if gs == nil {
			gs = []gaps.Gap{}
		}
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		return enc.Encode(gs)
	}
	_, err = io.WriteString(out, gaps.Text(gs))
	return err
}

// declaration parses the flags of a command that reads one declaration file.
func declaration(name string, args []string) (model.Spec, bool, error) {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	asJSON := fs.Bool("json", false, "write JSON")
	if err := fs.Parse(reorder(args)); err != nil {
		return model.Spec{}, false, err
	}
	if fs.NArg() != 1 {
		return model.Spec{}, false, fmt.Errorf("%s takes one declaration file", name)
	}
	data, err := os.ReadFile(fs.Arg(0))
	if err != nil {
		return model.Spec{}, false, err
	}
	spec, err := model.ParseSpec(data)
	return spec, *asJSON, err
}

func cmdCatalog(out io.Writer) error {
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	return enc.Encode(api.Catalog())
}

// reorder moves flags before positional arguments so "scout file --json" works.
// Every flag takes a value except the boolean ones listed here.
func reorder(args []string) []string {
	boolean := map[string]bool{"json": true, "check": true, "h": true, "help": true}
	var flags, rest []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		if len(a) < 2 || a[0] != '-' {
			rest = append(rest, a)
			continue
		}
		flags = append(flags, a)
		name := strings.TrimLeft(a, "-")
		if !strings.Contains(name, "=") && !boolean[name] && i+1 < len(args) {
			flags = append(flags, args[i+1])
			i++
		}
	}
	return append(flags, rest...)
}
