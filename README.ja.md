# arch-scouter

作る前に構成を読むためのツールです。Terraform を資源のグラフにし、想定する負荷を入口から流して、
ノードごとに4つの次元を読みます。

| 次元 | 出るもの | 経路での合成 |
| --- | --- | --- |
| 金額 | 月額（USD）と、その内訳の構成要素 | 全ノードの合計 |
| 余裕 | ピーク需要とサービスの上限・設定した容量の比 | ノードごと |
| 応答 | 1往復あたりの p50 と p99（前提として置く） | 経路上の合計（p99 は上界） |
| 可用性 | サービスの SLA | 経路上の積 |

[Infracost](https://github.com/infracost/infracost) のようなコスト計算は資源を1つずつ値付けします。
arch-scouter が答えるのは別の問いです。「入口に月3,000万件が来たとき、後ろの資源はそれぞれ何件を受け、
いくらかかり、どこが最初に上限に当たるか」。

[English README](README.md)

## 使い方

```sh
go install github.com/O6lvl4/arch-scouter/cmd/arch-scouter@latest

arch-scouter tf ./infra -o app.scouter.yaml                    # Terraform から宣言を作る。init・plan・認証は不要
$EDITOR app.scouter.yaml                                        # 入口の負荷と、null のまま残った前提を埋める
arch-scouter scout app.scouter.yaml                             # Markdown の表で読む（--json で機械可読）
arch-scouter tf ./infra --merge app.scouter.yaml -o app.scouter.yaml   # Terraform の変更を合流させる
```

合流の継ぎ目は Terraform のアドレスです。Terraform が持つのは型と属性だけで、
ID・前提・負荷・メモ・座標・辺は人が書いたものが残ります。Terraform から消えたノードは削除せず、
古いものとして印を付けて報告します。

例は [`examples/serverless-api`](examples/serverless-api) にあります。架空のメモアプリで、CloudFront・
API Gateway・ローカル module の Lambda・DynamoDB・SQS・S3・1時間ごとの掃除ジョブと、手で足した
Bedrock のモデルを持ちます。

## 画面

同じエンジンが WebAssembly としてブラウザでも動きます。Terraform のフォルダを読み込み（ブラウザの中で読むだけで送信しない）、
ノードを置いて繋ぎ、前提を埋めると、編集のたびにグラフ全体を読み直します。宣言は CLI と同じ YAML で開いて保存できます。
画面は式を持たず、入力欄はエンジンが返すスカウターの項目定義から組み立てます。

```sh
cd web && pnpm install && pnpm run dev   # エンジンを WebAssembly にビルドして http://localhost:5176 で開く
```

## 判断

| 論点 | 決めたこと | 理由 |
| --- | --- | --- |
| Terraform の読み方 | HCL を静的に評価する（hashicorp/hcl と go-cty） | init・state・認証なしで PR の段階から回る。apply 後にしか決まらない値（ARN など）は未確定のまま残し、辺は参照そのものから出す |
| 辺の推定 | ノード間の参照、仲介資源（API の統合・イベントソース・購読・ターゲット・S3 通知）、ロールに付いた IAM 方針 | IAM の許可アクションから読み書きの種類が決まる。`dynamic "statement"` を module 変数から展開する書き方も要素ごとに辿る |
| 前提の欠け | そのノードだけを誤りにし、負荷は下流へ流し続ける | 既定値で黙らない。欠けた前提は宣言に null として並ぶ |
| 未知のキー | 誤りにする | `durationMS` のような打ち間違いを黙って無視しない |
| 単位 | 読み値の単位と参照表の単位が違えば誤り | 「100万件あたり」や GB と GB-月の取り違えを数値の誤りにしない |
| 単価 | AWS の公開 Price List から取り込み、確認済みの印を付ける | 認証不要。週1回 CI が読み直し、値が変われば PR を開く |
| 未確認の値 | 報告の末尾に必ず並べる | 確かめていない数字を確かなものとして出さない |
| 上限の値 | 公開されている既定値を置き、アカウントで変わるものは備考に書く | AWS が公開していない値は未知のまま置き、需要だけを出す。AgentCore は SLA が未公開なので可用性も未知 |
| CloudFront への参照 | 呼び出しではなく言及として扱い、辺にしない | コールバック URL や招待メールのリンクは、内側から CloudFront を呼ぶことではない |
| count が0の資源 | 今の変数では無効な資源として警告に並べる | 黙って消すと、なぜ図に無いのかが分からない。`--var` で含められる |

## 構成

パッケージは「変わる理由」ごとに切り、依存は変わりにくい方へ一方向にする。`internal/layers` のテストが
Go の import を、`web/scripts/layers.mjs` が画面の import を検査し、逆向きの依存が入ると落ちる。

| 層 | パッケージ | 持つもの |
| --- | --- | --- |
| 語彙 | `model` `field` `book` | 宣言、タグから導く項目定義、参照表 |
| L1 | `meter` | 最小の読み値。金額は数量 × 単価 ID、上限は需要 ÷ 上限 ID。単位を照合する |
| L2 | `facet` | 使い回す読み値。サイズ刻みのリクエスト課金、GB 秒、保管量、プロビジョンド容量、同時実行、ログ、トークン、セッション（AgentCore の CPU 時間とメモリ時間）。前提の構造体を埋め込みで渡す |
| スカウター | `scouter` | 資源型1つの読み方。面を組み合わせて書く |
| 計算 | `engine` | 検証、負荷の伝播、経路の合成。クラウドの知識を持たない |
| L3 | `pattern` | 構成のひな型。1ノードとして置き、部分グラフに展開して読み、まとめ直す |
| Terraform | `terraform/config` `eval` `infer` `merge` | 構文の読み込み、静的評価、グラフの推定、合流。クラウドの知識を持たない |
| リソース | `definition` `catalog/aws` | リソースをデータとして1型1ディレクトリで持つ。定義は面を組み合わせたスカウターに組み上がる |
| AWS | `provider/aws` | カタログを読み込み、IAM の辺・スケジュールの書式・アカウント全体の規則・L3 のひな型を足す |

画面も同じ規則で、`lib` ← `ui` ← `composites` ← `features` ← `app` の向きにだけ依存し、機能どうしは互いを参照しない。

## リソースの追加

リソースは [`catalog/aws`](catalog/aws) の下に、資源型の名前のディレクトリとして1つずつ置く。
Go のコードは書かない。

| ファイル | 持つもの |
| --- | --- |
| `resource.yaml` | 受け付ける属性と前提、負荷を読み値に変える面の組み合わせ、Terraform の規則、IAM アクション |
| `books/prices.json` | 単価と、Price List で確かめるための取り込み条件 |
| `books/quotas.json` `books/slas.json` | 上限と SLA |
| `cases.yaml` | 入力と期待値の組。この前提とこの負荷なら、この金額と需要になる |

数値は式で、文字列には `{式}` を埋め込める。式はカタログを読み込んだ時点で型まで検査するので、
打ち間違いや型の食い違いが利用者の手元に届かない。`resource.yaml` を持たないディレクトリは、
複数のリソースが読む行（ログの単価など）を共有する。書き方の全体は [README.md](README.md#adding-a-resource) を見てください。

## ライセンス

[Apache License 2.0](LICENSE)
