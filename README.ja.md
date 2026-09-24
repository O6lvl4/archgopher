<h1 align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="assets/logo-dark.png">
    <img src="assets/logo.png" alt="archgopher" width="280">
  </picture>
</h1>

作る前に構成を読むためのツールです。Terraform（AWS・Azure・Google Cloud・Cloudflare）を資源のグラフにし、想定する負荷を入口から流して、
ノードごとに4つの次元を読みます。

| 次元 | 出るもの | 経路での合成 |
| --- | --- | --- |
| 金額 | 月額（USD）と、その内訳の構成要素 | 全ノードの合計 |
| 余裕 | ピーク需要とサービスの上限・設定した容量の比 | ノードごと |
| 応答 | 1往復あたりの p50 と p99（前提として置く） | 経路上の合計（p99 は上界） |
| 可用性 | サービスの SLA | 経路上の積 |

[Infracost](https://github.com/infracost/infracost) のようなコスト計算は資源を1つずつ値付けします。
archgopher が答えるのは別の問いです。「入口に月3,000万件が来たとき、後ろの資源はそれぞれ何件を受け、
いくらかかり、どこが最初に上限に当たるか」。

[English README](README.md)

## 使い方

```sh
go install github.com/O6lvl4/archgopher/cmd/archgopher@latest

archgopher tf ./infra -o app.scouter.yaml                    # Terraform から宣言を作る。init・plan・認証は不要
$EDITOR app.scouter.yaml                                        # 入口の負荷と、null のまま残った前提を埋める
archgopher scout app.scouter.yaml                             # Markdown の表で読む（--json で機械可読）
archgopher tf ./infra --merge app.scouter.yaml -o app.scouter.yaml   # Terraform の変更を合流させる
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
カードには、サービスのアイコン（`resource.yaml` の `icon`）とプロバイダの色の線（AWS・Azure・Google Cloud の4色・Cloudflare）が付きます。
アイコンは各社の構成図用アイコン集から取っており、出典と利用条件は [web/src/ui/icons/NOTICE.md](web/src/ui/icons/NOTICE.md) にあります。
枠も左の一覧から置けます。Network の下の VPC（VNet・VPC network）を選ぶと空の枠が出て、カードを枠の中に落とせばそのノードは VPC に入り、外へ出せば抜けます。
枠は見出しをつかむと中のカードごと動き、選択中は四隅で大きさを変えられます。見出しをクリックすると AZ 数と読み値が出ます。Terraform から読んだ枠はカードを囲む大きさで始まり、Tidy で囲み直します。

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
| 地域 | 10地域（us-east-1・us-east-2・us-west-2・eu-west-1・eu-central-1・ap-northeast-1・ap-northeast-2・ap-southeast-1・ap-southeast-2・ap-south-1）。全地域で同じ値は `*`、地域で違う上限は `*` の既定に地域名の例外を重ねる。地域差は全上限について各サービスの上限ページで確認した（API Gateway・SNS・SQS 高スループット・Step Functions・AgentCore Runtime と評価に差がある） | `sync --add-regions` で単価を Price List から一括で足せる。台帳の全行が全地域の値を持つことをテストで保つ |
| Azure | AWS と同じ仕組みで扱う。単価は Azure Retail Prices API から取り込み（認証不要）、地域は AWS の10地域に対応する10地域。ロール割り当て（azurerm_role_assignment）を、マネージド ID を持つ資源から対象の資源への辺にする | 資源の追加は catalog/azure にディレクトリを足すだけ。Go のコードは変えない |
| Google Cloud | 単価は Cloud Billing Catalog API から取り込む。この API だけは認証が要るので、環境変数でトークンか gcloud のアカウントか API キーを渡す。無い環境では Google Cloud の行を飛ばし、失敗にしない | CI に認証を置かなくても AWS と Azure の見直しは回る。地域は AWS の10地域に対応する10地域 |
| Cloudflare | 単価を取り込む API が無いので、料金ページから手で読み、確かめた日付（checkedAt）を付けて確認済みにする。`sync` では見直せない。単価は全地域で同じなので `*` の1行で持ち、どの地域の宣言でも同じ額になる。Workers 有料プランの月額はアカウントに1つのノードにし、プランに含まれる量は差し引かない | 含まれる量はアカウント全体で共有されるので、資源ごとに引くと二重に引く。少量の利用では実際より高く出る。Cloudflare だけの宣言では地域が無くても警告しない |
| VPC の枠 | ノードがセキュリティグループ・サブネット・サブネットグループを通じて VPC（VNet・Google Cloud のネットワーク）に行き着けば、その VPC の枠に入れる。ノードからは配置を表す属性だけをたどり、他のノードは通らない。枠は描くだけで計算には使わない | データベースを呼ぶだけの関数を、そのデータベースの VPC に入れない。モジュールごとに同じ VPC を引いていても1つの枠にまとめる。サブネットと AZ は資源が複数にまたがるので枠にしない |
| AZ 間の転送費 | 枠（VPC）の読み値にする。同じ枠の中の2ノードを結ぶ辺に `kb`（1単位あたりの KB、往復合計）を置き、枠の前提 `zones` から、またぐ割合（zones − 1）/ zones を掛ける。AWS は送り出しと受け取りの両方に $0.01/GB、Google Cloud は送る側だけ、Azure は無料 | ノードには AZ が分からないが、均等に散らばる前提なら割合は AZ 数だけで決まる。S3・DynamoDB などリージョンのサービスは AZ をまたがないので、その辺には `kb` を置かない。枠の外へ出る辺の `kb` は警告にして読まない |
| 提供のない地域 | Price List に単価が無い行は「提供なし」として記録し、その単価を使うノードは誤りにする | 0円として黙って通さない。Opus 4.5 を東京から呼ぶ構成などを、作る前に止める |
| CloudFront の単価 | 見る人の価格区分で引く。既定はリソースの地域の区分 | CloudFront の料金は配信先で決まり、リソースの地域では決まらない |
| ネットワーク資源 | Transit Gateway・VPN・NAT Gateway・VPC エンドポイントは経路の途中に置くノードにする。届いた負荷に1単位あたりの KB（`kbPerUnit`）を掛けて GB と帯域を出し、負荷はそのまま次のノードへ流す。VPC・サブネット・AZ はノードにしない | どの呼び出しがどの経路を通るかは Terraform から分からないので、辺は手で引く。負荷から GB を出すので、負荷を変えれば転送費も変わり、経路の遅延と可用性にもその区間が入る。負荷が届いているのに `kbPerUnit` が無ければ0にせず誤りにする |
| Bedrock の単価 | global と regional（地域プロファイル・リージョン内）を前提で選ぶ。既定は global | regional は global の1.1倍。上限は両者で別枠だが既定値は同じ |
| 未確認の値 | 報告の末尾に必ず並べる | 確かめていない数字を確かなものとして出さない |
| 上限の値 | 公開されている既定値を置き、アカウントで変わるものは備考に書く | AWS が公開していない値は未知のまま置き、需要だけを出す。AgentCore は SLA が未公開なので可用性も未知 |
| CloudFront への参照 | 呼び出しではなく言及として扱い、辺にしない | コールバック URL や招待メールのリンクは、内側から CloudFront を呼ぶことではない |
| data ソース | ノードにしない | 別の場所で管理される実物を読むだけで、コストはそちらに付く。複数のモジュールが同じ実物を読むと、その数だけ数えてしまう |
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

リソースは `catalog/<プロバイダ>`（aws・azure・gcp・cloudflare）の下に、資源型の名前のディレクトリとして1つずつ置く。
Go のコードは書かない。`resource.yaml` の `icon` は `web/src/ui/icons/<プロバイダ>/` の絵の名前で、
アイコンの無いリソースと、どのリソースも使わない絵はテストで落ちる。

| ファイル | 持つもの |
| --- | --- |
| `resource.yaml` | アイコン、受け付ける属性と前提、負荷を読み値に変える面の組み合わせ、Terraform の規則、IAM アクション |
| `books/prices.json` | 単価と、Price List で確かめるための取り込み条件 |
| `books/quotas.json` `books/slas.json` | 上限と SLA |
| `cases.yaml` | 入力と期待値の組。この前提とこの負荷なら、この金額と需要になる |

数値は式で、文字列には `{式}` を埋め込める。式はカタログを読み込んだ時点で型まで検査するので、
打ち間違いや型の食い違いが利用者の手元に届かない。`resource.yaml` を持たないディレクトリは、
複数のリソースが読む行（ログの単価など）を共有する。書き方の全体は [README.md](README.md#adding-a-resource) を見てください。

## ライセンス

[Apache License 2.0](LICENSE)

archgopher のロゴは、[Renée French](https://reneefrench.blogspot.com/) による Go gopher をもとにしています。
Go gopher は [CC BY 4.0](https://creativecommons.org/licenses/by/4.0/) で公開されています。

`web/src/ui/icons` のサービスのアイコンは各社のもので、Apache License の対象外です。出典と利用条件は [NOTICE.md](web/src/ui/icons/NOTICE.md) にあります。
