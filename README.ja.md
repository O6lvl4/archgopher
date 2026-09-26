<h1 align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="assets/logo-dark.png">
    <img src="assets/logo.png" alt="archgopher" width="280">
  </picture>
</h1>

作る前に構成を読むためのツールです。Terraform（AWS・Azure・Google Cloud・Cloudflare・ConoHa VPS）を
グラフにし、想定する負荷を入口から流して、ノードごとに**金額**・**余裕**・**応答**・**可用性**を読みます。

「入口に月3,000万件が来たとき、後ろの資源はそれぞれ何件を受け、いくらかかり、どこが最初に上限に当たるか」。

[English](README.md)

## 使い方

```sh
go install github.com/O6lvl4/archgopher/cmd/archgopher@latest

archgopher tf ./infra -o app.scouter.yaml   # Terraform から宣言を作る（init・認証は不要）
archgopher gaps app.scouter.yaml            # Terraform から分からないものを並べ、埋める
archgopher scout app.scouter.yaml           # 読む（--json で機械可読）
```

例は [`examples/serverless-api`](examples/serverless-api) にあります。

プルリクエストでは `uses: O6lvl4/archgopher@main` がコストと余裕の差分をコメントします。
同じエンジンはブラウザでも動きます（`cd web && pnpm install && pnpm run dev`）。

## ドキュメント（英語）

- [宣言](docs/declaration.md): ノード・辺・負荷・分からないものの埋め方
- [Terraform とプルリクエスト](docs/terraform.md): 取り込み・合流・差分・GitHub Action
- [参照表](docs/books.md): 単価・上限・SLA・リージョン・`sync`
- [スカウター](docs/scouters.md): 対応する資源の一覧と追加方法
- [画面](docs/web.md)
- [構成と限界](docs/architecture.md)
- [設計判断](docs/decisions.ja.md)（日本語）

## ライセンス

[Apache License 2.0](LICENSE)。ロゴは [Renée French](https://reneefrench.blogspot.com/) による Go gopher
（[CC BY 4.0](https://creativecommons.org/licenses/by/4.0/)）をもとにしています。
サービスアイコンの権利は各社にあります（[NOTICE](web/src/ui/icons/NOTICE.md)）。
