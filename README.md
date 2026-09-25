<h1 align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="assets/logo-dark.png">
    <img src="assets/logo.png" alt="archgopher" width="280">
  </picture>
</h1>

Read an architecture before you build it. archgopher turns Terraform (AWS, Azure,
Google Cloud and Cloudflare) into a graph of resources, pushes your expected load through it, and reads every node
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

# 2. List what Terraform cannot know, then fill it in: the load at the entry,
#    calls made outside Terraform, call ratios, numbers left as null.
archgopher gaps app.scouter.yaml           # --json for an agent's worklist
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

Each card shows its service's icon, named by `icon` in `resource.yaml`, and a
line in its provider's color: AWS, Azure, Google Cloud's four colors or
Cloudflare. The icons come from each provider's architecture icon set; see
[web/src/ui/icons/NOTICE.md](web/src/ui/icons/NOTICE.md) for their sources and
terms.

Frames are drawn from the catalog too: pick VPC (or VNet, VPC network) under
Network and an empty frame appears. Drop a card inside it to put the node in
the VPC, drag it out to take it out; the frame fits around its cards. Its
label drags it with its cards, clicking the label opens its zones and the
traffic it reads, and Delete removes it and keeps its cards.

Cards stay where you put them, and line up while you move them: an edge or
center that comes near another card's snaps to it and shows a guide line,
otherwise the card snaps to a 16 px grid, and a card dropped on another steps
aside. A declaration without positions (from Terraform or an example) is laid
out left to right on load; the button under the zoom controls lays everything
out again.

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
| `groups` / `group` | Boundaries drawn around nodes, such as a VPC (`{ id, kind, label, type, assumptions }`), and the one a node sits in. A node reads the same inside a group as outside; the group reads the traffic between its nodes. |
| `kb` | The size of one operation, the data it moves both ways. The target counts its billing units by it: a 25 KB DynamoDB read is seven 4 KB units, a 100 KB SQS message two 64 KB chunks. Between two nodes of one group, the group reads it too: `aws_vpc` charges the share that crosses Availability Zones, `(zones - 1) / zones`, out of one zone and into the other; a Google Cloud network charges the sender; an Azure VNet charges nothing. |
| `ops` | Several kinds of work per upstream unit, each with its own `kind`, `perUnit` and `kb`. `kind`, `perUnit` and `kb` on the edge are the one-operation short form. |

### Saying how much load

`load` is the number the engine uses: a month's volume and the peak per
second. `traffic` says the same the way people think of it, and the engine
works out the load and shows the arithmetic next to it (in the report's
"Load in" table and on the entry in the UI). A node has one or the other.

| Shape | Example | Becomes |
| --- | --- | --- |
| `rate` | `{ count: 20000, per: day }` | per `second`, `minute` or `hour` is the rate while active; per `day`, `week` or `month` is a total |
| `users` | `{ count: 5000, actions: 20, per: day }` | users × actions × the active days (or weeks, or 1 for a month) |
| `concurrent` | `{ users: 50, everySeconds: 30 }` | a closed model: 50 / 30 = 1.67 per second while active, never more |
| `schedule` | `rate(1 hour)`, `cron(0 2 * * ? *)`, `*/15 9-17 * * 1-5`, `0 */5 * * * *` | EventBridge, Unix (Cloud Scheduler) and Azure cron; the peak is one fire per shortest gap |
| `batch` | `{ items: 10000, every: rate(6 hours), withinSeconds: 600 }` | items × runs; the peak is items over the time a run takes |

For `rate`, `users` and `concurrent`, `hours` (`"9-18"`, `"9-12,13-18"`,
`"22-6"`) and `days` (`all`, `weekdays`, `weekends`) say when the traffic
comes: the same volume in fewer hours peaks higher. The peak is the average
while active times `peakFactor`, 2 for totals and users and 1 for rates and
concurrent users, unless `peakPerSecond` sets it.

```yaml
- id: staff
  type: entry
  traffic:
    users: { count: 8000, actions: 30, per: day }
    hours: "9-18"
    days: weekdays
# 8,000 users × 30 a day × 21.7 weekdays = 5,214,286 a month; 7.41/s on
# average over 9 h a day on weekdays, ×2 for peaks = 14.8/s
```

### One call, several operations

A call often does more than one thing to the same resource, each of its own
size. `ops` lists them, and the target counts each operation's billing units
by its size: operations with no `kb` take the size the target assumes (a
table's `itemSizeKb`, a queue's `messageKb`).

