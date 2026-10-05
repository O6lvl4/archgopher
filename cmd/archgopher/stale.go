package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/O6lvl4/archgopher/book"
)

// cmdStale lists the values of the books that nobody has checked for a while,
// grouped by the page they were read from: one visit to a pricing or quota
// page re-checks every value it holds. Values sync re-reads get a fresh date
// every week, so one of them showing up here means sync has stopped reaching it.
func cmdStale(args []string, out io.Writer) error {
	fs := flag.NewFlagSet("stale", flag.ContinueOnError)
	pattern := fs.String("books", "catalog/*/*/books/*.json", "books to read (glob)")
	days := fs.Int("days", 90, "a value checked longer ago than this is stale")
	today := fs.String("today", time.Now().UTC().Format(time.DateOnly), "the date to measure from")
	if err := fs.Parse(reorder(args)); err != nil {
		return err
	}
	now, err := time.Parse(time.DateOnly, *today)
	if err != nil {
		return fmt.Errorf("--today: %w", err)
	}
	files, err := filepath.Glob(*pattern)
	if err != nil {
		return err
	}
	if len(files) == 0 {
		return fmt.Errorf("no books match %s (run from the repository root)", *pattern)
	}
	cutoff := now.AddDate(0, 0, -*days).Format(time.DateOnly)
	pages := map[string]*stalePage{}
	for _, f := range files {
		if err := staleIn(f, cutoff, pages); err != nil {
			return err
		}
	}
	return writeStale(out, pages, *days)
}

// stalePage is what one source page holds that is due for a check.
type stalePage struct {
	source string
	oldest string // "" when some value was never checked
	ids    []string
}

// staleIn adds the values of one book checked before cutoff, or never.
func staleIn(path, cutoff string, pages map[string]*stalePage) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var b book.Book
	if err := json.Unmarshal(data, &b); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	for id, e := range b {
		checked, ok := lastChecked(e)
		if ok && checked >= cutoff {
			continue
		}
		p := pages[e.Source]
		if p == nil {
			p = &stalePage{source: e.Source, oldest: checked}
			pages[e.Source] = p
		}
		if checked < p.oldest {
			p.oldest = checked
		}
		p.ids = append(p.ids, id)
	}
	return nil
}

// lastChecked is the oldest check date of an entry's values; ok is false when
// some value was never checked or not verified.
func lastChecked(e book.Entry) (string, bool) {
	if len(e.Rows) > 0 {
		return e.CheckedAt, e.Verified && e.CheckedAt != ""
	}
	oldest, ok := "", len(e.Values) > 0
	for _, v := range e.Values {
		if !v.Verified || v.CheckedAt == "" {
			return "", false
		}
		if oldest == "" || v.CheckedAt < oldest {
			oldest = v.CheckedAt
		}
	}
	return oldest, ok
}

// writeStale writes the pages as Markdown, never-checked first, then oldest first.
func writeStale(out io.Writer, pages map[string]*stalePage, days int) error {
	if len(pages) == 0 {
		_, err := fmt.Fprintf(out, "Every value was checked in the last %d days.\n", days)
		return err
	}
	list := make([]*stalePage, 0, len(pages))
	total := 0
	for _, p := range pages {
		sort.Strings(p.ids)
		list = append(list, p)
		total += len(p.ids)
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].oldest != list[j].oldest {
			return list[i].oldest < list[j].oldest
		}
		return list[i].source < list[j].source
	})
	fmt.Fprintf(out, "Not checked in the last %d days: %s on %s. Re-read each page, correct what changed, and set checkedAt (and verified) on what it confirms.\n\n", days, plural(total, "value"), plural(len(list), "page"))
	fmt.Fprintln(out, "| Page | Last checked | Values |")
	fmt.Fprintln(out, "| --- | --- | --- |")
	for _, p := range list {
		when := p.oldest
		if when == "" {
			when = "never"
		}
		fmt.Fprintf(out, "| %s | %s | %s |\n", p.source, when, strings.Join(shortList(p.ids, 5), ", "))
	}
	return nil
}

func plural(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return fmt.Sprintf("%d %ss", n, noun)
}

// shortList is the first n ids and how many more there are.
func shortList(ids []string, n int) []string {
	if len(ids) <= n {
		return ids
	}
	return append(append([]string(nil), ids[:n]...), fmt.Sprintf("and %d more", len(ids)-n))
}
