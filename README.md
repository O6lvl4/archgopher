# arch-scouter

Read an architecture before you build it. arch-scouter turns Terraform into a
graph of resources, pushes your expected load through it, and reads every node
on four dimensions:

| Dimension | What you get | How paths combine |
| --- | --- | --- |
| Cost | Monthly USD, broken down into priced components | Sum over nodes |
| Headroom | Peak demand against service quotas and configured capacity | Per node |
| Latency | p50 and p99 per hop, from your assumptions | Sum along a path (p99 is an upper bound) |
| Availability | The service SLA | Product along a path |

Cost tools such as [Infracost](https://github.com/infracost/infracost) price
each resource on its own. arch-scouter asks a different question: *if 30
million requests a month arrive at the front door, what does every resource
behind it see, what does it cost, and where does it run out of room first?*

[日本語の README](README.ja.md)

## Quick start

```sh
go install github.com/O6lvl4/arch-scouter/cmd/arch-scouter@latest

# 1. Build a declaration from Terraform. No terraform init, plan or credentials.
arch-scouter tf ./infra -o app.scouter.yaml

# 2. Fill in what Terraform cannot know: the load at the entry and the
#    assumptions left as null (duration per call, item size, ...).
$EDITOR app.scouter.yaml

# 3. Read it.
arch-scouter scout app.scouter.yaml          # Markdown tables
arch-scouter scout app.scouter.yaml --json   # machine-readable

# 4. After Terraform changes, fold them in. Your assumptions, load, notes and
#    edges stay; only types and attributes follow Terraform.
arch-scouter tf ./infra --merge app.scouter.yaml -o app.scouter.yaml
```

A worked example lives in [`examples/serverless-api`](examples/serverless-api):
a fictional notes app with CloudFront, API Gateway, Lambda (in a local module),
DynamoDB, SQS, S3, an hourly cleanup job, and a Bedrock model added by hand.

```text
| Node         | Type          | Monthly  | Tightest headroom | p99      | SLA     |
| api_handler  | Lambda        | $12.00   | 99.4%             | 400 ms   | 99.950% |
| notes        | DynamoDB      | $10.14   | 99.9%             | -        | 99.990% |
| summarizer   | Bedrock model | $1597.20 | -                 | 3,000 ms | 99.900% |
...
```

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

`arch-scouter tf` evaluates HCL statically with
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

Prices, quotas and SLAs live in [`aws/books`](aws/books), one row per ID with
a value per region, a unit, a source URL and a `verified` flag.

- **Units are checked.** A reading that counts `GB` against a price per
  `GB-month` is an error, not a wrong number.
- **Unverified values are listed.** Any value nobody has checked, or that is
  unknown, appears at the end of every report.
- **Prices sync from the public Price List.** `arch-scouter sync` reads the
  AWS Price List bulk files (no credentials) and marks each price verified.
  A weekly workflow opens a pull request when a price changes.
  `arch-scouter explore <service> <region> [attr=regex...]` helps you write the
  filters for a new price.

```sh
arch-scouter sync --check      # exit 1 if the book is out of date
arch-scouter explore AWSLambda ap-northeast-1 'usagetype=.*GB-Second.*'
```

Quotas are the published defaults. Some are account-specific in practice
(Lambda concurrency on new accounts, Bedrock tokens per minute), and their
notes say so. A value AWS does not publish stays unknown: the report shows
the demand and says the capacity is unknown.

## Scouters

| Type | Reads | Headroom |
| --- | --- | --- |
| `aws_lambda_function` | Requests, GB-seconds (x86_64 / arm64), logs | Concurrency (Little's law: peak rate × duration) against the account or reserved concurrency |
| `aws_api_gateway_rest_api` | Requests | Account throttle |
| `aws_apigatewayv2_api` | Requests in 512 KB steps (HTTP APIs) | Account throttle |
| `aws_cloudfront_distribution` | HTTPS requests, transfer out | Requests per distribution |
| `aws_dynamodb_table` | On-demand request units or provisioned capacity (4 KB / 1 KB steps, consistency, transactions), storage | Table throughput or provisioned capacity |
| `aws_s3_bucket` | GET, PUT, storage (Standard) | Per-prefix request rate × prefixes |
| `aws_sqs_queue` | Requests in 64 KB chunks | FIFO send rate |
| `aws_sns_topic` | Publishes in 64 KB chunks | Publish rate |
| `aws_sfn_state_machine` | Transitions (standard), requests and GB-seconds (express) | StartExecution rate |
| `aws_rds_cluster` | Aurora Serverless v2 ACU-hours, storage, I/O (Standard or I/O-Optimized) | Peak ACU against max capacity |
| `aws_scheduler_schedule` | Invocations | - |
| `aws_cloudwatch_event_rule` | Nothing (scheduled rules are free) | - |
| `bedrock_model` | Input, output, cache read and cache write tokens (Claude 4.5 models) | Tokens per minute (output × burndown, cache reads excluded) and requests per minute |
| `entry` | Nothing; checks that load is set | - |

`arch-scouter catalog` prints every scouter with its fields as JSON.

### Adding a scouter

A scouter is two tagged structs and a function. The tags are the single
source for the Go types, validation, the catalog and the YAML contract.

```go
type queueAttrs struct {
	Fifo bool `scout:"fifo_queue" label:"FIFO" default:"false"`
}

type queueAssume struct {
	MessageKb float64 `scout:"messageKb" label:"Message size" unit:"KB" default:"1"`
}

var SQS = scout.Def[queueAttrs, queueAssume]{
	Info: scout.Meta{Type: "aws_sqs_queue", Label: "SQS", Kinds: []string{"send"}, SLA: "aws.sqs"},
	Run: func(a queueAttrs, p queueAssume, d scout.Demand, r *scout.Recorder) {
		r.Cost("Requests", d.Total().Monthly*scout.CeilDiv(p.MessageKb, 64)*3, "request", "aws.sqs.standard.requests")
	},
}
```

A non-pointer field without a default is required. A pointer field is
optional. Register the scouter in [`aws/aws.go`](aws/aws.go), add its prices
to the books, and the registry test checks that every reference it reads
exists in every region, in the unit it counts.

## Layout

| Package | Role |
| --- | --- |
| [`scout`](scout) | The engine: model, graph, load propagation, readings. No I/O, no provider knowledge. |
| [`aws`](aws) | AWS scouters, reference books, Terraform rules. |
| [`aws/pricelist`](aws/pricelist) | Reader for the public AWS Price List. |
| [`terraform`](terraform) | Static HCL evaluation, graph building, merging. |
| [`report`](report) | Markdown and JSON output. |
| [`cmd/arch-scouter`](cmd/arch-scouter) | The CLI. |

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
