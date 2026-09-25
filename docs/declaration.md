# The declaration

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

## Saying how much load

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

## One call, several operations

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
measurements, which is work for an agent. [`skills/archgopher-gaps`](../skills/archgopher-gaps/SKILL.md)
is the procedure as a Claude Code skill (copy it into `.claude/skills/`). It
holds for every provider: counts per resource, the front door's access log and
billed usage quantities exist everywhere under different names, and traces are
used when they exist. Provenance goes in `note`, which `tf --merge` keeps.
