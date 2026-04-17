---
title: "在 Discord 討論串中並行執行多個 Agent"
lang: zh-TW
date: "2026-03-23"
excerpt: "使用 Discord 討論串搭配 /focus，讓各個 agent 在獨立 context 下並行運作。"
description: "學習如何使用 Discord 討論串與 /focus 指令，讓多個 Tetora agent 同時以獨立工作階段並行運行。"
---

Tetora 的 Discord 整合讓你可以透過將不同討論串繫結至不同 agent，**同時**執行多個 agent。每個討論串都有自己獨立的工作階段——對話之間不會有 context 污染。

## 運作原理

你的主 Discord 頻道共用單一工作階段。若要讓不同 agent 執行並行任務，請建立討論串並使用 `/focus` 將 agent 指派給各個討論串。

```
#general（主頻道）                        ← 共用工作階段
  └─ Thread: "Refactor auth module"    ← /focus kokuyou → 獨立工作階段
  └─ Thread: "Write blog post"         ← /focus kohaku  → 獨立工作階段
  └─ Thread: "Competitor analysis"     ← /focus hisui   → 獨立工作階段
```

## 操作步驟

**1. 建立 Discord 討論串** — 右鍵點擊訊息 → 建立討論串，或使用 Discord 的討論串按鈕。

**2. 在討論串內繫結 agent：**

```
/focus kokuyou
```

繫結完成後，該討論串中的所有訊息都會路由至指定 agent，並使用其獨立的對話歷史記錄。

**3. 對其他任務重複同樣步驟** — 依需求開啟任意數量的討論串，每個討論串指派不同（或相同）的 agent。

**4. 完成後解除繫結：**

```
/unfocus
```

## 設定

在 `config.json` 中啟用討論串繫結功能：

```json
{
  "discord": {
    "threadBindings": {
      "enabled": true,
      "ttlHours": 24
    }
  }
}
```

## 注意事項

- **討論串繫結會過期** — 預設 24 小時後失效（可透過 `ttlHours` 調整）。過期後，討論串會回退至主頻道的路由規則。
- **工作階段完全隔離** — 討論串的 context 絕不會洩漏至主頻道或其他討論串。
- **並行限制** — 所有頻道與討論串共用全域 `maxConcurrent` 上限（預設為 8）。超過上限的訊息會進入佇列等待。
- `/focus` 僅在討論串內有效。主頻道始終使用單一工作階段——使用 `!new` 可重置它。