```yaml
- from: api
  to: notes
  ops:
    - { kind: read, kb: 25 }                 # a Query returning 25 KB: 7 read units
    - { kind: read, perUnit: 2, kb: 1 }      # two GetItems of 1 KB
    - { kind: write, perUnit: 0.1, kb: 2 }   # one call in ten writes 2 KB
```

Terraform import folds the kinds a role's permissions allow between two
nodes (DynamoDB reads and writes) into one edge with an operation for each.

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
  Schedules (EventBridge rules and schedules, Cloud Scheduler jobs) become
  the node's `traffic` as written, so a changed schedule follows on merge.
- **Boundaries.** A node that references a security group, subnet or subnet
  group leading to a VPC (an `aws_vpc`, a VNet, a Google Cloud network, managed
  or looked up with a data source) sits in that VPC's frame. Only placement
  attributes (`vpc_config`, `subnet_ids`, `network_configuration`, ...) are
  followed from the node, and never through another node, so a function that
  calls a database is not placed in the database's VPC. Modules that each look
  up the same VPC share one frame. Subnets and Availability Zones are not
  frames: resources usually span several.
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

Resource types that should be nodes but have no scouter yet still become
nodes: they pass load through and appear in the report as skipped.

Every import ends with its coverage, the way Infracost counts resources:

```text
43 resources read, 108 free, 17 without a price yet: aws_acm_certificate, aws_cloudwatch_log_group ×8, ...
```

Free resources cost nothing by themselves (roles, policies, rules,
associations) or only connect or place nodes (integrations, networks drawn as
frames). The list of free types per provider is `catalog/<provider>/free.txt`,
taken from Infracost's (Apache License 2.0); they never become nodes. The web
UI shows the same line under Problems after an import.

## Pull requests

`archgopher diff <before> <after>` reads two declarations, or two Terraform
directories folded into `--spec`, and writes what changed: monthly cost per
node and per cost line, tightest headroom, path p99 and availability, with
alerts for a node that goes over or under 20% of its capacity at peak or stops
reading. `--json` gives the same for machines.

```sh
archgopher diff --spec app.scouter.yaml ../main/infra ./infra
```

The repository is also a GitHub Action that posts the diff on a pull request
and keeps one comment up to date:

```yaml
on: pull_request
permissions:
  contents: read
  pull-requests: write
jobs:
  archgopher:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: O6lvl4/archgopher@main
        with:
          terraform-dir: infra
          declaration: infra/app.scouter.yaml   # each side uses its own version
```

It builds archgopher from the action's source, checks out the base commit
next to the head, builds a declaration on each side (`tf --merge`), and writes
the diff to the comment and the job summary. Outputs `before-usd`,
`after-usd` and `delta-usd` let a later step fail a pull request over a
budget. No cloud credentials are needed.

## Filling the gaps

Terraform says what exists and what may call what, not how much is called or
what calls from outside it. `archgopher gaps` lists those unknowns:

| Kind | Listed when |
| --- | --- |
| `load` | An entry has no load or traffic |
| `caller` | A node's readings grow with the work it receives, but no edge feeds it: calls from application code, roles made elsewhere, other accounts, or the platform itself (an encryption key used by storage). Found by reading the node once idle and once busy, so alarms and other fixed-price nodes are not listed |
| `assumption` | A required number is null or missing |
| `ratio` | An edge has neither `perUnit`, `ops` nor a `note`. A note saying why one call per unit holds closes it |
| `failed` | The node could not be read for another reason |

Filling them means reading the application code and the cloud's own
measurements, which is work for an agent. [`skills/archgopher-gaps`](skills/archgopher-gaps/SKILL.md)
is the procedure as a Claude Code skill (copy it into `.claude/skills/`). It
holds for every provider: counts per resource, the front door's access log and
billed usage quantities exist everywhere under different names, and traces are
used when they exist. Provenance goes in `note`, which `tf --merge` keeps.

## Reference books

Prices, quotas and SLAs live next to each resource in `catalog/<provider>/<type>/books`, one row per ID with
a value per region, a unit, a source URL and a `verified` flag.

- **Units are checked.** A reading that counts `GB` against a price per
  `GB-month` is an error, not a wrong number.
- **Unverified values are listed.** Any value nobody has checked, or that is
  unknown, appears at the end of every report.
