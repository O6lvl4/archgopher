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
	"github.com/O6lvl4/archgopher/cloud"
	"github.com/O6lvl4/archgopher/engine"
	"github.com/O6lvl4/archgopher/model"
	"github.com/O6lvl4/archgopher/provider/aws/servicequotas"
	"github.com/O6lvl4/archgopher/provider/gcp/billingcatalog"
	"github.com/O6lvl4/archgopher/provider/gcp/cloudquotas"
)

// cmdQuotas reads the values an account runs against, for every quota the
// declaration's limits use, and writes the declaration with them under
// quotas: so its headroom is the account's own. AWS quotas come from
// Service Quotas for a profile, Google Cloud quotas from the Cloud Quotas API
// for a project.
func cmdQuotas(args []string, out io.Writer) error {
	fs := flag.NewFlagSet("quotas", flag.ContinueOnError)
	profile := fs.String("profile", servicequotas.EnvProfile(), "AWS CLI profile of the account (default ARCHGOPHER_AWS_PROFILE or AWS_PROFILE)")
	project := fs.String("project", "", "Google Cloud project whose quotas to read (credentials as for sync: ARCHGOPHER_GCP_ACCOUNT, _TOKEN)")
	file := fs.String("o", "", "write to this file instead of stdout")
	codesFile := fs.String("codes", "", "JSON file mapping quota ids to quota codes ({\"aws.lambda.concurrent_executions\": {\"source\": \"servicequotas\", \"service\": \"lambda\", \"quota\": \"L-B99A9384\"}}), over the codes in the books")
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
	acct := &account{profile: *profile, project: *project}
	read, kept, err := applyAccountQuotas(&spec, acct, codes)
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "%d quotas read from the account\n", read)
	if len(kept) > 0 {
		fmt.Fprintf(os.Stderr, "published default kept, no account value: %s\n", strings.Join(kept, ", "))
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

// account reads quota values from one AWS account and one Google Cloud
// project, creating each client on first use.
type account struct {
	profile, project string
	aws              *servicequotas.Client
	gcp              *cloudquotas.Client
}

// read is a quota's value in the account, in the book's unit, and where it
// came from; errAbsent when the account has no value for it.
func (a *account) read(code json.RawMessage, region string) (float64, string, error) {
	var probe struct {
		Source string `json:"source"`
	}
	if err := json.Unmarshal(code, &probe); err != nil {
		return 0, "", err
	}
	switch probe.Source {
	case "", servicequotas.Source:
		return a.readAWS(code, region)
	case cloudquotas.Source:
		return a.readGCP(code, region)
	}
	return 0, "", errAbsent
}

var errAbsent = errors.New("no account value")

func (a *account) readAWS(code json.RawMessage, region string) (float64, string, error) {
	var q servicequotas.Spec
	if err := json.Unmarshal(code, &q); err != nil {
		return 0, "", err
	}
	if a.aws == nil {
		a.aws = servicequotas.NewClient()
		a.aws.Credentials = func() (servicequotas.Credentials, error) { return servicequotas.ProfileCredentials(a.profile) }
	}
	v, err := a.aws.Applied(q, region)
	if errors.Is(err, servicequotas.ErrAbsent) {
		return 0, "", errAbsent
	}
	return q.PerUnit(v.Value), fmt.Sprintf("Service Quotas %s %s, profile %s", v.QuotaCode, v.Label(), a.profile), err
}

func (a *account) readGCP(code json.RawMessage, region string) (float64, string, error) {
	var q cloudquotas.Spec
	if err := json.Unmarshal(code, &q); err != nil {
		return 0, "", err
	}
	if a.project == "" {
		return 0, "", fmt.Errorf("Google Cloud quotas need --project")
	}
	if a.gcp == nil {
		auth, err := billingcatalog.EnvAuth()
		if err != nil {
			return 0, "", err
		}
		a.gcp = cloudquotas.NewClient(auth)
	}
	info, err := a.gcp.Info(a.project, q)
	if err == nil {
		var v float64
		if v, err = info.Value(region); err == nil {
			return q.PerUnit(v), fmt.Sprintf("Cloud Quotas %s %s, project %s", q.Service, info.Label(), a.project), nil
		}
	}
	if errors.Is(err, cloudquotas.ErrAbsent) {
		return 0, "", errAbsent
	}
	return 0, "", err
}

// applyAccountQuotas sets spec.Quotas from the account for the quotas the
// declaration reads, and lists those it has no value for.
func applyAccountQuotas(spec *model.Spec, a *account, codes map[string]json.RawMessage) (int, []string, error) {
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
		code, ok := codes[id]
		if !ok {
			code = books.Quotas[id].Sync
		}
		if len(code) == 0 {
			kept = append(kept, id)
			continue
		}
		v, source, err := a.read(code, spec.Region)
		if errors.Is(err, errAbsent) {
			kept = append(kept, id)
			continue
		}
		if err != nil {
			return 0, nil, fmt.Errorf("%s: %w", id, err)
		}
		spec.Quotas[id] = model.AppliedQuota{Value: v, Source: source, CheckedAt: today}
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

// readCodes reads a file of quota codes by quota id; none is empty.
func readCodes(path string) (map[string]json.RawMessage, error) {
	codes := map[string]json.RawMessage{}
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
