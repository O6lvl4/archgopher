# Terraform and pull requests

## Terraform import

`archgopher tf` evaluates HCL statically with
[hashicorp/hcl](https://github.com/hashicorp/hcl) and
[go-cty](https://github.com/zclconf/go-cty). It runs without `terraform init`,
without state and without cloud credentials, so it also works on a pull request.

- **Values.** Variables (defaults, `terraform.tfvars`, `*.auto.tfvars`,
  `--var-file`, `--var`), locals, module inputs and outputs, `count`,
  `for_each`, conditionals and the common built-in functions are evaluated.
  A resource whose `count` is 0 disappears, and so does a module. Otherwise
  the counts become the node's `instances`, multiplied through the modules
  around it; `tf --merge` keeps a count you wrote where Terraform cannot know
  one before apply.
- **Unknowns.** Values that exist only after apply (ARNs, IDs) stay unknown.
  Edges do not need them: they come from references.
- **Edges.** A node that references another node calls it (a Lambda whose
  environment names a table). Helper resources connect nodes (API Gateway
  integrations, event source mappings, EventBridge targets, S3
  notifications). A helper that bills on its own sits on the path instead:
  an SNS subscription is a node between its topic and its endpoint, and a resource
  that sits between two others (an EventBridge pipe, a Firehose stream or Flink
  SQL application reading Kinesis) is fed by its source and calls its target. IAM policies attached to a node's role add edges with
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
- **Forwarding.** A call to some nodes also reaches the node they name
  (`forward:` in `resource.yaml`): an app that names a Cosmos DB container
  calls the container, which checks its own throughput, and the account,
  which bills serverless request units.
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
59 resources read, 109 free, 1 without a price yet: aws_appsync_graphql_api
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