- **Prices sync from the public price lists.** `archgopher sync` reads the
  AWS Price List bulk files and the Azure Retail Prices API (neither needs
  credentials) and the Google Cloud Billing Catalog, and marks each price
  verified. The Billing Catalog needs credentials: set `ARCHGOPHER_GCP_TOKEN`,
  `ARCHGOPHER_GCP_ACCOUNT` (a gcloud account to take a token from) or
  `ARCHGOPHER_GCP_API_KEY`. Without them, Google Cloud rows are skipped, not
  failed, and keep their values.
  A weekly workflow opens a pull request when a price changes.
  `archgopher explore <service> <region> [attr=regex...]` helps you write the
  filters for a new price.

```sh
archgopher sync --check      # exit 1 if the book is out of date
archgopher sync --add-regions eu-west-2   # add a region to every price
archgopher explore AWSLambda ap-northeast-1 'usagetype=.*GB-Second.*'
archgopher explore azure Functions japaneast 'meterName=Standard.*'
ARCHGOPHER_GCP_ACCOUNT=you@example.com archgopher explore gcp 152E-C115-5142 asia-northeast1
```

**Regions.** Ten AWS regions are covered: us-east-1, us-east-2, us-west-2,
eu-west-1, eu-central-1, ap-northeast-1, ap-northeast-2, ap-southeast-1,
ap-southeast-2 and ap-south-1. Ten Azure regions match them: eastus, eastus2,
westus2, northeurope, germanywestcentral, japaneast, koreacentral,
southeastasia, australiaeast and centralindia. Ten Google Cloud regions match
them too: us-east4, us-east5, us-west1, europe-west1, europe-west3,
asia-northeast1, asia-northeast3, asia-southeast1, australia-southeast1 and
asia-south1. A row that is the same everywhere is `*`; a
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
| `aws_rds_cluster` | Aurora Serverless v2 ACU-hours (provisioned instances are their own nodes), storage, I/O (Standard or I/O-Optimized), backup storage, Backtrack, snapshot export, the managed master password secret | Peak ACU against max capacity |
| `aws_rds_cluster_instance` | Provisioned Aurora instance-hours (Standard or I/O-Optimized), surplus CPU credits, Database Insights and Extended Support per vCPU | - |
| `aws_db_instance` | Instance-hours by engine, license and Single-AZ or Multi-AZ, storage (gp2, gp3, io1, io2, magnetic), IOPS and gp3 throughput above the baseline, magnetic I/O, backups beyond the free allowance, CPU credits, Database Insights, Extended Support, the managed master password secret | Storage IOPS against the volume |
| `aws_docdb_cluster` / `aws_neptune_cluster` | Storage (standard or I/O-Optimized), I/O, backups beyond the free allowance | Storage against the cluster limit |
| `aws_docdb_cluster_instance` / `aws_neptune_cluster_instance` | Instance-hours (standard or I/O-Optimized), serverless DCU or NCU-hours, CPU credits | - |
| `aws_docdb_cluster_snapshot` / `aws_neptune_cluster_snapshot` | Snapshot storage beyond the free allowance | - |
| `aws_elasticache_cluster` / `aws_elasticache_replication_group` | Node-hours by engine (Redis OSS, Valkey, Memcached) and node count, retained snapshots | Data set against node memory × shards |
| `aws_redshift_cluster` | Node-hours (doubled on Multi-AZ), managed storage, concurrency scaling, Spectrum, backups in three tiers | Stored data against the nodes' storage |
| `aws_dms_replication_instance` | Instance-hours (Single-AZ or Multi-AZ), storage above what the class includes | - |
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
| `aws_ec2_transit_gateway_vpc_attachment` | Attachment-hours and data processed, billed to the attachment owner | Bandwidth per zone |
| `aws_vpn_connection` | Connection-hours (standard or large tunnels), data sent out | Bandwidth per tunnel |
| `aws_nat_gateway` | Gateway-hours and data processed | Bandwidth |
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
| `aws_codebuild_project` | Build minutes by environment and compute type, Lambda compute seconds, remote Docker server seconds | Concurrent builds per environment and compute type, build time against the timeout |
| `aws_cloudformation_stack` / `aws_cloudformation_stack_set` | Handler operations and handler time beyond 30 s for third-party and private registry types, per stack instance | - |
| `aws_cloudhsm_v2_hsm` | HSM-hours (hsm1.medium or hsm2m.medium) | RSA 2048 signatures per second per HSM (published guidance) |
| `aws_directory_service_directory` | Directory-hours (Simple AD, AD Connector), domain controller-hours and sharing (Microsoft AD) | - |
| `aws_grafana_workspace` | Editor, viewer and Enterprise plugins licenses | - |
| `aws_kms_external_key` | Key-months, requests | Cryptographic requests per second (varies by region) |
| `aws_ssm_activation` | Session Manager sessions and Run Command invocations on hybrid nodes | - |
| `entry` | Nothing; checks that load is set | - |

