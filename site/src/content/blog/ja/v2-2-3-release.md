---
title: "Tetora v2.2.3–v2.2.4 — モデルピッカー、TTS、Human Gate"
lang: ja
date: "2026-04-04"
tag: release
readTime: "約5分"
excerpt: "インタラクティブなモデル切り替え、VibeVoice TTS、Human Gate の改善、Skill AllowedTools、Discord の !model コマンド。"
description: "Tetora v2.2.3 はインタラクティブなモデル切り替え（Discord + Dashboard）、VibeVoice TTS、Human Gate の retry/cancel/通知、Skill AllowedTools、learned skill の自動抽出を追加します。"
---

v2.2.3 と v2.2.4 がまとめてリリースされました。Discord と Dashboard でのインタラクティブなモデル切り替え、VibeVoice によるローカル・クラウド TTS、より強力な Human Gate、スキルごとのツール制限、セッション履歴からの自動スキル抽出と、充実した内容です。v2.2.4 はバグ修正とインフラ強化を続けて提供します。

> **TL;DR：** `!model pick` で Discord からインタラクティブに provider とモデルを切り替えられます。VibeVoice でローカル TTS とクラウドフォールバックが利用可能になります。Human Gate が retry、cancel、Discord 通知に対応しました。`allowedTools` で Skill が呼び出せるツールを制限できます。Learned skill はセッション履歴から自動抽出されます。

## モデル切り替え

### Discord コマンド

設定ファイルを触らずに推論モデルを切り替えられます。Tetora が有効なチャンネルで 3 つの新コマンドが使えます：

**`!model pick`** — 3 ステップのフローでインタラクティブなピッカーを開きます：

```
ステップ 1: provider 選択  →  ステップ 2: モデル選択  →  ステップ 3: 確認
```

各ステップは番号付き選択肢を含む Discord メッセージで表示されます。番号を入力すると次のステップに進みます。

**`!local` / `!cloud`** — すべての agent の推論モードを一括切り替えします。`!local` はすべての agent を設定済みのローカル provider（Ollama、LM Studio など）に切り替えます。`!cloud` でクラウド provider に戻します。

**`!mode`** — 現在の推論設定のサマリーを表示します：アクティブな provider、モデル、グローバルモード。

### Dashboard モデルピッカー

Dashboard の agent カードにモデル設定が直接表示されるようになりました：

- **Provider バー** — 各 agent カードの上部にアクティブな provider をカラーコードバッジで表示（クラウドは青、ローカルは緑）
- **モデルドロップダウン** — agent カード上でクリックするだけで、Settings に移動せずその agent のモデルを切り替え可能
- **グローバル推論モードトグル** — ヘッダーバーのスイッチ 1 つで、すべての agent を Cloud と Local の間で一括切り替え

### Claude Provider 設定

`config.json` に新しい `claudeProvider` フィールドが追加され、Tetora が Claude モデルを呼び出す方法を制御できます：

```json
{
  "claudeProvider": "claude-code"
}
```

- `"claude-code"` — Claude Code CLI 経由で Claude を呼び出します。有効な Claude サブスクリプションがあるローカル環境のデフォルト。
- `"anthropic"` — `ANTHROPIC_API_KEY` を使って Anthropic API を直接呼び出します。ヘッドレス環境や CI での実行時のデフォルト。

フィールドはインストールごとに設定可能なため、ローカル開発マシンとリモートサーバーが設定の競合なく異なる呼び出しパスを使えます。

## VibeVoice TTS

Tetora がしゃべるようになりました。VibeVoice 統合により agent の応答に音声出力が追加され、2 段階のフォールバックチェーンを提供します：

1. **ローカル VibeVoice** — デバイス上で実行。モデル読み込み後はゼロレイテンシ、完全プライバシー
2. **fal.ai クラウド TTS** — ローカル VibeVoice が使えないか失敗した場合に自動切り替え

`config.json` で設定します：

```json
{
  "tts": {
    "enabled": true,
    "provider": "vibevoice",
    "fallback": "fal"
  }
}
```

