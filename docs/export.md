# Export

`archgopher export` draws a declaration as an architecture diagram.

```sh
archgopher export app.scouter.yaml -o app.svg
archgopher export app.scouter.yaml --format svg      # to stdout
```

The diagram is one self-contained SVG: the providers' architecture icons are
embedded, no fonts or images are fetched, and it opens in a browser, an
editor or a slide.

## What is drawn

- **Nodes** are their catalog icon, with the catalog label, the id, and under
  them the nodes folded into them (see below). A node with `instances` shows
  the count in its label.
- **Edges** are arrows from icon to icon, labelled with their `kind` and, when
  it is not one, the calls per unit (`read ×0.5`).
- **Groups** (a VPC, a VNet, a VPC network) are frames around their members.
  A frame is laid out on its own and placed as one block, so nothing that is
  not a member falls inside it.
- **The cloud and its region** frame every node that belongs to one provider
  when the declaration has only one. Entries and other sources from outside
  (`external` in the catalog, with nothing calling them) sit left of the
  frame. A declaration that mixes clouds gets no outer frame.

## Attached nodes

Some resources are configuration of another one rather than a box of their
own: a log group, an alarm, an API's stage, a topic's subscription, a bucket's
lifecycle rules, a KMS key, a cluster's instances. Their `resource.yaml` says
`attach: true`, and the diagram counts them under their owner instead of
drawing them:

```text
Lambda
api.handler
alarm ×2 · log group ×1
```

The owner is the one node the attached node points at (a stage points at its
API); otherwise the one node it is connected to (a log group its function
writes); otherwise the busiest node of its module, the id before the first
dot (`api.errors` goes under the `api.*` node with the most edges). A node
with no owner is drawn on its own. Edges through an attached node reach the
owner: `users → stage → api` is drawn as `users → api`.

## Placement

When every drawn node has a `position` (the web UI saves them), the diagram
uses those. Otherwise nodes are laid out left to right, callers before what
they call: each node's column is the longest path from a source, the rows are
ordered to keep neighbours close, and an edge that skips columns bends
through a lane in each column it crosses instead of running over the nodes
there. Nodes without edges sit beside the busiest node of their module.

## Formats

`--format svg` is the only format today. The flag is there so that others
(an HTML page with the readings, draw.io) can join without changing how the
command is called.