Zone crossings are read by the VPC, not by a node. Set `kb` on an edge between
two nodes inside it (a function and its database) and `zones` on the VPC: with
the nodes spread evenly, two in three calls over three zones cross. Leave `kb`
off edges to regional services such as S3 and DynamoDB, which do not cross
zones. A `kb` on an edge that leaves the group is reported, not read. See
[`examples/private-network`](examples/private-network).

Network resources sit on the path. Terraform cannot tell which calls go
through a transit gateway, VPN, NAT gateway or endpoint, so draw the edge
through the node by hand: load that reaches it carries `kbPerUnit` each, turns
into GB and bandwidth, and flows on to the next node, which then counts the
hop in the path's latency and availability. `gbPerMonth` adds traffic that is
not drawn as load. A node that load reaches without `kbPerUnit` is an error,
not zero.

### Azure

| Type | Reads | Headroom |
| --- | --- | --- |
| `azurerm_linux_function_app` / `azurerm_windows_function_app` | Consumption executions and GB-seconds (other plans are priced on the plan) | Instances and timeout |
| `azurerm_function_app_flex_consumption` | On-demand executions and GB-seconds, always-ready baseline | Instances per function group |
| `azurerm_service_plan` | Instance-hours by SKU and OS, times workers | - |
| `azurerm_container_app` | Consumption vCPU- and GiB-seconds, active and idle, requests | Replicas |
| `azurerm_storage_account` | Hot tier storage, write and read operations by redundancy, transfer out to the internet | Account request rate (varies by region) |
| `azurerm_cosmosdb_account` | Serverless request units or provisioned RU/s-hours, storage, per region | Provisioned RU/s |
| `azurerm_postgresql_flexible_server` | Compute by SKU (doubled with high availability), storage | Connections by SKU |
| `azurerm_mssql_database` / `azurerm_sql_database` | vCore hours (serverless: vCore-hours used), SQL license, zone redundancy, Hyperscale replicas, storage; or DTU objective per day and extra storage; backups. In a pool, backups only | Concurrent workers |
| `azurerm_mssql_elasticpool` / `azurerm_sql_elasticpool` | eDTUs per day and extra storage, or vCore hours, SQL license, zone redundancy, storage | - |
| `azurerm_mssql_managed_instance` / `azurerm_sql_managed_instance` | vCore hours, SQL license, zone redundancy, storage above 32 GB, backups | Concurrent workers |
| `azurerm_mysql_flexible_server` | Compute by SKU (doubled with high availability), storage, IOPS above 360, backups | Connections by SKU |
| `azurerm_mysql_server` / `azurerm_postgresql_server` / `azurerm_mariadb_server` | Single servers (retired): vCore hours by tier, storage, backups | Connections by SKU |
| `azurerm_api_management` | Consumption calls, or unit-hours by tier with included calls | Requests per unit (published guidance) |
| `azurerm_cdn_frontdoor_profile` | Base fee, requests and transfer out by the viewers' zone | - |
| `azurerm_servicebus_namespace` | Operations (Basic, Standard with its base fee), or Premium messaging units | Operations per second |
| `azurerm_eventgrid_topic` | Operations | Events per second |
| `azurerm_key_vault` | Secret operations | Requests per vault |
| `azurerm_key_vault_key` | Operations (RSA 2048 or advanced), HSM key-months per version, automatic rotations | Vault rate for the key type and protection |
| `azurerm_key_vault_certificate` | Renewals read from the policy, operations | Requests per vault |
| `azurerm_key_vault_managed_hardware_security_module` | HSM pool hours (keys and operations included) | RSA 2048 unwrap throughput |
| `azurerm_cognitive_deployment` | Azure OpenAI input, cached and output tokens by model and deployment type | Tokens and requests per minute from capacity or quota |
| `azurerm_linux_virtual_machine` / `azurerm_windows_virtual_machine` / `azurerm_virtual_machine` | Instance hours by size (Linux, Windows, or the base rate with Hybrid Benefit), OS disk tier and operations, inline data disks (legacy), Ultra Disk reservation | - |
| `azurerm_linux_virtual_machine_scale_set` / `azurerm_windows_virtual_machine_scale_set` / `azurerm_virtual_machine_scale_set` | Instance hours by size times instances, their OS and data disks | - |
| `azurerm_managed_disk` | Standard HDD, Standard SSD and Premium SSD by tier with operations; Ultra and Premium SSD v2 capacity, IOPS and throughput | IOPS against the tier or the provisioned IOPS |
| `azurerm_snapshot` / `azurerm_image` | Data held, at the snapshot price | - |
| `azurerm_backup_protected_vm` / `azurerm_recovery_services_vault` | Protected instances by data size / backup storage by the vault's redundancy | - |
| `azurerm_kubernetes_cluster` / `azurerm_kubernetes_cluster_node_pool` | Standard tier uptime SLA or Premium long-term support, node hours by size and OS, managed OS disks, load balancer data, the HTTP routing DNS zone | - |
| `azurerm_lb` | Standard and cross-region: the fee covering the first five rules, data processed; gateway: gateway-, chain-hours and data processed (Basic is free) | - |
| `azurerm_lb_rule` / `azurerm_lb_outbound_rule` | Rule-hours for each rule beyond a load balancer's first five | - |
| `azurerm_application_gateway` | v2: fixed hours and capacity units, reserved or used (throughput, compute units); v1: instance-hours and data processed beyond the free allowance | Instances at peak against the autoscale maximum or manual count |
| `azurerm_public_ip` / `azurerm_public_ip_prefix` | Address-hours by SKU, tier and allocation / address-hours for every address in the prefix | - |
| `azurerm_nat_gateway` | Gateway-hours and data processed (Standard and StandardV2) | Bandwidth |
| `azurerm_bastion_host` | Host-hours by SKU, instances beyond two, data out | Concurrent RDP or SSH sessions per instance |
| `azurerm_firewall` | Deployment-hours and data processed by tier, in a VNet or a secured hub | Throughput by tier |
| `azurerm_firewall_policy` | The fee per region once two or more firewalls use it, policy analytics; rule collection groups fold in | - |
| `azurerm_private_endpoint` | Endpoint-hours and data processed | - |
| `azurerm_virtual_network_peering` | Data sent and received, within a region or global | - |
| `azurerm_sentinel_data_connector_*` (8 connectors) | Sentinel pay-as-you-go ingestion per GB, including the Log Analytics charge; free data sources (alerts, most Office 365 audit logs) read nothing | - |
| `azurerm_security_center_subscription_pricing` | The Defender for Cloud plan's unit: servers, instances, accounts (with transaction overage and malware scanning), vaults, subscriptions, vCores, images, RU/s, queries or tokens | - |
| `azurerm_active_directory_domain_service` / `_replica_set` | Hours by SKU, per replica set | Recommended authentications per hour |
| `azurerm_application_insights` | Telemetry after sampling and the daily cap, and retention beyond 90 days, at the workspace's prices when workspace-based or classic prices | Events per second |
| `azurerm_monitor_action_group` | Emails, push, ITSM events, webhooks (plain and secure), SMS and voice calls by country code, per time fired | Emails, SMS and calls per address or number |
| `azurerm_monitor_metric_alert` | Time series monitored (resources in scope times criteria or dimension values), dynamic thresholds | - |
| `azurerm_monitor_scheduled_query_rules_alert` / `azurerm_monitor_scheduled_query_rules_alert_v2` | The rule by evaluation frequency / and time series beyond the first | - |
| `azurerm_monitor_data_collection_rule` | Custom metric samples; logs flow on to the workspace | - |
| `azurerm_monitor_diagnostic_setting` | Platform logs sent to storage, an event hub or a partner | - |
| `azurerm_log_analytics_solution` | Microsoft Sentinel analysis per GB; other solutions have no price of their own | - |
| `azurerm_automation_account` | Job minutes and non-Azure configuration nodes | Job submissions and concurrent jobs |
| `azurerm_automation_job_schedule` / `azurerm_automation_watcher` | Job minutes per run / watcher hours | - |
| `azurerm_automation_dsc_configuration` / `azurerm_automation_dsc_nodeconfiguration` | Non-Azure configuration nodes | - |