TTS はデフォルトで無効です。有効にすると、agent は Discord ボイスチャンネルと Dashboard のモニタービューで応答を読み上げます。

## Human Gate の改善

Human Gate——agent の実行を一時停止して人間の承認を求める Tetora のメカニズム——が大幅に使いやすくなりました。

### Retry と Cancel

レビュアーは手動介入なしに、以前拒否されたゲートに対してアクションを取れるようになりました：

- **Retry API** — `POST /api/gate/:id/retry` でゲートをレビュー待ちに再投入し、状態を `waiting` にリセット
- **Cancel API** — `POST /api/gate/:id/cancel` で一時停止中のタスクをクリーンに終了
- 両アクションは Dashboard の Task Detail モーダルに既存の Approve/Reject ボタンと並んで表示

### Discord 通知

Human Gate イベントが設定済みの通知チャンネルに Discord メッセージを送信するようになりました：

- **Waiting** — ゲートが開いてレビュー待ちになったときにレビュアーに通知
- **Timeout** — ゲートがアクションなしに期限切れになったとき、影響を受けたタスクを含めてチャンネルに通知
- **Assignee メンション** — ゲートに割り当てレビュアーがいる場合、そのユーザーを通知内で直接 `@mention`

### 統合アクションフィールド

ゲートイベントスキーマが承認データを 2 つのフィールドに統合しました：

```json
{
  "action": "approve | reject | retry | cancel",
  "decision": "approved | rejected"
}
```

これにより、以前の `approved`、`rejected`、`action` フィールドが混在していた状態が解消されます。旧フィールドは 1 リリースサイクルの間は読み取り可能で、その後削除されます。

## Skill AllowedTools

Skill がツール制限リストに対応しました。Skill の設定に `allowedTools` を設定することで、その Skill が呼び出せる MCP ツールを制限できます：

```json
{
  "name": "freee-check",
  "allowedTools": ["mcp__freee__list_transactions", "mcp__freee__get_company"],
  "prompt": "Check unprocessed entries for all companies."
}
```

`allowedTools` を設定すると、Skill はサンドボックス context で実行され、シェルコマンド、ファイルシステムへのアクセス、リストにない MCP ツールなど他のツールは利用できなくなります。これにより Skill レベルで最小権限が強制され、監査証跡もクリーンになります。

## Learned Skill 自動抽出

Tetora がセッション履歴から再利用可能なパターンを自動的に特定し、新しい Skill として提案するようになりました。

セッション終了後、バックグラウンドプロセスが会話をスキャンして繰り返しコマンドシーケンスやマルチステップパターンを探します。候補は `SKILL.md` と `metadata.json` とともに `skills/learned/` に書き込まれ、レビューが完了するまで `approved: false` としてフラグが立てられます。

CLI から提案中の skill を確認します：

```bash
tetora skill list --pending      # レビュー待ちの提案 skill を表示
tetora skill approve <name>      # アクティブに昇格
tetora skill reject <name>       # 提案を破棄
```

承認された skill はすぐにスラッシュコマンドとして使えます。

## v2.2.4 修正

v2.2.4 は安定化リリースです。主な修正：

- **i18n URL 重複排除** — 生成された URL でロケールプレフィックスが二重になるルーティングバグを修正（例：`/en/en/blog/...` → `/en/blog/...`）。
- **Skills cache RWMutex** — skills cache のプレーン mutex を読み書きロックに変更し、読み取り負荷の高いワークロードのスループットを向上。
- **SEO 改善** — すべてのブログとドキュメントページに `BreadcrumbList` 構造化データと正しい `og:locale` 値を追加。
- **リグレッションガードテスト** — i18n URL 重複排除修正と skills cache のインテグレーションテストを追加してリグレッションを防止。

## アップグレード

```bash
tetora upgrade
```

シングルバイナリ。依存関係ゼロ。macOS / Linux / Windows 対応。

[GitHub で完全な Changelog を見る](https://github.com/TakumaLee/Tetora/releases/tag/v2.2.4)
