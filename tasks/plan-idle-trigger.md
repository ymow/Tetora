# Idle-Triggered Self-Evolution — 實作計畫

## Context

琉璃的自我進化目前用固定 cron（每天 3:30 AM）。問題：主人休息時間不固定，固定時間可能被干擾。

**目標**：用 heartbeat 偵測系統閒置狀態，閒置夠久時自動觸發進化，取代固定排程。

## 核心概念

- **Heartbeat**（每 30 秒）= 感測器，追蹤系統是否閒置
- **Cron engine** = 執行器，看到閒置信號後觸發 job
- 新增 `trigger: "idle"` 模式：不需要 cron 時間匹配，純靠閒置觸發

## 閒置判定條件（全部同時滿足）

1. 無執行中 dispatched tasks（`dispatchState.running` 為空）
2. 無活躍 hook workers（所有 `hookWorker.State != "working"`）
3. 無使用者 session（`countUserSessions() == 0`）

三者全滿足 → 開始計時。持續閒置 ≥ `idleMinMinutes` → 可觸發。

---

## 修改檔案

### 1. hooks.go — 新增 `HasActiveWorkers()`

```go
// HasActiveWorkers returns true if any hook worker is in "working" state.
func (hr *hookReceiver) HasActiveWorkers() bool {
    hr.hookWorkersMu.RLock()
    defer hr.hookWorkersMu.RUnlock()
    for _, w := range hr.hookWorkers {
        if w.State == "working" {
            return true
        }
    }
    return false
}
```

---

### 2. heartbeat.go — 擴展系統閒置追蹤

**HeartbeatMonitor 新增欄位：**

```go
// 在 HeartbeatMonitor struct 中新增：
systemIdleCheckFn func() bool   // 注入的閒置檢查函式
idleMu            sync.RWMutex
systemIdleSince   time.Time     // 系統開始閒置的時間（zero = 非閒置）
```

**新增 setter（不改 constructor 簽名）：**

```go
// SetIdleCheckFn sets the function used to check system idle state.
func (h *HeartbeatMonitor) SetIdleCheckFn(fn func() bool) {
    h.systemIdleCheckFn = fn
}
```

**check() 末尾新增閒置追蹤邏輯：**

```go
// 現有的 task stall 檢查之後（check() 函式結尾）...
if h.systemIdleCheckFn != nil {
    idle := h.systemIdleCheckFn()
    h.idleMu.Lock()
    if idle {
        if h.systemIdleSince.IsZero() {
            h.systemIdleSince = time.Now()
            logDebug("heartbeat: system entered idle state")
        }
    } else {
        if !h.systemIdleSince.IsZero() {
            logDebug("heartbeat: system left idle state",
                "idleDuration", time.Since(h.systemIdleSince).Round(time.Second).String())
        }
        h.systemIdleSince = time.Time{} // reset
    }
    h.idleMu.Unlock()
}
```

**新增查詢方法：**

```go
// SystemIdleDuration returns how long the system has been continuously idle.
// Returns 0 if the system is not idle or idle tracking is not configured.
func (h *HeartbeatMonitor) SystemIdleDuration() time.Duration {
    h.idleMu.RLock()
    defer h.idleMu.RUnlock()
    if h.systemIdleSince.IsZero() {
        return 0
    }
    return time.Since(h.systemIdleSince)
}
```

**HeartbeatStats 新增（health API 可查閒置狀態）：**

```go
// 在 HeartbeatStats struct 中新增：
SystemIdleSince *time.Time `json:"systemIdleSince,omitempty"`
```

**Stats() 方法更新：**

```go
func (h *HeartbeatMonitor) Stats() HeartbeatStats {
    h.mu.Lock()
    s := h.stats
    h.mu.Unlock()

    // Add idle info.
    h.idleMu.RLock()
    if !h.systemIdleSince.IsZero() {
        t := h.systemIdleSince
        s.SystemIdleSince = &t
    }
    h.idleMu.RUnlock()
    return s
}
```

---

### 3. cron.go — 新增 idle trigger 模式

**CronJobConfig 新增欄位：**

```go
// 在 CronJobConfig struct 中新增：
Trigger        string  `json:"trigger,omitempty"`        // "idle" = 閒置觸發模式（不需要 schedule）
IdleMinMinutes int     `json:"idleMinMinutes,omitempty"` // 持續閒置 N 分鐘後觸發（default 30）
CooldownHours  float64 `json:"cooldownHours,omitempty"`  // 兩次觸發之間最小間隔（default 20）
```

**CronEngine 新增欄位：**

```go
// 在 CronEngine struct 中新增：
heartbeatMon *HeartbeatMonitor // 用於查詢閒置狀態
```

**CronEngine 新增 setter：**

```go
// SetHeartbeatMonitor wires the heartbeat monitor for idle-trigger jobs.
func (ce *CronEngine) SetHeartbeatMonitor(h *HeartbeatMonitor) {
    ce.heartbeatMon = h
}
```

**loadJobs() 改動 — idle trigger jobs 不需要 schedule：**

```go
// 在 loadJobs() 裡 parse expr 的邏輯前加：
if jc.Trigger == "idle" {
    // Idle-trigger jobs don't need a cron schedule.
    job := &cronJob{CronJobConfig: jc}
    if jc.TZ != "" {
        if loc, err := time.LoadLocation(jc.TZ); err == nil {
            job.loc = loc
        }
    }
    if job.loc == nil {
        job.loc = time.Local
    }
    jobs = append(jobs, job)
    continue
}
```

**tick() 新增 idle trigger 評估（在現有 `for _, j := range ce.jobs` 迴圈中，cron expression 處理之前）：**

