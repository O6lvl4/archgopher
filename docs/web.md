# Web UI

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
[web/src/ui/icons/NOTICE.md](../web/src/ui/icons/NOTICE.md) for their sources and
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
