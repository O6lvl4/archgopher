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
| 上限の値 | 公開されている既定値を置き、アカウントで変わるものは備考に書く | AWS が公開していない値は未知のまま置き、需要だけを出す |
| CloudFront への参照 | 呼び出しではなく言及として扱い、辺にしない | コールバック URL や招待メールのリンクは、内側から CloudFront を呼ぶことではない |
| count が0の資源 | 今の変数では無効な資源として警告に並べる | 黙って消すと、なぜ図に無いのかが分からない。`--var` で含められる |

## スカウターの追加

スカウターは、タグ付きの構造体2つと関数1つです。タグが Go の型・検証・カタログ・YAML の契約の
唯一の出どころになります。ポインタでない項目で既定値が無いものは必須、ポインタの項目は任意です。
[`aws/aws.go`](aws/aws.go) に登録し、参照表に単価を足すと、登録簿のテストが「読む参照がすべての地域に、
数える単位のまま存在するか」を確かめます。書き方は [README.md](README.md#adding-a-scouter) を見てください。

## ライセンス

[Apache License 2.0](LICENSE)