```go
// Idle-trigger jobs: evaluated purely on idle duration, no cron expression.
if j.Trigger == "idle" {
    // Need heartbeat monitor for idle detection.
    if ce.heartbeatMon == nil {
        continue
    }
    // Already running — skip.
    if j.runCount >= j.effectiveMaxConcurrentRuns() {
        continue
    }
    // Cooldown check: default 20 hours between triggers.
    cooldown := j.CooldownHours
    if cooldown <= 0 {
        cooldown = 20
    }
    if !j.lastRun.IsZero() && time.Since(j.lastRun) < time.Duration(cooldown*float64(time.Hour)) {
        continue
    }
    // Idle duration check: default 30 minutes.
    minIdle := j.IdleMinMinutes
    if minIdle <= 0 {
        minIdle = 30
    }
    idleDur := ce.heartbeatMon.SystemIdleDuration()
    if idleDur < time.Duration(minIdle)*time.Minute {
        continue
    }
    // All conditions met — trigger!
    logInfo("cron: idle trigger firing",
        "jobId", j.ID, "name", j.Name,
        "idleMinutes", int(idleDur.Minutes()),
        "threshold", minIdle)

    j.runCount++
    j.running = true
    jobCtx, jobCancel := context.WithCancel(ctx)
    j.cancelFn = jobCancel
    ce.jobWg.Add(1)
    go func(j *cronJob) {
        defer ce.jobWg.Done()
        ce.runJob(jobCtx, j)
    }(j)
    continue
}
```

**startupReplay() — 排除 idle trigger jobs：**

```go
// 在 startupReplay() 的 skip 邏輯中加：
if j.Trigger == "idle" {
    continue
}
```

**CronJobInfo 新增（for API display）：**

```go
// 在 CronJobInfo struct 中新增：
Trigger        string  `json:"trigger,omitempty"`
IdleMinMinutes int     `json:"idleMinMinutes,omitempty"`
CooldownHours  float64 `json:"cooldownHours,omitempty"`
```

在 `ListJobs()` 中填充這些欄位。

---

### 4. dispatch.go — 無改動

`dispatchState.running` 已經有 `len(state.running)` 可查。heartbeat 的 idle check fn 會直接查 `state.running`，不需要改 dispatch.go。

---

### 5. main.go — 接線

在 daemon mode 中，heartbeat monitor 建立之後：

```go
// Agent heartbeat monitor.
var heartbeatMon *HeartbeatMonitor
if cfg.Heartbeat.Enabled {
    heartbeatMon = newHeartbeatMonitor(cfg.Heartbeat, state, notifyFn)

    // Wire idle detection: check dispatched tasks + hook workers + user sessions.
    heartbeatMon.SetIdleCheckFn(func() bool {
        // 1. No running dispatched tasks.
        state.mu.Lock()
        hasRunning := len(state.running) > 0
        state.mu.Unlock()
        if hasRunning {
            return false
        }
        // 2. No active hook workers.
        if hookRecv.HasActiveWorkers() {
            return false
        }
        // 3. No user sessions.
        if countUserSessions(cfg.HistoryDB) > 0 {
            return false
        }
        return true
    })

    go heartbeatMon.Start(ctx)
}
```

在 cron engine 建立之後，wire heartbeat monitor：

```go
// Cron engine.
cron := newCronEngine(cfg, sem, childSem, notifyFn)
if err := cron.loadJobs(); err != nil {
    logWarn("cron load error, continuing without cron", "error", err)
} else {
    registerDailyNotesJob(ctx, cfg, cron)
    // Wire heartbeat for idle-trigger jobs.
    if heartbeatMon != nil {
        cron.SetHeartbeatMonitor(heartbeatMon)
    }
    cron.start(ctx)
}
```

**注意**：main.go 中 heartbeat monitor 的建立在 cron engine 之後（第 633 行），需要把 heartbeat 建立移到 cron 建立之前，或是用 setter 在 heartbeat 建立後回頭 wire 到 cron。

→ **選 setter 方案**（最小改動）：在 heartbeat 建立後加一行 `cron.SetHeartbeatMonitor(heartbeatMon)`。

---

### 6. examples/jobs.example.json — 新增 idle trigger 範例

把現有 self-evolution job 改用 idle trigger：

```json
{
    "id": "self-evolution",
    "name": "Self-Evolution Session",
    "enabled": true,
    "trigger": "idle",
    "idleMinMinutes": 30,
    "cooldownHours": 20,
    "tz": "Asia/Taipei",
    "agent": "ruri",
    "task": {
        "prompt": "You are a self-improvement agent for the Tetora AI orchestration system...",
        "model": "sonnet",
        "timeout": "30m",
        "budget": 1.0,
        "permissionMode": "plan"
    },
    "notify": true,
    "notifyChannel": "notify"
}
```

---

## 實作順序

1. **hooks.go** — `HasActiveWorkers()` 方法（1 分鐘，獨立）
2. **heartbeat.go** — idle tracking 欄位 + setter + check() 尾巴 + `SystemIdleDuration()` + Stats 更新
3. **cron.go** — `Trigger`/`IdleMinMinutes`/`CooldownHours` 欄位 + loadJobs skip + tick() idle 評估 + startupReplay skip + CronJobInfo 更新
4. **main.go** — wire idle check fn + wire heartbeat → cron
5. **examples/jobs.example.json** — 更新範例

## 驗證

- [ ] `go build .` — 編譯通過
- [ ] `go test ./...` — 全部通過
- [ ] Health API (`/healthz`) 可看到 `systemIdleSince` 欄位
- [ ] Cron jobs API 可看到 idle trigger jobs 的 trigger/idleMinMinutes/cooldownHours
- [ ] 日誌確認 idle 狀態轉換（"system entered idle state" / "system left idle state"）
- [ ] `make bump` 部署後觀察實際行為
