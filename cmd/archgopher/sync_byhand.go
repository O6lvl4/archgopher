package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"text/tabwriter"

	"github.com/O6lvl4/archgopher/book"
)

// byHand lists the rows a new region still needs that the Price List cannot
// give: prices without a sync spec, and quotas and SLAs that vary by region.
func byHand(out io.Writer, priceFiles []string, regions []string) error {
	if len(regions) == 0 {
		return nil
	}
	var missing []string
	for _, f := range priceFiles {
		for _, name := range []string{"prices", "quotas", "slas"} {
			m, err := missingIn(filepath.Join(filepath.Dir(f), name+".json"), name == "prices", regions)
			if err != nil {
				return err
			}
			missing = append(missing, m...)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	sort.Strings(missing)
	fmt.Fprintf(out, "\n%d rows need a value by hand:\n", len(missing))
	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	for _, m := range missing {
		fmt.Fprintln(w, m)
	}
	return w.Flush()
}

// missingIn lists, as "path id region", the rows of one book that have no
// value in some of the regions and that only a person can fill. A book that
// does not exist has none.
func missingIn(path string, prices bool, regions []string) ([]string, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var b book.Book
	if err := json.Unmarshal(data, &b); err != nil {
		return nil, err
	}
	var missing []string
	for id, e := range b {
		if !filledByHand(e, prices) {
			continue
		}
		for _, r := range regions {
			if _, has := e.Values[r]; !has {
				missing = append(missing, fmt.Sprintf("%s\t%s\t%s", path, id, r))
			}
		}
	}
	return missing, nil
}

// filledByHand reports whether a row varies by region and has no sync spec
// to fill a new region from.
func filledByHand(e book.Entry, prices bool) bool {
	if _, any := e.Values[book.AnyRegion]; any {
		return false
	}
	return !prices || len(e.Sync) == 0
}
