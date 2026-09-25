package definition

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/O6lvl4/archgopher/book"
)

// Unit is one directory of a catalog: a resource (resource.yaml), the books
// it owns and its test cases. A unit without resource.yaml only shares books
// that several resources read, such as log prices.
type Unit struct {
	Name     string
	Dir      string
	Resource *Resource
	Books    book.Books
	Cases    []Case
}

// LoadAll reads every unit directly under root, a provider's catalog
// directory whose name is the provider of every resource in it.
func LoadAll(fsys fs.FS, root string) ([]Unit, error) {
	entries, err := fs.ReadDir(fsys, root)
	if err != nil {
		return nil, err
	}
	var units []Unit
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		u, err := Load(fsys, path.Join(root, e.Name()))
		if err != nil {
			return nil, err
		}
		if u.Resource != nil {
			// The catalog directory names the cloud: catalog/aws, catalog/azure.
			u.Resource.provider = path.Base(root)
		}
		units = append(units, u)
	}
	sort.Slice(units, func(i, j int) bool { return units[i].Name < units[j].Name })
	return units, nil
}

// Load reads one unit.
func Load(fsys fs.FS, dir string) (Unit, error) {
	u := Unit{Name: path.Base(dir), Dir: dir}
	var err error
	if u.Books, err = book.Load(fsys, path.Join(dir, "books")); err != nil {
		return u, err
	}
	data, err := fs.ReadFile(fsys, path.Join(dir, "resource.yaml"))
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return u, nil
	case err != nil:
		return u, err
	}
	var f File
	if err := strict(data, &f); err != nil {
		return u, fmt.Errorf("%s/resource.yaml: %w", dir, err)
	}
	if f.Type != u.Name {
		return u, fmt.Errorf("%s/resource.yaml: type %q must match the directory name", dir, f.Type)
	}
	if u.Resource, err = Compile(f); err != nil {
		return u, fmt.Errorf("%s: %w", dir, err)
	}
	cases, err := fs.ReadFile(fsys, path.Join(dir, "cases.yaml"))
	if err == nil {
		if err := strict(cases, &u.Cases); err != nil {
			return u, fmt.Errorf("%s/cases.yaml: %w", dir, err)
		}
	}
	return u, nil
}

func strict(data []byte, out any) error {
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	return dec.Decode(out)
}

// FreeTypes reads root/free.txt: resource types that cost nothing by
// themselves, one per line, with # comments. A catalog without one has none.
func FreeTypes(fsys fs.FS, root string) (map[string]bool, error) {
	data, err := fs.ReadFile(fsys, path.Join(root, "free.txt"))
	if errors.Is(err, fs.ErrNotExist) {
		return map[string]bool{}, nil
	}
	if err != nil {
		return nil, err
	}
	out := map[string]bool{}
	for _, line := range strings.Split(string(data), "\n") {
		if t := strings.TrimSpace(line); t != "" && !strings.HasPrefix(t, "#") {
			out[t] = true
		}
	}
	return out, nil
}
