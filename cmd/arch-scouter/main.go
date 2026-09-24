// Command arch-scouter reads an architecture on cost, headroom, latency and availability.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/O6lvl4/arch-scouter/aws"
	"github.com/O6lvl4/arch-scouter/report"
	"github.com/O6lvl4/arch-scouter/scout"
)

const usage = `arch-scouter reads an architecture on cost, headroom, latency and availability.

Usage:
  arch-scouter scout <spec.yaml> [--json]     Read a declaration
  arch-scouter tf <dir> [flags]               Build a declaration from Terraform
  arch-scouter catalog                        List scouters and their fields as JSON
  arch-scouter sync [--check]                 Verify the price book against the AWS Price List
  arch-scouter explore <service> <region> [attr=regex...]
                                              Search the Price List to write sync filters

Run a subcommand with -h for its flags.
`

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		if !errors.Is(err, flag.ErrHelp) {
			fmt.Fprintln(os.Stderr, "arch-scouter:", err)
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
	fs := flag.NewFlagSet("scout", flag.ContinueOnError)
	asJSON := fs.Bool("json", false, "write JSON instead of Markdown")
	if err := fs.Parse(reorder(args)); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("scout takes one declaration file")
	}
	data, err := os.ReadFile(fs.Arg(0))
	if err != nil {
		return err
	}
	spec, err := scout.ParseSpec(data)
	if err != nil {
		return err
	}
	books, err := aws.Books()
	if err != nil {
		return err
	}
	res, err := scout.Run(spec, aws.Registry(), books)
	if err != nil {
		return err
	}
	if *asJSON {
		return report.JSON(out, res)
	}
	return report.Markdown(out, res)
}

type catalogEntry struct {
	scout.Meta
	Attributes  []scout.Field `json:"attributes"`
	Assumptions []scout.Field `json:"assumptions"`
}

func cmdCatalog(out io.Writer) error {
	reg := aws.Registry()
	var entries []catalogEntry
	for _, t := range reg.Types() {
		s := reg[t]
		entries = append(entries, catalogEntry{Meta: s.Meta(), Attributes: s.Attributes(), Assumptions: s.Assumptions()})
	}
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	return enc.Encode(entries)
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
