<h1 align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="assets/logo-dark.png">
    <img src="assets/logo.png" alt="archgopher" width="280">
  </picture>
</h1>

Read an architecture before you build it. archgopher turns Terraform into a
graph of resources, pushes your expected load through it, and reads every node
on four dimensions:

| Dimension | What you get | How paths combine |
| --- | --- | --- |
| Cost | Monthly USD, broken down into priced components | Sum over nodes |
| Headroom | Peak demand against service quotas and configured capacity | Per node |
| Latency | p50 and p99 per hop, from your assumptions | Sum along a path (p99 is an upper bound) |
| Availability | The service SLA | Product along a path |

Cost tools such as [Infracost](https://github.com/infracost/infracost) price
each resource on its own. archgopher asks a different question: *if 30
million requests a month arrive at the front door, what does every resource
behind it see, what does it cost, and where does it run out of room first?*

[日本語の README](README.ja.md)

## Quick start

```sh
go install github.com/O6lvl4/archgopher/cmd/archgopher@latest

# 1. Build a declaration from Terraform. No terraform init, plan or credentials.
archgopher tf ./infra -o app.scouter.yaml

# 2. Fill in what Terraform cannot know: the load at the entry and the
#    assumptions left as null (duration per call, item size, ...).
$EDITOR app.scouter.yaml

# 3. Read it.
archgopher scout app.scouter.yaml          # Markdown tables
archgopher scout app.scouter.yaml --json   # machine-readable

# 4. After Terraform changes, fold them in. Your assumptions, load, notes and
#    edges stay; only types and attributes follow Terraform.
archgopher tf ./infra --merge app.scouter.yaml -o app.scouter.yaml
```

A worked example lives in [`examples/serverless-api`](examples/serverless-api):
a fictional notes app with CloudFront, API Gateway, Lambda (in a local module),
DynamoDB, SQS, S3, an hourly cleanup job, and a Bedrock model added by hand.

```text
| Node         | Type          | Monthly  | Tightest headroom | p99      | SLA     |
| api_handler  | Lambda        | $12.00   | 99.4%             | 400 ms   | 99.950% |
| notes        | DynamoDB      | $10.14   | 99.9%             | -        | 99.990% |
| summarizer   | Bedrock model | $1452.00 | 93.4%             | 3,000 ms | 99.900% |
...
```

## Web UI

The same engine runs in the browser as WebAssembly. Import a Terraform folder
(read locally, never uploaded), place and connect nodes, fill in assumptions,
and every edit re-reads the whole graph. Declarations open and save as the
same YAML the CLI reads.

```sh
cd web
pnpm install
pnpm run dev      # builds the engine to WebAssembly, then serves http://localhost:5176
```

The UI holds no formulas. It asks the engine for the catalog of scouters and
builds every form from the fields the Go structs declare.

## The declaration

```yaml
name: Notes app
region: ap-northeast-1
nodes:
  - id: users
    type: entry
    load: { monthly: 30000000, peakPerSecond: 120 }
  - id: api_handler
    type: aws_lambda_function                       # the scouter; for Terraform resources, the resource type
    address: module.api_handler.aws_lambda_function.this
    attributes: { memory_size: 512, architectures: [arm64] }   # from Terraform (snake_case)
    assumptions: { durationMs: 120, latencyP99Ms: 400 }        # yours (camelCase)
edges:
  - { from: users, to: api_handler }
  - { from: api_handler, to: notes, kind: write, perUnit: 0.2 } # 1 in 5 calls writes
```

| Field | Meaning |
| --- | --- |
| `load` | Monthly volume (drives cost) and peak per second (drives headroom). A node with load is an entry. |
| `attributes` | Terraform attributes the scouter reads. `tf` fills them; you can also write them. |
| `assumptions` | Numbers Terraform cannot know. A missing required assumption is an error on that node, never a silent default. |
| `kind` | The work the downstream node receives (`read` / `write` for DynamoDB). Defaults to the node's first kind. |
| `perUnit` | Downstream units per upstream unit. Defaults to 1. |

Load flows in topological order: a node's total throughput times `perUnit`
lands on the downstream node under `kind`. Cycles are errors.

Unknown keys are errors too. A typo such as `durationMS` fails loudly instead
of being ignored.

## Terraform import

`archgopher tf` evaluates HCL statically with
[hashicorp/hcl](https://github.com/hashicorp/hcl) and
[go-cty](https://github.com/zclconf/go-cty). It runs without `terraform init`,
without state and without cloud credentials, so it also works on a pull request.

- **Values.** Variables (defaults, `terraform.tfvars`, `*.auto.tfvars`,
  `--var-file`, `--var`), locals, module inputs and outputs, `count`,
  `for_each`, conditionals and the common built-in functions are evaluated.
  A resource whose `count` is 0 disappears, and so does a module.
- **Unknowns.** Values that exist only after apply (ARNs, IDs) stay unknown.
  Edges do not need them: they come from references.
- **Edges.** A node that references another node calls it (a Lambda whose
  environment names a table). Helper resources connect nodes (API Gateway
  integrations, event source mappings, SNS subscriptions, EventBridge targets,
  S3 notifications). IAM policies attached to a node's role add edges with
  kinds: `dynamodb:PutItem` becomes a `write` edge, `s3:GetObject` a `read`
  edge. Policies written with `jsonencode`, `aws_iam_policy_document` and
  `dynamic "statement"` blocks fed from module variables are all followed
  element by element.
- **Entries.** Front doors nobody calls (CloudFront, API Gateway, load
  balancers, Cognito, Lambda function URLs) get a shared `users` entry.
  `rate()` and `cron()` schedules become the load of their rule.
- **Mentions.** A reference to a CloudFront distribution (a callback URL, a
  link in an invitation email) is a mention, not a call, and makes no edge.
- **Resources that are off.** Resources and modules whose `count` or
  `for_each` is 0 with the current variables are listed in a warning, so you
  can pass `--var` to include them.
- **Modules.** Local sources are read directly. Registry and git sources are
  read from `.terraform/modules` when `terraform init` has run; otherwise they
  are reported and skipped.
- **Cycles** (a callback URL, mutual references) are broken by dropping the
  closing edge, with a warning.

Resource types that should be nodes but have no scouter yet (ECS services,
Kinesis streams, load balancers...) still become nodes: they pass load through
and appear in the report as skipped.

## Reference books

Prices, quotas and SLAs live next to each resource in `catalog/aws/<type>/books`, one row per ID with
a value per region, a unit, a source URL and a `verified` flag.

- **Units are checked.** A reading that counts `GB` against a price per
  `GB-month` is an error, not a wrong number.
- **Unverified values are listed.** Any value nobody has checked, or that is
  unknown, appears at the end of every report.
- **Prices sync from the public Price List.** `archgopher sync` reads the
  AWS Price List bulk files (no credentials) and marks each price verified.
  A weekly workflow opens a pull request when a price changes.
  `archgopher explore <service> <region> [attr=regex...]` helps you write the
  filters for a new price.

```sh
archgopher sync --check      # exit 1 if the book is out of date
archgopher sync --add-regions eu-west-2   # add a region to every price
archgopher explore AWSLambda ap-northeast-1 'usagetype=.*GB-Second.*'
```

**Regions.** Ten regions are covered: us-east-1, us-east-2, us-west-2,
eu-west-1, eu-central-1, ap-northeast-1, ap-northeast-2, ap-southeast-1,
ap-southeast-2 and ap-south-1. A row that is the same everywhere is `*`; a
quota that differs names its regions over a `*` default ("2,500 elsewhere").
`sync --add-regions` reads every price from the Price List for the new region
and lists anything left to fill by hand. Where the Price List has no price, the
row is recorded as not offered, and a node that needs it fails with "not
offered in <region>" instead of costing nothing. A test keeps every region
complete.

Quotas are the published defaults. Some are account-specific in practice
(Lambda concurrency on new accounts, Bedrock tokens per minute), and their
notes say so. AgentCore publishes no SLA, so its availability stays unknown. A value AWS does not publish stays unknown: the report shows
the demand and says the capacity is unknown.

## Scouters

| Type | Reads | Headroom |
| --- | --- | --- |
| `aws_lambda_function` | Requests, GB-seconds (x86_64 / arm64), logs | Concurrency (Little's law: peak rate × duration) against the account or reserved concurrency |
| `aws_api_gateway_rest_api` | Requests | Account throttle |
| `aws_apigatewayv2_api` | Requests in 512 KB steps (HTTP APIs) | Account throttle |
| `aws_cloudfront_distribution` | HTTPS requests, transfer out, by the price zone of the viewers (the region's zone unless set) | Requests per distribution |
| `aws_dynamodb_table` | On-demand request units or provisioned capacity (4 KB / 1 KB steps, consistency, transactions), storage | Table throughput or provisioned capacity |
| `aws_s3_bucket` | GET, PUT, storage (Standard), direct transfer out to the internet | Per-prefix request rate × prefixes |
| `aws_sqs_queue` | Requests in 64 KB chunks | FIFO send rate, by region in high throughput mode |
| `aws_sns_topic` | Publishes in 64 KB chunks | Publish rate |
| `aws_sfn_state_machine` | Transitions (standard), requests and GB-seconds (express) | StartExecution rate |
| `aws_rds_cluster` | Aurora Serverless v2 ACU-hours, storage, I/O (Standard or I/O-Optimized), the managed master password secret | Peak ACU against max capacity |
| `aws_scheduler_schedule` | Invocations | - |
| `aws_cloudwatch_event_rule` | Nothing (scheduled rules are free) | - |
| `aws_ecs_task_definition` | Fargate vCPU and GB hours per run (1-minute minimum, x86_64 or arm64), ephemeral storage above 20 GB | vCPUs running against the Fargate quota, task launch rate, RunTask calls |
| `aws_ecr_repository` | Image storage | Image pulls and layer downloads per second |
| `aws_kms_key` | Key-months with rotated versions, requests | Cryptographic requests per second (varies by region). References to a key are mentions: connect callers by hand |
| `aws_secretsmanager_secret` | Secret-months with replicas, API calls | GetSecretValue rate |
| `aws_ssm_parameter` | Advanced parameter-hours, API interactions (advanced or higher throughput) | GetParameter rate, standard or higher throughput |
| `aws_cognito_user_pool` | Monthly active users by feature plan (Lite tiered, Essentials, Plus) | Authentication and user creation rates |
| `aws_wafv2_web_acl` | Web ACL-months, rule-months, requests by inspection capacity | Requests per web ACL (regional) |
| `aws_cloudfront_function` | Invocations | - |
| `aws_vpc_endpoint` | Interface endpoint-hours per zone and data processed; Gateway endpoints are free | - |
| `aws_ec2_transit_gateway_vpc_attachment` | Attachment-hours and data processed, billed to the attachment owner | - |
| `aws_cloudwatch_metric_alarm` | Alarm metric-months, standard or high resolution, anomaly detection | - |
| `aws_bedrock_guardrail` | Text units per configured policy (content, topics, sensitive information, contextual grounding) | ApplyGuardrail and per-policy text units per second (varies by region) |
| `bedrock_model` | Input, output, cache read and cache write tokens (Claude 4.5 models), global or regional inference | Tokens per minute (output × burndown, cache reads excluded) and requests per minute |
| `aws_bedrockagentcore_agent_runtime` | Active vCPU-hours and peak-memory GB-hours per session (platform V1 or V2), logs | Concurrent sessions, session creation rate, data-plane calls, session length |
| `aws_bedrockagentcore_harness` | Session compute at Runtime prices, logs; managed memory asks for a Memory node | Runtime quotas: concurrent sessions, session creation, data-plane calls, session length |
| `aws_bedrockagentcore_policy_engine` | Authorization requests for the tool calls of its gateways, policy generation tokens | - |
| `aws_bedrockagentcore_registry` | Records stored, search calls | Search calls per second |
| `aws_bedrockagentcore_memory` | Short-term events, long-term records stored (built-in or custom strategy), retrievals | CreateEvent and retrieval rates, extraction tokens per minute |
| `aws_bedrockagentcore_gateway` | API invocations, search, tool indexing, VPC data processing | Tool calls and search calls per second |
| `aws_bedrockagentcore_code_interpreter` / `aws_bedrockagentcore_browser` | Session vCPU-hours and memory GB-hours | Concurrent sessions, session starts, invocations, session length |
| `aws_bedrockagentcore_workload_identity` | Credential requests, free through Runtime or Gateway | Token requests per second |
| `aws_bedrockagentcore_evaluator` / `aws_bedrockagentcore_online_evaluation_config` | Custom evaluations / sampled built-in evaluator tokens (on demand or batch) | Evaluation tokens and evaluations per minute |
| `agentcore_web_search` / `agentcore_knowledge_base` | Queries / retrievals and storage (external, placed by hand) | Query rate |
| `entry` | Nothing; checks that load is set | - |

`archgopher catalog` prints every scouter with its fields as JSON.

### Adding a resource

Every resource is a directory in [`catalog/aws`](catalog/aws), named after its
type. It holds everything about that resource and nothing else; adding one
needs no Go code.

```text
catalog/aws/aws_sqs_queue/
  resource.yaml      what it accepts, how load becomes readings, Terraform rules, IAM actions
  books/prices.json  its prices, with the Price List filters that verify them
  books/quotas.json  its quotas
  books/slas.json    its SLA
  cases.yaml         worked examples: these values and this load read these costs
```

A definition composes facets (the L2 readings in [`facet`](facet)). Numbers
are expressions; text can embed `{expressions}`. Expressions are checked when
the catalog loads, so a typo or a type mismatch never reaches a user.

```yaml
type: aws_sqs_queue
kinds: [send]
attributes:
  - { key: fifo_queue, label: FIFO, type: boolean, default: false }
assumptions:
  - { key: messageKb, label: Message size, type: number, unit: KB, default: 1 }
readings:
  - requests:
      name: Requests
      price: 'aws.sqs.{fifo_queue ? "fifo" : "standard"}.requests'
      count: total.monthly * 3
      chunkKb: 64
      sizeKb: messageKb
  - rate: { when: fifo_queue, name: FIFO send rate, unit: messages/second, quota: aws.sqs.fifo.tps, peak: total.peak }
iam:
  send: ["sqs:SendMessage"]
```

| Reading | What it records |
| --- | --- |
| `requests` | Requests, optionally billed in size chunks |
| `compute` | GB-seconds, optionally in memory steps |
| `storage` / `capacity` | GB-months / units provisioned all month |
| `rate` | Peak rate against a quota, optionally scaled |
| `concurrency` | Little's law: peak × duration against a quota or a set capacity |
| `logs` / `tokens` | Log ingestion and retention / model tokens with cache and burndown |
| `session` | Session compute: active vCPU-hours, peak-memory GB-hours, concurrent sessions |
| `cost` / `limit` | Any quantity × price / any demand against a quota or capacity |
| `fail` | A problem with the declaration; stops the node unless `continue: true` |

Expressions see every attribute and assumption by key (optional ones are nil
when unset), `total.monthly` and `total.peak`, `demand.<kind>.monthly` and
`.peak`, `region`, earlier `let` values, and `ceilDiv(a, b)`. `includes: [logs]` adds a
facet's own assumption fields. A directory without `resource.yaml` holds rows
several resources share, such as log prices.

`go test ./provider/aws` loads the catalog, runs every resource's cases and
checks that every row belongs to one directory.

## Architecture

Packages are cut by the reason they change, and imports point one way, toward
what changes least. A test in [`internal/layers`](internal/layers) fails when
an import breaks the rule, and `web/scripts/layers.mjs` does the same for the UI.

```text
cmd/archgopher, cmd/wasm          edges of the system
        │
       api                          JSON in, JSON out; what the browser calls
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
| Vocabulary | [`model`](model), [`field`](field), [`book`](book) | The declaration, tag-derived schemas, reference books |
| L1 | [`meter`](meter) | The smallest readings: a cost is quantity × price id, a limit is peak demand ÷ quota id, units checked |
| L2 | [`facet`](facet) | Reusable readings with their own assumption structs: requests in size chunks, GB-seconds, storage, provisioned capacity, Little's law concurrency, logs, tokens |
| Scouter | [`scouter`](scouter) | How one resource type is read: catalog entry, fields, and a function built from facets |
| Engine | [`engine`](engine) | Validation, load propagation in topological order, path composition. No provider knowledge |
| L3 | [`pattern`](pattern) | Reusable architectures that expand into nodes and edges, then roll up |
| Terraform | [`terraform/config`](terraform/config), [`eval`](terraform/eval), [`infer`](terraform/infer), [`merge`](terraform/merge) | Parse, evaluate, infer a graph, merge into edits. No provider knowledge |
| Resources | [`definition`](definition), [`catalog/aws`](catalog/aws) | Resources as data, one directory per type: a definition compiles into a scouter built from facets |
| AWS | [`provider/aws`](provider/aws) | Loads the catalog and adds IAM edges, the schedule syntax, account-wide Terraform rules and the L3 patterns |

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
- Free tiers are ignored. Tiered prices use one tier per row (the first,
  unless the row says otherwise).
- Auto scaling, caching behaviour and retries are not modelled. Express them
  through `perUnit` and assumptions.
- AWS only, for now. The engine does not know about AWS; another provider
  would be another package like `aws`.

## License

[Apache License 2.0](LICENSE)
