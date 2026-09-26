package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/O6lvl4/archgopher/api"
	"github.com/O6lvl4/archgopher/book"
	"github.com/O6lvl4/archgopher/cloud"
	"github.com/O6lvl4/archgopher/engine"
	"github.com/O6lvl4/archgopher/model"
	"github.com/O6lvl4/archgopher/provider/aws/servicequotas"
)

// cmdQuotas reads from Service Quotas the values an account runs against,
// for every quota the declaration's limits use, and writes the declaration
// with them under quotas: so its headroom is the account's own.
func cmdQuotas(args []string, out io.Writer) error {
	fs := flag.NewFlagSet("quotas", flag.ContinueOnError)
	profile := fs.String("profile", servicequotas.EnvProfile(), "AWS CLI profile of the account (default ARCHGOPHER_AWS_PROFILE or AWS_PROFILE)")
	file := fs.String("o", "", "write to this file instead of stdout")
	codesFile := fs.String("codes", "", "JSON file mapping quota ids to Service Quotas codes ({\"aws.lambda.concurrent_executions\": {\"service\": \"lambda\", \"quota\": \"L-B99A9384\"}}), over the codes in the books")
	if err := fs.Parse(reorder(args)); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("quotas takes one declaration file")
	}
	data, err := os.ReadFile(fs.Arg(0))
	if err != nil {
		return err
	}
	spec, err := model.ParseSpec(data)
	if err != nil {
		return err
	}
	codes, err := readCodes(*codesFile)
	if err != nil {
		return err
	}
	c := servicequotas.NewClient()
	c.Credentials = func() (servicequotas.Credentials, error) { return servicequotas.ProfileCredentials(*profile) }
	read, kept, err := applyAccountQuotas(&spec, c, *profile, codes)
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "%d quotas read from the account (profile %s)\n", read, *profile)
	if len(kept) > 0 {
		fmt.Fprintf(os.Stderr, "published default kept, not in Service Quotas: %s\n", strings.Join(kept, ", "))
	}
	text, err := api.MarshalYAML(spec)
	if err != nil {
		return err
	}
	if *file != "" {
		return os.WriteFile(*file, []byte(text), 0o644)
	}
	_, err = io.WriteString(out, text)
	return err
}

// applyAccountQuotas sets spec.Quotas from the account for the quotas the
// declaration reads, and lists those without a Service Quotas code.
func applyAccountQuotas(spec *model.Spec, c *servicequotas.Client, profile string, codes map[string]servicequotas.Spec) (int, []string, error) {
	res, err := api.Scout(*spec)
	if err != nil {
		return 0, nil, err
	}
	books, err := cloud.Books()
	if err != nil {
		return 0, nil, err
	}
	if spec.Quotas == nil {
		spec.Quotas = map[string]model.AppliedQuota{}
	}
	today := time.Now().Format("2006-01-02")
	read, kept := 0, []string{}
	for _, id := range quotaIDs(res) {
		q, ok := codes[id]
		if !ok {
			q, ok = quotaSpec(books.Quotas[id])
		}
		if !ok {
			kept = append(kept, id)
			continue
		}
		v, err := c.Applied(q, spec.Region)
		if errors.Is(err, servicequotas.ErrAbsent) {
			kept = append(kept, id)
			continue
		}
		if err != nil {
			return 0, nil, fmt.Errorf("%s: %w", id, err)
		}
		spec.Quotas[id] = model.AppliedQuota{Value: q.PerUnit(v.Value), Source: fmt.Sprintf("Service Quotas %s %s, profile %s", v.QuotaCode, v.Label(), profile), CheckedAt: today}
		read++
	}
	return read, kept, nil
}

// quotaIDs are the quotas a result's limits read, once each, sorted.
func quotaIDs(res engine.Result) []string {
	seen := map[string]bool{}
	for _, n := range append(append([]engine.NodeResult(nil), res.Nodes...), res.Groups...) {
		for _, l := range n.Limits {
			if l.QuotaID != "" {
				seen[l.QuotaID] = true
			}
		}
	}
	ids := make([]string, 0, len(seen))
	for id := range seen {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// quotaSpec is the Service Quotas code a quota entry syncs from, if any.
func quotaSpec(e book.Entry) (servicequotas.Spec, bool) {
	var q servicequotas.Spec
	if len(e.Sync) == 0 || json.Unmarshal(e.Sync, &q) != nil || q.Source != servicequotas.Source {
		return servicequotas.Spec{}, false
	}
	return q, true
}

// readCodes reads a file of Service Quotas codes by quota id; none is empty.
func readCodes(path string) (map[string]servicequotas.Spec, error) {
	codes := map[string]servicequotas.Spec{}
	if path == "" {
		return codes, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(data, &codes); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return codes, nil
}
