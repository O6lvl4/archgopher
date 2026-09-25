# Architecture and limits

## Architecture

Packages are cut by the reason they change, and imports point one way, toward
what changes least. A test in [`internal/layers`](../internal/layers) fails when
an import breaks the rule, and `web/scripts/layers.mjs` does the same for the UI.

```text
cmd/archgopher, cmd/wasm          edges of the system
        │
       api ── gaps                  JSON in, JSON out; what the declaration does not know
        │
provider/aws ── iam, schedule,     loads the catalog; IAM edges, schedule syntax,
   │      │     pattern            account-wide rules; L3 patterns
   │   definition ── catalog/aws    resources as data, one directory per type
   │      │
   │   terraform/infer ── merge     resources → declaration; fold into edits
   │   terraform/eval ── config     static HCL evaluation; module parsing
   │
pattern ── engine ── report         L3 expansion; the computation; output
   │         │
facet ── scouter                    L2 reusable readings; how one type is read
   │         │
 meter ── field                     L1 readings (quantity × price id); schemas
   │         │
 book      model                    reference books; the declaration
```

| Layer | Package | Holds |
| --- | --- | --- |
| Vocabulary | [`model`](../model), [`field`](../field), [`book`](../book) | The declaration, tag-derived schemas, reference books |
| L1 | [`meter`](../meter) | The smallest readings: a cost is quantity × price id, a limit is peak demand ÷ quota id, units checked |
| L2 | [`facet`](../facet) | Reusable readings with their own assumption structs: requests in size chunks, GB-seconds, storage, provisioned capacity, Little's law concurrency, logs, tokens |
| Scouter | [`scouter`](../scouter) | How one resource type is read: catalog entry, fields, and a function built from facets |
| Engine | [`engine`](../engine) | Validation, load propagation in topological order, path composition. No provider knowledge |
| Gaps | [`gaps`](../gaps) | What a declaration does not know yet, found from the readings and the scouters' fields. No provider knowledge |
| L3 | [`pattern`](../pattern) | Reusable architectures that expand into nodes and edges, then roll up |
| Terraform | [`terraform/config`](../terraform/config), [`eval`](../terraform/eval), [`infer`](../terraform/infer), [`merge`](../terraform/merge) | Parse, evaluate, infer a graph, merge into edits. No provider knowledge |
| Resources | [`definition`](../definition), [`catalog/aws`](../catalog/aws) | Resources as data, one directory per type: a definition compiles into a scouter built from facets |
| AWS | [`provider/aws`](../provider/aws) | Loads the catalog and adds IAM edges, the schedule syntax, account-wide Terraform rules and the L3 patterns |

A pattern is placed as one node and reads exactly like the nodes it expands
into (a test checks this):

```yaml
- id: orders
  type: aws.pattern.serverless_api       # API Gateway → Lambda → DynamoDB
  assumptions: { durationMs: 90, itemSizeKb: 3, storageGb: 40 }
```

The UI follows the same rule: `lib` ← `ui` ← `composites` ← `features` ← `app`,
and a feature never imports another feature.

## Limits

- Latency and availability are only as good as your per-hop assumptions and
  the published SLAs. Path p99 is the sum of hop p99s, an upper bound.
- Free tiers are not counted: the reading is what the architecture costs
  month after month. Volume tiers and fees paid once are counted over the
  whole declaration, which is one account.
- Auto scaling, caching behaviour and retries are not modelled. Express them
  through `perUnit` and assumptions.
- One region per declaration. A declaration that mixes AWS, Azure and Google
  Cloud prices every node in that one region, so nodes of the other clouds
  find no price. Cloudflare prices are the same everywhere and price in any
  region.
