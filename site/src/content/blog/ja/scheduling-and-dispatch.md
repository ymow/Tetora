---
title: "Tetora のスケジューリングシステム — シンプルな Cron から複雑なワークフローまで"
lang: ja
date: "2026-04-11"
tag: explainer
readTime: "約6分"
excerpt: "Tetora がひとつの cron 式を、人間の承認ゲート付きの完全自動化ワークフローに変える仕組みと、スケジューリングスタックの各要素の使いどころ。"
description: "Tetora のスケジューリングシステム完全ガイド：cronジョブ、dispatchキュー、Human Gate承認チェックポイント、複雑な多段階自動化のためのWorkflow DAG。"
---

ほとんどの AI エージェントツールはインタラクティブだ——プロンプトを入力すると、エージェントが応答する。アドホックな作業にはそれで十分だ。しかし、最も時間を節約する自動化は、あなたが関与しなくても動くものだ。

Tetora にはスケジューリングスタックが組み込まれている。この記事では各レイヤーを順に解説し、それぞれの使いどころと、組み合わせることでどれだけ強力なシステムになるかを説明する。

## レイヤー1：Cron スケジューラ

最もシンプルなスケジューリングの基本単位は cron ジョブだ。実行時刻とディスパッチ内容を指定するだけでいい：

```bash
tetora job add --cron "0 21 * * *" "Run nightly accounting check"
tetora job add --cron "0 9 * * 1" "Weekly team standup summary"
```

標準の cron 構文——分、時、日、月、曜日の5フィールドだ。cron の経験があれば新しいことは何もない。経験がなくても、cron 式ジェネレーターはネット上にたくさんある。Tetora は有効な cron 文字列であれば何でも受け付ける。

スケジュール済みジョブの確認：

```bash
tetora job list
```

ジョブの削除：

```bash
tetora job remove <job-id>
```

cron ジョブが起動すると、ディスパッチキューにタスクを作成してすぐに返る。cron スケジューラはタスクの完了を待たない——タスクが時間通りに作成されることだけを保証する。

## レイヤー2：Dispatch キュー

Dispatch キューは実行レイヤーだ。タスクはさまざまな経路でキューに入る——cron ジョブ、手動の `tetora dispatch` 呼び出し、ワークフローステップ、webhook トリガー——そしてエージェントが受け取って実行する。

キューの並行数は設定可能だ：

```json
{
  "dispatch": {
    "maxSlots": 4,
    "slotPressureThreshold": 0.8
  }
}
```

`maxSlots` は同時実行するタスク数を制御する。すべてのスロットが埋まっているときに新しいタスクが来ると、強制起動してリソースを奪い合うのではなく、キューで待機する。`slotPressureThreshold` は追加のバッファを設ける。スロット利用率がこの割合を超えると、技術的にまだ空きがあっても新しいタスクはキューに入る。

個人利用のほとんどのケースでは `maxSlots: 2` か `3` が適切だ——並行タスクを処理しつつ、ローカルリソースを圧迫しないバランスだ。

キューの状態はいつでも確認できる：

```bash
tetora status
```

実行履歴の確認：

```bash
tetora history fails          # 直近の失敗を表示
tetora history trace <task-id>  # 特定タスクの完全トレース
```

## レイヤー3：Human Gate

完全に自動化されたパイプラインに乗せるべきでないタスクがある。複雑だからではなく、結果が重大だからだ——取り返しのつかない操作、実際の外部システムに影響するアクション、エージェントに単独で判断させるべきでない意思決定がそれにあたる。

Human Gate は任意のワークフローステップに承認チェックポイントを追加する。エージェントがそのステップに到達すると、一時停止し、通知を送り、明示的な承認を受けるまで待機する。

```json
{
  "humanGate": {
    "enabled": true,
    "timeoutHours": 4,
    "notifyDiscord": true
  }
}
```

`timeoutHours` は、エスカレーションまたはステップの放棄をするまでエージェントが待つ時間を制御する。`notifyDiscord` は承認が必要なとき設定した Discord チャンネルにメッセージを送る——キーボードから離れているときに実行されるワークフローで便利だ。

