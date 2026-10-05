# Catalog drift review

`monitor.py` discovers upstream changes and prepares review Issues. It never
changes catalog definitions or prices, runs `terraform plan/apply`, creates cloud
resources, configures accounts, or creates credentials.

## What is checked

- AWS (`hashicorp/aws`), Azure (`hashicorp/azurerm`), Google Cloud
  (`hashicorp/google`), and Cloudflare (`cloudflare/cloudflare`): latest stable
  Terraform Registry release, exported with `terraform providers schema -json` in
  an isolated temporary configuration. Added/removed resource types are detected;
  structural schema changes are compared for modeled and explicitly free types.
  Description-only changes and provider version bumps alone are ignored.
- ConoHa: the [Aid-On fork](https://github.com/Aid-On/terraform-provider-conohavps)
  resource documentation and matching implementation file hashes at its current
  HEAD. This is **not an executed schema**; prose-only changes can need review,
  and undocumented resource registrations can be missed.
- Pricing: the distinct official source URLs already referenced by
  `catalog/*/*/books/prices.json`, excluding FX rows. Normalized HTML/PDF text
  keeps table order, plans, units, currencies, included usage, tiers, tax and
  commitment/renewal terms. A changed document is a **review candidate**, not
  proof that a rate changed or a product is billable. Dynamic/API-only prices,
  new products outside the linked pages, private discounts and SKU/region
  additions absent from those pages are not complete coverage.

The existing Monday 03:00 UTC price workflow continues to update existing mapped
AWS/Azure rates (and GCP only when credentials were separately configured). It
now writes machine-readable coverage, checks the resulting books, opens a draft
price PR, and reports failures or credential skips as a deduplicated Issue.
It does not discover arbitrary new SKUs, tiers or billing dimensions.

## Safe local dry run

Requirements: Python 3.12, Terraform 1.14.5, `pdftotext`, and public network access.
The monitor uses the standard Python library only.

```sh
python3 -m unittest discover -s tools/drift -v
python3 tools/drift/monitor.py
# Optional isolated inspection; use different state files for partial runs:
python3 tools/drift/monitor.py --only prices \
  --state .drift/prices-state.json --out .drift/prices-report.json
```

Without `--publish`, the report contains proposed Issue actions and no GitHub
mutation occurs. The dry run still updates its local state, so use a fresh state
path to test initialization and retain it for repeat-run comparison. `GH_TOKEN`
is optional for public read-only requests; if provided, it is sent only to the
GitHub API and is never inherited by Terraform subprocesses.

The first successful observation of each source creates its baseline without
opening historical backlog Issues. Sources that fail are combined in one health
finding. HTTP errors, partial responses, unusable pages, truncated ConoHa trees,
invalid schemas and substantial pricing-page content loss do not replace the
last good snapshot. Conservative shrink detection may also flag a legitimate
major page redesign: review it before deliberately replacing that source's
baseline. No checker can establish that an initially fetched page is exhaustive.

Reports contain full snapshots/differences, coverage, limitations and proposed
or completed actions. Any failed source gives exit status 1, even if other
sources succeeded. `--only` is for isolated local diagnostics, not the scheduled
baseline: it checks only that subset and reports health only for that subset.

## Schedule, state and review decisions

`.github/workflows/drift.yml` runs daily at 05:23 UTC **after merge to main**.
It does not run on pull requests, feature branches or forks. Manual dispatch
on main defaults to a dry run; set `publish=true` deliberately to publish.
Only the built-in repository token is used. No secrets or cloud credentials are
added by this change.

The latest retained `catalog-drift-state` artifact from this repository's trusted
main-branch scheduled/manual runs supplies the baseline and provider cache.
Successful source observations and pending findings are persisted even when
other checks or publication fail. Artifacts are retained for 90 days and normal
daily runs renew them. Loss/expiry after detection has run fails closed rather
than silently forgetting history. A first run that failed before detection does
not prevent initial bootstrap. The workflow refuses parallel runs.

For state recovery, restore a reviewed copy of `state.json` (and optional
`cache/`) from a trusted run, run the monitor locally without `--publish` to
verify it, and restore it through a reviewed workflow change or an authorized
Actions run that uploads `catalog-drift-state`. Never substitute an untrusted
PR artifact or delete state merely to make a red run green. An intentional
source rebaseline should preserve every other source and finding and record why
that source's old evidence was discarded.

Each stable subject has at most one maintained open bot Issue. New evidence
updates the bot-owned block while preserving the Issue title, comments and text
outside the block. An edited/missing block pauses only that subject and makes
the run incomplete; restore its marker or label it `drift:ignore` to resolve it.
Closing an Issue acknowledges that exact fingerprint; it is
never reopened for identical evidence. Later new evidence can create a linked
successor. A `drift:ignore` label on a matching bot Issue, open or closed, pauses
all publication for that subject until the label is removed. A full upstream
reversion updates an existing open Issue explicitly and never creates an empty
new one. Human-created Issues and pull requests are never rewritten.

Issues identify the official source, affected price files or resource types,
observed difference, run evidence, and an acceptance checklist. Review billability,
free/helper classification, region/SKU, unit and currency, tiers and included
pools, term/tax/discount validity, then add reproducible tests before a draft PR.
No automatic merge or deployment is part of either workflow.
