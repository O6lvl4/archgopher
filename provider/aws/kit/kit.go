// Package kit is what one AWS service package hands to the provider: its
// scouters, its reference books, its piece of the Terraform rules and the IAM
// actions that carry load to its resources. Each service package fills one
// Service; the provider combines them.
package kit

import (
	"io/fs"

	"github.com/O6lvl4/arch-scouter/book"
	"github.com/O6lvl4/arch-scouter/scouter"
	"github.com/O6lvl4/arch-scouter/terraform/infer"
)

// Actions maps a resource type to kinds of work and the IAM actions that do that work.
type Actions map[string]map[string][]string

// Service is one AWS service's contribution.
type Service struct {
	Name     string
	Scouters []scouter.Scouter
	// Books holds books/prices.json, books/quotas.json and books/slas.json (any may be missing).
	Books     fs.FS
	Terraform infer.Rules
	Actions   Actions
}

// LoadBooks reads the service's books.
func (s Service) LoadBooks() (book.Books, error) {
	if s.Books == nil {
		return book.Books{Prices: book.Book{}, Quotas: book.Book{}, SLAs: book.Book{}}, nil
	}
	return book.Load(s.Books, "books")
}

// Nodes lists the Terraform types of the service's scouters, which become nodes.
func (s Service) Nodes() map[string]bool {
	out := map[string]bool{}
	for _, sc := range s.Scouters {
		if !sc.Meta().External {
			out[sc.Meta().Type] = true
		}
	}
	return out
}
