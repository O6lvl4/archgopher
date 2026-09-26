package engine

import (
	"fmt"
	"maps"
	"sort"

	"github.com/O6lvl4/archgopher/book"
	"github.com/O6lvl4/archgopher/model"
)

// withApplied puts an account's own quota values over the published
// defaults, for one run. A value for a quota no book defines is a warning.
func withApplied(books book.Books, applied map[string]model.AppliedQuota) (book.Books, []string) {
	if len(applied) == 0 {
		return books, nil
	}
	quotas := maps.Clone(books.Quotas)
	var warnings []string
	for _, id := range sortedIDs(applied) {
		a := applied[id]
		e, ok := quotas[id]
		if !ok {
			warnings = append(warnings, fmt.Sprintf("quotas: no quota %q in the books", id))
			continue
		}
		e.Values = map[string]book.Value{book.AnyRegion: {Value: &a.Value, Verified: true, CheckedAt: a.CheckedAt}}
		e.Per, e.Source, e.Applied = 0, a.Source, a.Source
		quotas[id] = e
	}
	books.Quotas = quotas
	return books, warnings
}

func sortedIDs(m map[string]model.AppliedQuota) []string {
	ids := make([]string, 0, len(m))
	for id := range m {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}
