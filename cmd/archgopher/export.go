package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/O6lvl4/archgopher/api"
	"github.com/O6lvl4/archgopher/model"
)

// cmdExport draws a declaration:
//
//	export <spec.yaml> [--format svg] [-o file]
func cmdExport(args []string, out io.Writer) error {
	fs := flag.NewFlagSet("export", flag.ContinueOnError)
	format := fs.String("format", "svg", "what to write: svg")
	o := fs.String("o", "", "write to this file instead of stdout")
	if err := fs.Parse(reorder(args)); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("export takes one declaration file")
	}
	data, err := os.ReadFile(fs.Arg(0))
	if err != nil {
		return err
	}
	spec, err := model.ParseSpec(data)
	if err != nil {
		return err
	}
	var b []byte
	switch *format {
	case "svg":
		b, err = api.ExportSVG(spec)
	default:
		return fmt.Errorf("unknown format %q: svg", *format)
	}
	if err != nil {
		return err
	}
	if *o != "" {
		return os.WriteFile(*o, b, 0o644)
	}
	_, err = out.Write(b)
	return err
}
