---
title: "為什麼 Tetora 使用官方 Claude API——以及為什麼這很重要"
lang: zh-TW
date: "2026-04-10"
tag: explainer
readTime: "~4 分鐘"
excerpt: "Anthropic 在 2026 年 4 月的執法措施澄清了 Claude 訂閱可以做什麼。以下是 Tetora 如何整合 Claude 的技術細節，以及為什麼我們不會被封鎖。"
description: "Tetora 透過官方 claude-code CLI 或直接 API 金鑰整合 Claude，而非 OAuth 訂閱令牌。以下是技術細節，以及為什麼這對可靠性至關重要。"
---

2026 年 2 月 20 日，Anthropic 更新了使用政策，其中一項澄清影響了大量第三方 Claude 工具生態系：**訂閱 OAuth 令牌不得用於第三方應用程式**。這個政策一直都是這個意思；這次更新讓它變得更加明確。

4 月 4 日，Anthropic 依此政策對 OpenClaw 採取執法措施，全面封鎖了該工具。對每一個使用它的人來說，服務在當天就中斷了。

這篇文章詳細說明 Tetora 如何整合 Claude，以及為什麼這個設計選擇讓 Tetora 不受這次執法措施的影響。

## 兩條允許的整合路徑

Anthropic 的允許使用政策指定了兩種建構 Claude 應用程式的方式：

1. **官方 `claude-code` CLI** — Anthropic 發布的第一方命令列介面
2. **使用 API 金鑰的 Anthropic API** — 用從 console.anthropic.com 生成的 API 金鑰進行驗證的直接 HTTP 呼叫

兩者都是穩定、明確支援的，並遵循 Anthropic 的標準 API 條款。使用這兩條路徑時，你與 Anthropic 的帳單關係是直接且透明的——你用多少付多少。

**不被允許**的是在第三方工具中使用支撐 Claude.ai 訂閱的 OAuth 令牌。那個令牌是用來驗證 claude.ai 的網頁工作階段；用它來驅動外部工具從未獲得授權，即使技術上可行。

## Tetora 的設定方式

Tetora 的整合方法由單一設定欄位決定：`claudeProvider`。

對於已安裝 `claude-code` CLI 的使用者：

```json
{
  "claudeProvider": "claude-code"
}
```

Tetora 以子行程方式呼叫 `claude-code`，就像你從終端機使用它一樣。CLI 自行管理驗證、工作階段處理以及與 Anthropic 伺服器的通訊。Tetora 完全不直接碰你的驗證憑證。

對於偏好直接 API 存取的使用者：

```json
{
  "claudeProvider": "anthropic"
}
```

在這個模式下，Tetora 使用儲存在你本地 Tetora 設定中的 API 金鑰直接呼叫 Anthropic API（也可以透過環境變數提供）。這個金鑰從你的 Anthropic 控制台發行，與你帳號的 API 帳單綁定。你可以設定用量上限、監控費用，並隨時透過 Anthropic 自己的工具輪換金鑰。

兩種整合路徑都不使用訂閱 OAuth 令牌，也不繞過 Anthropic 的帳單或存取控制。兩者都正是 Anthropic 支援的整合模式。

## 為什麼這對可靠性很重要

當你在 Tetora 上建構自動化時，你是建立在有兩個重要特性的基礎上：

**沒有執法風險。** Tetora 沒有做任何違反 Anthropic 政策的事。不存在類似 OpenClaw 那樣的情境——某個政策執法行動在一夜之間讓你的工作流程全部停擺。

**可預測的費用。** API 金鑰用量計量計費，透過你的 Anthropic 帳號結算。你可以設定硬性消費上限、設定門檻警示，並在 Anthropic 控制台查看每次請求的費用明細。訂閱用量沒有這種粒度——你付固定費率，然後希望它能涵蓋你的用量。對於按排程或事件觸發的自動化來說，計量計費與實際消耗模式的對應好得多。

**可攜性。** API 金鑰無論在你的筆電、家用伺服器還是遠端虛擬機上運行 Tetora 都一樣有效。不需要維護帳號工作階段，不需要保持瀏覽器驗證，沒有定期重新驗證的流程。金鑰在你輪換之前一直有效。

## 關於合規的更廣泛思考

人們容易把合規視為一種限制——一套限制你能建什麼的規則。OpenClaw 的情況說明了相反的框架：**合規是承重基礎設施**。

當一個工具建立在未授權的整合路徑上，不只是在抽象意義上違反了規則。它是把使用者的工作流程建立在一個隨時可能被終止的依賴上，而終止只需要一個超出工具控制範圍的執法決定。那個工具的每個使用者都暴露在這個風險下，不管他們是否意識到。

Tetora 遵守 Anthropic API 條款不是行銷重點——這是設計要求。自動化只有持續運行才有用。

## 確認你的設定

如果你目前使用 `claude-code` 模式，可以用這個指令確認：

```bash
tetora config show | grep claudeProvider
```

如果你想切換到直接 API 模式，更新設定並提供你的金鑰：

```bash
tetora config set claudeProvider anthropic
tetora config set anthropicApiKey sk-ant-...
```

你的 API 金鑰也可以透過 `ANTHROPIC_API_KEY` 環境變數設定，如果設定欄位沒有填寫，Tetora 會自動讀取它。

兩種模式都完全支援。選哪個取決於你的偏好：如果你已經使用 CLI，`claude-code` 設定更簡單；`anthropic` 則提供更直接的費用可見性與控制。
