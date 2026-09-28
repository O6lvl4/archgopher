package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/O6lvl4/archgopher/api"
	"github.com/O6lvl4/archgopher/model"
)

// cmdExport draws a declaration:
//
//	export <spec.yaml> [-o file] [--format svg|png|html] [--scale 2]
//
// The format follows the file's extension; --format overrides it, and svg
// is the default when writing to stdout.
func cmdExport(args []string, out io.Writer) error {
	fs := flag.NewFlagSet("export", flag.ContinueOnError)
	format := fs.String("format", "", "what to write: "+strings.Join(api.ExportFormats, ", ")+" (default: the -o file's extension, else svg)")
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
	f := *format
	if f == "" {
		f = strings.TrimPrefix(strings.ToLower(filepath.Ext(*o)), ".")
	}
	if f == "" {
		f = "svg"
	}
	b, err := api.Export(spec, f)
	if err != nil {
		return err
	}
	if *o != "" {
		return os.WriteFile(*o, b, 0o644)
	}
	_, err = out.Write(b)
	return err
}
