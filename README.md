<h1 align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="assets/logo-dark.png">
    <img src="assets/logo.png" alt="archgopher" width="280">
  </picture>
</h1>

Read an architecture before you build it. archgopher turns Terraform (AWS,
Azure, Google Cloud, Cloudflare and ConoHa VPS) into a graph, pushes your expected load
through it, and reads every node for **cost**, **headroom**, **latency** and
**availability**.

*If 30 million requests a month arrive at the front door, what does every
resource behind it see, what does it cost, and where does it run out of room
first?*

[日本語](README.ja.md)

## Quick start

```sh
go install github.com/O6lvl4/archgopher/cmd/archgopher@latest

archgopher tf ./infra -o app.scouter.yaml   # Terraform → declaration (no init, no credentials)
archgopher gaps app.scouter.yaml            # what Terraform cannot know; fill it in
archgopher scout app.scouter.yaml           # read it (--json for machines)
```

```text
| Node         | Type          | Monthly  | Tightest headroom | p99      | SLA     |
| api_handler  | Lambda        | $12.00   | 99.4%             | 400 ms   | 99.950% |
| notes        | DynamoDB      | $10.14   | 99.9%             | -        | 99.990% |
| summarizer   | Bedrock model | $1452.00 | 93.4%             | 3,000 ms | 99.900% |
```

A worked example: [`examples/serverless-api`](examples/serverless-api).

On pull requests, `uses: O6lvl4/archgopher@main` comments the cost and
headroom diff. The same engine also runs in the browser: `cd web && pnpm
install && pnpm run dev`.

## Docs

- [The declaration](docs/declaration.md): nodes, edges, load, filling the gaps
- [Terraform and pull requests](docs/terraform.md): import, merge, diff, GitHub Action
- [Reference books](docs/books.md): prices, quotas, SLAs, regions, `sync`
- [Scouters](docs/scouters.md): every supported resource, and adding one
- [Web UI](docs/web.md)
- [Architecture and limits](docs/architecture.md)

## License

[Apache License 2.0](LICENSE). The logo is derived from the Go gopher by
[Renée French](https://reneefrench.blogspot.com/) ([CC BY 4.0](https://creativecommons.org/licenses/by/4.0/)).
Service icons belong to their owners; see [their notice](web/src/ui/icons/NOTICE.md).