Workflow YAML では `humanGate: true` でステップをゲート付きとしてマークする：

```yaml
- id: review-uncertain
  humanGate: true
  run: "Flag transactions with confidence < 0.8 for human review"
  depends: [classify]
```

エージェントはこのステップに到達すると、信頼度の低いトランザクションをレビュー用に提示して待機する。承認（CLI または Discord 経由）すると次のステップに進む。拒否すると、そこでワークフローが停止し、結果がログに記録される。

Human Gate は自動化を使いにくくするものではない。完全な自律が不適切な状況でも安全に使えるようにするものだ。

## レイヤー4：Workflow DAG

各タスクが独立している場合、シンプルな cron ジョブで十分だ。ステップ間に依存関係がある多段階プロセスには、Workflow DAG でパイプライン全体を宣言的に定義できる。

実用的な例を見てみよう——公認会計士の畠山謙人氏が使うパターンをベースにした毎晩の記帳ワークフローだ：

```yaml
name: nightly-accounting
steps:
  - id: fetch-transactions
    run: "Fetch today's unprocessed transactions from the freee API"
  - id: classify
    run: "Classify each transaction by account category"
    depends: [fetch-transactions]
  - id: review-uncertain
    humanGate: true
    run: "Flag transactions with confidence < 0.8 for human review"
    depends: [classify]
  - id: post-entries
    run: "Post approved entries to freee"
    depends: [review-uncertain]
```

実行は DAG に沿って流れる。`fetch-transactions` が最初に動き、次に `classify`（フェッチ完了に依存）、次に `review-uncertain`（人間の承認を待って一時停止）、最後に `post-entries`（承認を受けてから実行）の順だ。

`fetch-transactions` が失敗すると、下流のステップは何も実行されない。`review-uncertain` が承認なくタイムアウトすると、`post-entries` は実行されない。DAG 構造により、失敗モードが明確になり追跡可能になる。

`depends` フィールドを持たないステップは、同時に準備できていれば並行実行される。マージステップの前に複数の独立したソースからデータを取得するワークフローでは、フェッチが並行して走り、合計実行時間が短縮される。

### 条件分岐

ステップには `condition` フィールドを設定できる——条件が真のときだけステップが実行される：

```yaml
- id: send-alert
  run: "Send Slack alert about anomalous transactions"
  condition: "{{classify.anomaly_count}} > 0"
  depends: [classify]
```

分類ステップで異常が見つからなければ、アラートステップは完全にスキップされる。別のトリガー機構を用意せずに、パイプラインがデータに応じて動作を変える。

## 組み合わせると

これらのレイヤーは綺麗に組み合わさる：

- **Cron** が毎晩21:00にジョブを起動する
- そのジョブが **Workflow** タスクをディスパッチする
- ワークフローがいくつかの自動化ステップを **dispatch キュー** を通して実行する
- 重要な書き戻しステップで **Human Gate** が承認待ちで一時停止する
- 確認後、残りのワークフローが完了する

結果は、スケジュール通りに確実に動き、エラーを予測可能に処理し、可能なところで並行化し、自動化すべきでない判断を人間にエスカレーションするパイプラインだ。

これはまさに畠山氏が数ヶ月かけて手作りしたセットアップだ——Tetora はそれをカスタムインフラ不要の組み合わせ可能なシステムとして提供する。

## 設計哲学

Tetora のスケジューリングシステムの目標は、適切なコントロールレベルを適切なアクションに対応させることだ。

API からデータを取得する？完全自動。キーワードでトランザクションを分類する？完全自動。クライアントの財務記録に影響する仕訳を転記する？Human Gate。レポートをクライアントに送る？Human Gate。

どのアクションがどちらのカテゴリに属するかの判断は、あなたに委ねられる——自分のビジネスコンテキストとリスク許容度を理解している人間だ。Tetora はその判断を一度だけエンコードすれば、毎回の実行で一貫して適用される仕組みを提供する。

**ルーティンを自動化する。判断はエスカレーションする。** スケジューリングスタックは、この原則を実際に動かす仕組みだ。