Role assignments become edges: an `azurerm_role_assignment` connects the
resource whose managed identity holds the role (system- or user-assigned) to
the resource it is scoped to, with the kinds the role grants. Each resource's
`iam` lists Azure role names per kind, where AWS resources list IAM actions.

### Google Cloud

| Type | Reads | Headroom |
| --- | --- | --- |
| `google_cloud_run_v2_service` | Request-based vCPU- and GiB-seconds from the container limits and concurrency, requests | Instances against max instances |
| `google_cloudfunctions2_function` | Cloud Run prices for its CPU and memory, invocations | Instances against max instances |
| `google_cloud_scheduler_job` | Job-months | - |
| `google_cloud_tasks_queue` | Operations in 32 KB chunks | Dispatch rate against max dispatches |
| `google_storage_bucket` | Standard storage in a region, Class A and B operations, transfer out to the internet | Initial read and write rates per bucket |
| `google_firestore_database` | Document reads, writes and deletes, stored data (Native mode, Standard edition) | - |
| `google_sql_database_instance` | vCPU and memory hours or a shared-core instance, SSD, with high-availability prices | PostgreSQL connections by memory |
| `google_pubsub_topic` | Throughput | Publish throughput per region (varies by region) |
| `google_secret_manager_secret` | Active versions and access operations | Access requests per minute |
| `google_api_gateway_gateway` | Calls | Quota units per second |
| `google_compute_forwarding_rule` / `google_compute_global_forwarding_rule` | Rule-hours (the first five of a project share one minimum), data processed by passthrough rules, proxy instances for INTERNAL_MANAGED rules; Private Service Connect endpoint-hours and data processed | - |
| `google_compute_target_http_proxy` / `_https_proxy` / `google_compute_region_target_http_proxy` / `_https_proxy` | Data processed, at the external or internal Application Load Balancer price | - |
| `google_compute_target_tcp_proxy` / `google_compute_target_ssl_proxy` | Data processed by the proxy Network Load Balancer | - |
| `google_compute_target_grpc_proxy` | Cloud Service Mesh client-hours | - |
| `google_compute_router_nat` | Public NAT uptime by VM instances (capped from 33), NAT IP-hours, data processed; Private NAT uptime and data | Source ports on manual NAT addresses |
| `google_compute_vpn_tunnel` | Tunnel-hours | - |
| `google_compute_vpn_gateway` / `google_compute_ha_vpn_gateway` / `google_compute_external_vpn_gateway` | IPsec traffic sent: internet transfer out to a device outside Google Cloud (read on the External VPN gateway for HA VPN), between-zones to a gateway in the same region | - |
| `google_service_networking_connection` | Data crossing zones to a Google-managed service network | - |
| `google_dns_managed_zone` / `google_dns_record_set` | Zone-months by the account's zone count / queries, with routing policies priced higher | - |
| `gemini_model` | Gemini 2.5 input, cached and output tokens (placed by hand) | Tokens per minute |
| `google_logging_{project,folder,organization,billing_account}_sink` | Logging storage for a log bucket written out as its destination; routing elsewhere is free and billed by the destination | Log write rate per project and region |
| `google_logging_{project,folder,organization,billing_account}_bucket_config` | Logging storage of what sinks route in, retention beyond 30 days | - |
| `google_monitoring_metric_descriptor` | Metric volume in bytes ingested by tier, read API time series | One point every 5 seconds per time series, active time series |

