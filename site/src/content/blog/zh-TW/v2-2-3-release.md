---
title: "Tetora v2.2.3–v2.2.4 — 模型選擇器、TTS、Human Gate"
lang: zh-TW
date: "2026-04-04"
tag: release
readTime: "~5 分鐘"
excerpt: "互動式模型切換器、VibeVoice TTS、Human Gate 改進、Skill AllowedTools，以及 Discord !model 指令。"
description: "Tetora v2.2.3 新增互動式模型切換（Discord + Dashboard）、VibeVoice TTS、Human Gate retry/cancel/通知、Skill AllowedTools，以及從工作階段歷史自動提取 learned skill。"
---

v2.2.3 與 v2.2.4 同步登場，帶來一組聚焦的改進：Discord 與 Dashboard 的互動式模型切換、透過 VibeVoice 實現本地與雲端 TTS、更強大的 Human Gate、per-skill 工具限制，以及從工作階段歷史自動提取 skill。v2.2.4 接續進行 bug 修復與基礎設施強化。

> **TL;DR：** `!model pick` 讓你從 Discord 互動切換 provider 與模型。VibeVoice 帶來本地 TTS 搭配雲端備援。Human Gate 現支援 retry、cancel 與 Discord 通知。`allowedTools` 限制 Skill 可呼叫的工具。Learned skill 從工作階段歷史自動提取。

## 模型切換

### Discord 指令

無需修改設定檔即可切換推理模型。在任何 Tetora 活動的頻道中可使用三個新指令：

**`!model pick`** — 開啟互動式選擇器，三步驟流程：

```
第一步：選擇 provider  →  第二步：選擇模型  →  第三步：確認
```

每個步驟以附有編號選項的 Discord 訊息呈現，輸入數字即可進入下一步。

**`!local` / `!cloud`** — 一次批量切換所有 agent 的推理模式。`!local` 將所有 agent 切換至設定的本地 provider（Ollama、LM Studio 等），`!cloud` 則切回雲端 provider。

**`!mode`** — 顯示目前推理設定的摘要：啟用的 provider、模型與全域模式。

### Dashboard 模型選擇器

Dashboard 現在在 agent 卡片上直接呈現模型設定：

- **Provider 狀態列** — 每張 agent 卡片頂端顯示啟用中的 provider，以色碼標章區分（雲端為藍色，本地為綠色）
- **模型下拉選單** — 在 agent 卡片上點擊即可切換該 agent 的模型，無需前往 Settings
- **全域推理模式切換** — 標頭列的單一開關，一次將所有 agent 在 Cloud 與 Local 之間切換

### Claude Provider 設定

`config.json` 新增 `claudeProvider` 欄位，控制 Tetora 呼叫 Claude 模型的方式：

```json
{
  "claudeProvider": "claude-code"
}
```

- `"claude-code"` — 透過 Claude Code CLI 呼叫 Claude。適用於擁有有效 Claude 訂閱的本地安裝，為預設值。
- `"anthropic"` — 使用 `ANTHROPIC_API_KEY` 直接呼叫 Anthropic API。在無頭環境或 CI 中執行時的預設值。

此欄位可按安裝環境個別設定，本地開發機器與遠端伺服器可使用不同的呼叫路徑，不會產生設定衝突。

## VibeVoice TTS

Tetora 現在會說話了。VibeVoice 整合為 agent 回應帶來文字轉語音輸出，並提供兩層備援機制：

1. **本地 VibeVoice** — 在裝置上執行，模型載入後零延遲，完全保護隱私
2. **fal.ai 雲端 TTS** — 當本地 VibeVoice 不可用或失敗時自動啟用

在 `config.json` 中設定：

```json
{
  "tts": {
    "enabled": true,
    "provider": "vibevoice",
    "fallback": "fal"
  }
}
```

TTS 預設關閉。啟用後，agent 在 Discord 語音頻道與 Dashboard 監控視圖中會朗讀回應內容。

## Human Gate 改進

Human Gate——Tetora 暫停 agent 執行並請求人工審核的機制——獲得了重大的使用體驗提升。

### Retry 與 Cancel

審查者現在無需手動介入即可對先前被拒絕的 gate 採取行動：

- **Retry API** — `POST /api/gate/:id/retry` 將 gate 重新加入佇列等待審查，狀態重置為 `waiting`
- **Cancel API** — `POST /api/gate/:id/cancel` 乾淨地終止暫停中的任務
- 兩個操作均在 Dashboard Task Detail modal 中呈現，與現有的 Approve/Reject 按鈕並列

### Discord 通知

Human Gate 事件現在會在設定的通知頻道觸發 Discord 訊息：

- **Waiting** — gate 開啟並等待審核時通知審查者
- **Timeout** — gate 未經處理到期時通知頻道，包含受影響的任務資訊
- **Assignee 提及** — 若 gate 有指定審查者，該用戶會在通知中被直接 `@mention`

### 統一動作欄位

Gate 事件 schema 將核准資料整合為兩個欄位：

```json
{
  "action": "approve | reject | retry | cancel",
  "decision": "approved | rejected"
}
```

這取代了先前混用的 `approved`、`rejected` 與 `action` 欄位。舊欄位在一個版本週期內仍可讀取，之後將移除。

## Skill AllowedTools

Skill 現在支援工具限制清單。在 Skill 設定中設定 `allowedTools`，可限制該 Skill 能呼叫哪些 MCP 工具：

```json
{
  "name": "freee-check",
  "allowedTools": ["mcp__freee__list_transactions", "mcp__freee__get_company"],
  "prompt": "Check unprocessed entries for all companies."
}
```

設定 `allowedTools` 後，Skill 在沙箱 context 中執行，其他工具——包含 shell 指令、檔案系統存取，以及清單以外的任何 MCP 工具——均不可用。這在 Skill 層級強制執行最小權限原則，並讓稽核追蹤更加清晰。

## Learned Skill 自動提取

Tetora 現在會自動識別工作階段歷史中的可重用模式，並將其提案為新的 Skill。

工作階段結束後，背景程序會掃描對話，尋找重複的指令序列與多步驟模式。候選項目會寫入 `skills/learned/`，包含 `SKILL.md` 與 `metadata.json`，並標記為 `approved: false` 直到審查完成。

透過 CLI 審查提案中的 skill：

```bash
tetora skill list --pending      # 顯示等待審查的提案 skill
tetora skill approve <name>      # 升級為啟用狀態
tetora skill reject <name>       # 捨棄提案
```

核准的 skill 立即可作為斜線指令使用。

## v2.2.4 修復

v2.2.4 是穩定性版本，主要修復：

- **i18n URL 去重複** — 修復生成的 URL 中語言代碼前綴重複的路由問題（例如 `/en/en/blog/...` → `/en/blog/...`）。
- **Skills cache RWMutex** — 將 skills cache 的普通 mutex 改為讀寫 mutex，提升讀取密集工作負載的吞吐量。
- **SEO 改進** — 為所有部落格與文件頁面新增 `BreadcrumbList` 結構化資料與正確的 `og:locale` 值。
- **回歸防護測試** — 新增涵蓋 i18n URL 去重複修復與 skills cache 的整合測試，防止回歸。

## 升級

```bash
tetora upgrade
```

單一執行檔，零外部依賴。支援 macOS / Linux / Windows。

[在 GitHub 上查看完整 Changelog](https://github.com/TakumaLee/Tetora/releases/tag/v2.2.4)
