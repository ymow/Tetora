---
title: "Tetora 的排程系統——從簡單 Cron 到複雜工作流程"
lang: zh-TW
date: "2026-04-11"
tag: explainer
readTime: "~6 分鐘"
excerpt: "Tetora 如何把一個 cron 表達式變成全自動、有人工審核節點的工作流程——以及什麼時候該用排程堆疊的哪個部分。"
description: "Tetora 排程系統完整指南：cron 任務、dispatch 佇列、Human Gate 審核節點，以及用於複雜多步驟自動化的 Workflow DAG。"
---

大多數 AI Agent 工具是互動式的——你輸入提示詞，Agent 回應。這對臨時性工作很有用。但最省時間的自動化，是那些不需要你在場就能運行的自動化。

Tetora 內建了完整的排程堆疊。這篇文章逐層說明每個層次的用途、使用時機，以及它們如何組合成比任何單一部件都更強大的系統。

## 第一層：Cron 排程器

最基本的排程原語是 cron 任務。你指定何時執行以及要 dispatch 什麼：

```bash
tetora job add --cron "0 21 * * *" "Run nightly accounting check"
tetora job add --cron "0 9 * * 1" "Weekly team standup summary"
```

標準 cron 語法——五個欄位分別對應分鐘、小時、日期、月份、星期幾。如果你用過 cron，沒有新東西要學。如果沒用過，網路上有很多 cron 表達式生成器；Tetora 接受任何有效的 cron 字串。

查看已排程的任務：

```bash
tetora job list
```

移除任務：

```bash
tetora job remove <job-id>
```

當 cron 任務觸發時，它會在 dispatch 佇列中建立一個任務然後立即返回。Cron 排程器不等待任務完成——它只確保任務準時建立。

## 第二層：Dispatch 佇列

Dispatch 佇列是執行層。任務從各種來源進入佇列——cron 任務、手動 `tetora dispatch` 呼叫、工作流程步驟、webhook 觸發——然後由 Agent 接手執行。

佇列的並發設定可以調整：

```json
{
  "dispatch": {
    "maxSlots": 4,
    "slotPressureThreshold": 0.8
  }
}
```

`maxSlots` 控制同時執行幾個任務。當所有槽位都佔用而新任務進來時，它會在佇列中等待，而不是強制啟動並與其他任務競爭資源。`slotPressureThreshold` 增加了額外的緩衝：一旦槽位使用率超過這個比例，即使技術上還沒全滿，新任務也會排隊等待。

對大多數個人使用情境，`maxSlots: 2` 或 `3` 是合理的——足以執行並行任務，又不會壓垮本地資源。

隨時可以查看佇列狀態：

```bash
tetora status
```

以及查看執行歷史：

```bash
tetora history fails          # 顯示最近的失敗
tetora history trace <task-id>  # 特定任務的完整追蹤
```

## 第三層：Human Gate

有些任務不應該在完全自動化的管道中執行。不是因為它們複雜，而是因為它們後果重大——難以撤銷的操作、影響真實外部系統的操作，或者需要不應由 Agent 單方面做出的判斷。

Human Gate 為任何工作流程步驟新增審核節點。當 Agent 到達該步驟時，它會暫停、通知你，並等待明確批准後才繼續。

```json
{
  "humanGate": {
    "enabled": true,
    "timeoutHours": 4,
    "notifyDiscord": true
  }
}
```

`timeoutHours` 控制 Agent 在升級或放棄該步驟前等待多久。`notifyDiscord` 會在需要批准時傳訊息到你設定的 Discord 頻道——對於你不在電腦旁時執行的工作流程很實用。

在 Workflow YAML 中，用 `humanGate: true` 標記需要審核的步驟：

```yaml
- id: review-uncertain
  humanGate: true
  run: "Flag transactions with confidence < 0.8 for human review"
  depends: [classify]
```

Agent 到達這個步驟時，會把低信賴度的交易呈現給你審查，然後等待。你批准（透過 CLI 或 Discord）後，它繼續下一步。如果你拒絕，工作流程在此停止並記錄結果。

Human Gate 不會讓自動化變得不好用。它讓自動化可以安全地用在完全自主會不適當的場景中。

## 第四層：Workflow DAG

當每個任務都是獨立的時候，簡單的 cron 任務效果很好。對於步驟之間有依賴關係的多步驟流程，Workflow DAG 讓你以宣告式方式定義整個管道。

以下是一個實際例子——以稅務師畠山謙人使用的模式為基礎的每晚會計工作流程：

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

執行依照 DAG 流動：`fetch-transactions` 先執行，然後是 `classify`（依賴取得完成），接著是 `review-uncertain`（暫停等待人工批准），最後是 `post-entries`（只在收到批准後執行）。

如果 `fetch-transactions` 失敗，下游的任何步驟都不會執行。如果 `review-uncertain` 在沒有批准的情況下逾時，`post-entries` 永遠不會執行。DAG 結構讓失敗模式變得明確且可追蹤。

沒有 `depends` 欄位的步驟，如果同時就緒，可以並行執行。對於在合併步驟之前需要從多個獨立來源取得資料的工作流程，這意味著取得動作會同步進行，減少總執行時間。

### 條件路由

步驟可以包含 `condition` 欄位——只有當條件為真時，步驟才會執行：

```yaml
- id: send-alert
  run: "Send Slack alert about anomalous transactions"
  condition: "{{classify.anomaly_count}} > 0"
  depends: [classify]
```

如果分類步驟沒有發現異常，警示步驟完全跳過。管道會根據資料調整，不需要另外的觸發機制。

## 組合在一起

這些層次組合得很乾淨：

- **Cron** 每晚 21:00 觸發一個任務
- 該任務 dispatch 一個 **Workflow** 任務
- 工作流程透過 **dispatch 佇列**執行幾個自動化步驟
- 在關鍵的回寫步驟，**Human Gate** 暫停等待批准
- 確認後，工作流程的其餘部分完成

結果是一個按排程可靠執行、可預期地處理錯誤、在可能的地方並行執行，並且把不應該自動化的判斷升級給人類處理的管道。

這正是畠山花數月手動建立的設定——Tetora 把它作為一個可組合的系統提供，不需要任何自建基礎設施。

## 設計理念

Tetora 排程系統的目標是把正確的控制層次對應到正確的操作。

從 API 取得資料？完全自動。用關鍵字分類交易？完全自動。回寫影響客戶財務記錄的分錄？Human Gate。傳送報告給客戶？Human Gate。

哪個操作屬於哪個類別的判斷，屬於你——那個了解自己業務脈絡和風險承受度的人。Tetora 給你一次性編碼這個判斷的機制，並在每次執行時一致地執行它。

**自動化例行事務，升級需要判斷的事。** 排程堆疊就是這個原則如何變成可操作的方式。