Google Cloud prices are read from the pricing pages until a credentialed
`sync` checks them against the Billing Catalog; they are marked unverified
until then.

### Cloudflare

| Type | Reads | Headroom |
| --- | --- | --- |
| `cloudflare_workers_script` | Requests and CPU milliseconds (routes, custom domains and cron triggers fold in) | CPU time per request |
| `cloudflare_workers_paid_plan` | The Workers Paid base fee, one per account (placed by hand) | - |
| `cloudflare_durable_object` | Requests, duration of active objects, SQLite rows read and written, storage (placed by hand) | Requests per second per object |
| `workers_ai_model` | Input, cached and output tokens by model (placed by hand) | Requests per minute |
| `cloudflare_zone` | The plan's monthly fee (DNS records and settings fold in) | - |
| `cloudflare_r2_bucket` | Storage, Class A and B operations, Infrequent Access retrieval; egress is free | Writes per second to one key |
| `cloudflare_d1_database` | Rows read and written, storage | Database size; queries per second from one query at a time |
| `cloudflare_workers_kv_namespace` | Reads, writes (deletes and lists cost the same), storage | Writes per second to one key |
| `cloudflare_queue` | Operations in 64 KB chunks | Messages per second per queue, message size |

Cloudflare publishes no price API, so `sync` cannot check these prices. They
are read from the pricing pages by hand and marked verified with the date they
were read. Each price is the same everywhere (`*`), so a Cloudflare node prices
in the region of any declaration, and a declaration of Cloudflare alone needs
no region. The allowances included in the Workers Paid plan are shared across
the account and are not subtracted, so small workloads read higher than the
bill.

