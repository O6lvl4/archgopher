---
name: archgopher-gaps
description: Fill what an archgopher declaration built from infrastructure code cannot know (entry load, calls made outside that code, call ratios, unknown numbers) from the application code and the cloud's own measurements, then reconcile the result with the bill. Use when a declaration comes from `archgopher tf`, when `archgopher gaps` lists anything, or when a reading and the bill disagree.
---

# Filling the gaps of a declaration

Infrastructure code says what exists and what may call what. It does not say
how much is called, by which code path, or which calls happen outside it.
`archgopher gaps` lists those unknowns; this procedure fills them. It holds for
every provider: the sources differ only by name (see [Sources](#sources)).

## Loop

1. Build or refresh the declaration. Merging keeps everything filled before.
   ```sh
   archgopher tf <iac-dir> -o spec.yaml                  # first time
   archgopher tf <iac-dir> --merge spec.yaml -o spec.yaml # afterwards
   ```
2. List the unknowns: `archgopher gaps spec.yaml --json`.
3. Fill each gap as [below](#gaps), writing where every number came from in
   the node's or the edge's `note`.
4. Read it: `archgopher scout spec.yaml`, then [reconcile](#reconcile).
5. Repeat until `gaps` is empty or what remains is written down as unknowable.

## Gaps

| Kind | What it means | How to fill it |
| --- | --- | --- |
| `load` | An entry has no volume, so nothing downstream is read | `load` or `traffic` from the front door's request count or access log for a window; `traffic` when the plan speaks of users, schedules or batches |
| `caller` | A node's readings grow with work, but nothing sends it any | Find the call outside the infrastructure code (see [Calls it does not show](#calls-it-does-not-show)) and add the edge; or give the node its own `load` / `traffic` (a job run by hand) |
| `assumption` | A number the reading needs is unknown | Measure it (duration, stored size, item size, run time) over the same window as the counts |
| `ratio` | An edge sends one call per upstream unit and nobody said so | `perUnit` = downstream count / upstream count over one window. Several kinds or sizes on one call: `ops`. If one per unit is right, say why in `note` and the gap closes |
| `failed` | The node could not be read for another reason | Read the message; it names the missing piece |

### Calls it does not show

Look for these in the application code and the account, not in the
infrastructure code:

- **Code paths.** A handler that serves several routes calls a dependency on
  some of them only. The edge's `perUnit` is that route's share, not 1.
- **Loops.** An agent calls its model several times per turn (tool use); a
  client retries. Count calls, not requests.
- **Once per session.** Work done on the first turn only divides by sessions,
  not by calls.
- **Callers made elsewhere.** Roles, jobs or functions defined in another
  repository, another account or project, or by hand, that write to a table,
  a queue or a bucket.
- **Implicit callers.** The platform calls a node for you: an encryption key
  used by storage and databases, secrets and parameters read at start-up,
  network hubs carrying traffic between networks.
- **Size.** Where the target bills by size (a 4 KB read unit, a 64 KB message
  chunk), give each operation its `kb`; billing rounds per operation, so an
  average size is wrong for mixed sizes.

## Rules

- **One window for everything.** Take counts, sizes and the bill from the
  same period and convert to a month of 730 hours.
- **`perUnit` is per unit of what the source receives.** An agent receives
  sessions, so its edges are per session even if it is invoked more often.
- **Check what a metric already includes.** Some counters are already billing
  units with a factor applied (consumed capacity after the consistency
  discount). Divide them back into operations, or the factor applies twice.
- **Provenance goes in `note`.** Source, window, raw counts:
  `note: 18 model calls over 10 sessions, 2026-09-10..23`. YAML comments are
  lost when `tf --merge` rewrites the file.
- **Read, never change, what is measured.** Production data and metrics are
  read-only inputs. Leave Terraform-derived `attributes` to the next merge.
- **Traces are a bonus.** A trace service map gives edges and counts at once;
  use it when it exists, never require it.

## Reconcile

Compare each node's reading with the bill's usage quantities for the window,
not only the totals: totals hide errors that cancel out. Aim for a few
percent per node and explain what remains (resources outside the
infrastructure code, a bucket still growing, a price that changed).

## Sources

The kinds of measurement are the same everywhere; only the names differ.

| Measurement | AWS | Azure | Google Cloud | Cloudflare |
| --- | --- | --- | --- | --- |
| Counts per resource and operation | CloudWatch metrics | Azure Monitor metrics | Cloud Monitoring | GraphQL Analytics API |
| Front door access log, per route | API Gateway / ALB / CloudFront logs | Application Gateway / Front Door logs | Load Balancing / API Gateway logs | Logpush, Workers analytics |
| Billed usage quantities | Cost Explorer, CUR | Cost Management exports | Billing export to BigQuery | Billing usage |
| Traces (optional) | X-Ray, OpenTelemetry | Application Insights | Cloud Trace | Workers tracing |
