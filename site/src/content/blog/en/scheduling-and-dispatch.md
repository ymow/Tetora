---
title: "Tetora's Scheduling System — From Simple Cron to Complex Workflows"
lang: en
date: "2026-04-11"
tag: explainer
readTime: "~6 min"
excerpt: "How Tetora turns a single cron expression into a fully automated, human-gated workflow — and when to use each piece of the scheduling stack."
description: "A complete guide to Tetora's scheduling system: cron jobs, dispatch queues, Human Gate approval checkpoints, and Workflow DAG for complex multi-step automation."
---

Most AI agent tools are interactive — you type a prompt, the agent responds. That's useful for ad-hoc work. But the automation that saves the most time is automation that runs without you.

Tetora has a full scheduling stack built in. This post walks through each layer, when to use it, and how they compose together into something more capable than any individual piece.

## Layer 1: Cron Scheduler

The simplest scheduling primitive is a cron job. You specify when it runs and what to dispatch:

```bash
tetora job add --cron "0 21 * * *" "Run nightly accounting check"
tetora job add --cron "0 9 * * 1" "Weekly team standup summary"
```

Standard cron syntax — five fields for minute, hour, day-of-month, month, day-of-week. If you've used cron before, there's nothing new to learn. If you haven't, there are plenty of cron expression generators online; Tetora accepts any valid cron string.

To see scheduled jobs:

```bash
tetora job list
```

To remove one:

```bash
tetora job remove <job-id>
```

When a cron job fires, it creates a task in the dispatch queue and returns immediately. The cron scheduler doesn't wait for the task to complete — it just ensures the task gets created on time.

## Layer 2: Dispatch Queue

The dispatch queue is the execution layer. Tasks enter the queue — from cron jobs, from manual `tetora dispatch` calls, from workflow steps, from webhook triggers — and agents pick them up.

The queue is configurable for concurrency:

```json
{
  "dispatch": {
    "maxSlots": 4,
    "slotPressureThreshold": 0.8
  }
}
```

`maxSlots` controls how many tasks run simultaneously. When all slots are full and a new task arrives, it waits in the queue rather than force-starting and competing for resources. `slotPressureThreshold` adds an additional buffer: once slot utilization exceeds this fraction, new tasks queue even before all slots are technically full.

For most personal setups, `maxSlots: 2` or `3` is reasonable — enough to run parallel tasks without overwhelming local resources.

You can inspect the queue at any time:

```bash
tetora status
```

And review execution history:

```bash
tetora history fails          # Show recent failures
tetora history trace <task-id>  # Full trace for a specific task
```

## Layer 3: Human Gate

Some tasks don't belong in a fully automated pipeline. Not because they're complex, but because they're consequential — actions that are hard to reverse, that affect real external systems, or that require judgment that the agent shouldn't be trusted to make unilaterally.

Human Gate adds an approval checkpoint to any workflow step. When the agent reaches that step, it pauses, notifies you, and waits for explicit approval before continuing.

```json
{
  "humanGate": {
    "enabled": true,
    "timeoutHours": 4,
    "notifyDiscord": true
  }
}
```

`timeoutHours` controls how long the agent waits before escalating or abandoning the step. `notifyDiscord` sends a message to your configured Discord channel when approval is needed — useful for workflows that run while you're away from the keyboard.

In a workflow YAML, you mark a step as gated with `humanGate: true`:

```yaml
- id: review-uncertain
  humanGate: true
  run: "Flag transactions with confidence < 0.8 for human review"
  depends: [classify]
```

The agent reaches this step, surfaces the low-confidence transactions for your review, and waits. When you approve (via CLI or Discord), it continues to the next step. If you reject, the workflow stops there and logs the outcome.

Human Gate doesn't make automation less useful. It makes it safe to use in contexts where full autonomy would be inappropriate.

## Layer 4: Workflow DAG

Simple cron tasks work well when each task is self-contained. For multi-step processes with dependencies between steps, Workflow DAG lets you define the full pipeline declaratively.

Here's a practical example — a nightly accounting workflow based on the pattern used by CPA Kento Hatakeyama:

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

Execution flows through the DAG: `fetch-transactions` runs first, then `classify` (which depends on the fetch being done), then `review-uncertain` (which pauses for human approval), then `post-entries` (which runs only after approval is received).

If `fetch-transactions` fails, nothing downstream runs. If `review-uncertain` times out without approval, `post-entries` never executes. The DAG structure makes failure modes explicit and traceable.

Steps without a `depends` field can run in parallel if they're ready at the same time. For a workflow that fetches data from multiple independent sources before a merge step, this means the fetches run concurrently, reducing total wall time.

### Conditional Routing

Steps can include a `condition` field — the step only executes if the condition evaluates to true:

```yaml
- id: send-alert
  run: "Send Slack alert about anomalous transactions"
  condition: "{{classify.anomaly_count}} > 0"
  depends: [classify]
```

If the classification step found no anomalies, the alert step is skipped entirely. The pipeline adapts to the data without requiring a separate trigger mechanism.

## Putting It Together

These layers compose cleanly:

- **Cron** fires a job every night at 21:00
- The job dispatches a **Workflow** task
- The workflow runs several automated steps through the **dispatch queue**
- At the critical write-back step, **Human Gate** pauses for approval
- The rest of the workflow completes after confirmation

The result is a pipeline that runs reliably on a schedule, handles errors predictably, parallelizes where possible, and escalates the judgment calls that shouldn't be automated.

This is the actual setup that Hatakeyama built manually over months — Tetora provides it as a composable system with no custom infrastructure required.

## The Design Philosophy

The goal of Tetora's scheduling system is to match the right control level to the right action.

Fetching data from an API? Fully automated. Classifying transactions by keyword? Fully automated. Posting entries that affect a client's financial records? Human Gate. Sending a report to a client? Human Gate.

The judgment about which category an action falls into belongs to you — the person who understands your business context and risk tolerance. Tetora gives you the mechanism to encode that judgment once and have it enforced consistently across every run.

**Automate the routine. Escalate the judgment.** The scheduling stack is how that principle becomes operational.