`archgopher catalog` prints every scouter with its fields as JSON.

### Adding a resource

Every resource is a directory in [`catalog/aws`](catalog/aws),
[`catalog/azure`](catalog/azure), [`catalog/gcp`](catalog/gcp) or
[`catalog/cloudflare`](catalog/cloudflare), named after its type. It holds
everything about that resource and nothing else; adding one needs no Go code.
Its `icon` names a picture in `web/src/ui/icons/<provider>/`; a test fails when
a resource has none or a picture is unused.

```text
catalog/aws/aws_sqs_queue/
  resource.yaml      its icon, what it accepts, how load becomes readings, Terraform rules, IAM actions
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

An attribute's `path` says where it sits in the resource block (`sku.name`
reads the first `sku` block). A path ending in `.#` counts what it names across
every block, such as `criteria.dimension.values.#` for every value of every
dimension of every criterion. A boolean read from a block or an id reads
whether it is written, even when the id is known only after apply.

Prices that differ by one attribute only (an instance type, a database
class) are a table: one entry with `rows` instead of `values`, one number per
region, `null` where the row is not offered. Row `t3.micro` of
`aws.ec2.linux` is priced as `aws.ec2.linux.t3.micro`, so a reading names it
with `'aws.ec2.linux.{instance_type}'`. `{row}` in the sync filters stands for
the row key, so one spec verifies every row.

```json
"aws.ec2.linux": {
  "unit": "instance-hour", "source": "https://aws.amazon.com/ec2/pricing/on-demand/",
  "sync": {"service": "AmazonEC2", "filters": {"instanceType": "{row}", "operatingSystem": "Linux", "tenancy": "Shared", "preInstalledSw": "NA", "capacitystatus": "Used"}},
  "rows": {"t3.micro": {"us-east-1": 0.0104, "ap-northeast-1": 0.0136}},
  "verified": true, "checkedAt": "2026-09-25"
}
```

`go test ./provider/aws` loads the catalog, runs every resource's cases and
checks that every row belongs to one directory.

## Architecture

Packages are cut by the reason they change, and imports point one way, toward
what changes least. A test in [`internal/layers`](internal/layers) fails when
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
| Vocabulary | [`model`](model), [`field`](field), [`book`](book) | The declaration, tag-derived schemas, reference books |
| L1 | [`meter`](meter) | The smallest readings: a cost is quantity × price id, a limit is peak demand ÷ quota id, units checked |
| L2 | [`facet`](facet) | Reusable readings with their own assumption structs: requests in size chunks, GB-seconds, storage, provisioned capacity, Little's law concurrency, logs, tokens |
| Scouter | [`scouter`](scouter) | How one resource type is read: catalog entry, fields, and a function built from facets |
| Engine | [`engine`](engine) | Validation, load propagation in topological order, path composition. No provider knowledge |
| Gaps | [`gaps`](gaps) | What a declaration does not know yet, found from the readings and the scouters' fields. No provider knowledge |
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
- One region per declaration. A declaration that mixes AWS, Azure and Google
  Cloud prices every node in that one region, so nodes of the other clouds
  find no price. Cloudflare prices are the same everywhere and price in any
  region.

## License

[Apache License 2.0](LICENSE)

The archgopher logo is derived from the Go gopher, designed by
[Renée French](https://reneefrench.blogspot.com/) and licensed under
[CC BY 4.0](https://creativecommons.org/licenses/by/4.0/).

The service icons in `web/src/ui/icons` belong to their owners and are not
covered by the Apache License; see [their notice](web/src/ui/icons/NOTICE.md).
