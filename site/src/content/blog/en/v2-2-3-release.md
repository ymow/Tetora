---
title: "Tetora v2.2.3–v2.2.4 — Model Picker, TTS, Human Gate"
lang: en
date: "2026-04-04"
tag: release
readTime: "~5 min"
excerpt: "Interactive model switcher, VibeVoice TTS, Human Gate improvements, Skill AllowedTools, and the Discord !model command."
description: "Tetora v2.2.3 adds interactive model switching (Discord + Dashboard), VibeVoice TTS, Human Gate retry/cancel/notifications, Skill AllowedTools, and learned skill auto-extraction."
---

v2.2.3 and v2.2.4 arrive together with a focused set of improvements: interactive model switching across Discord and the Dashboard, local and cloud TTS via VibeVoice, a more capable Human Gate, per-skill tool restrictions, and automatic skill extraction from session history. v2.2.4 follows up with bug fixes and infrastructure hardening.

> **TL;DR:** `!model pick` lets you switch providers and models interactively from Discord. VibeVoice brings local TTS with a cloud fallback. Human Gate now supports retry, cancel, and Discord notifications. `allowedTools` restricts which tools a Skill can call. Learned skills are extracted automatically from session history.

## Model Switching

### Discord Commands

Switch your inference model without touching a config file. Three new commands are available in any channel where Tetora is active:

**`!model pick`** — Opens an interactive picker with a three-step flow:

```
Step 1: Select provider  →  Step 2: Select model  →  Step 3: Confirm
```

Each step is presented as a Discord message with numbered options. Type the number to advance to the next step.

**`!local` / `!cloud`** — Bulk toggle the inference mode for all agents at once. `!local` switches every agent to the configured local provider (Ollama, LM Studio, etc.). `!cloud` switches back to the cloud provider.

**`!mode`** — Prints a summary of the current inference configuration: active provider, model, and global mode.

### Dashboard Model Picker

The Dashboard now surfaces model configuration directly on agent cards:

- **Provider bar** at the top of each agent card shows the active provider with a color-coded badge (Cloud in blue, Local in green)
- **Model dropdown** on each agent card — click to switch the model for that specific agent without navigating to Settings
- **Global inference mode toggle** — a single switch in the header bar to flip all agents between Cloud and Local at once

### Claude Provider Config

A new `claudeProvider` field in `config.json` controls how Tetora invokes the Claude model:

```json
{
  "claudeProvider": "claude-code"
}
```

- `"claude-code"` — Invokes Claude via the Claude Code CLI. Default for local installations with an active Claude subscription.
- `"anthropic"` — Calls the Anthropic API directly using an `ANTHROPIC_API_KEY`. Default when running headless or in CI.

The field can be set per-installation, so a local dev machine and a remote server can use different invocation paths without config conflicts.

## VibeVoice TTS

Tetora can now speak. VibeVoice integration brings text-to-speech output to agent responses, with a two-tier fallback chain:

1. **Local VibeVoice** — runs on-device, zero latency after model load, full privacy
2. **fal.ai cloud TTS** — used automatically if local VibeVoice is unavailable or fails

Configure in `config.json`:

```json
{
  "tts": {
    "enabled": true,
    "provider": "vibevoice",
    "fallback": "fal"
  }
}
```

TTS is off by default. When enabled, agents speak their responses in Discord voice channels and in the Dashboard's monitor view.

## Human Gate Improvements

Human Gate — Tetora's mechanism for pausing agent execution and requesting human approval — received significant quality-of-life updates.

### Retry and Cancel

Reviewers can now act on previously rejected gates without manual intervention:

- **Retry API** — `POST /api/gate/:id/retry` re-queues the gate for review, resetting its state to `waiting`
- **Cancel API** — `POST /api/gate/:id/cancel` terminates the paused task cleanly
- Both actions are surfaced in the Dashboard's Task Detail modal alongside the existing Approve/Reject buttons

### Discord Notifications

Human Gate events now trigger Discord messages in the configured notification channel:

- **Waiting** — notifies reviewers when a gate opens and is waiting for approval
- **Timeout** — alerts the channel if a gate expires without action, including which task was affected
- **Assignee mentions** — if a gate has an assigned reviewer, that user is `@mentioned` directly in the notification

### Unified Action Fields

The gate event schema consolidates approval data into two fields:

```json
{
  "action": "approve | reject | retry | cancel",
  "decision": "approved | rejected"
}
```

This replaces the previous mix of `approved`, `rejected`, and `action` fields. The old fields remain readable for one release cycle before removal.

## Skill AllowedTools

Skills now support a tool restriction list. Set `allowedTools` in a Skill's config to limit which MCP tools that Skill can invoke:

```json
{
  "name": "freee-check",
  "allowedTools": ["mcp__freee__list_transactions", "mcp__freee__get_company"],
  "prompt": "Check unprocessed entries for all companies."
}
```

When `allowedTools` is set, the Skill runs in a sandboxed context where other tools — including shell commands, file system access, and any MCP tools not in the list — are unavailable. This enforces least-privilege at the Skill level and makes audit trails cleaner.

## Learned Skill Auto-Extraction

Tetora now automatically identifies reusable patterns in session history and proposes them as new Skills.

After a session ends, a background process scans the conversation for repeated command sequences and multi-step patterns. Candidates are written to `skills/learned/` with a `SKILL.md` and `metadata.json`, and flagged as `approved: false` until reviewed.

Review proposed skills from the CLI:

```bash
tetora skill list --pending      # show proposed skills awaiting review
tetora skill approve <name>      # promote to active
tetora skill reject <name>       # discard the proposal
```

Approved skills become available as slash commands immediately.

## v2.2.4 Fixes

v2.2.4 is a stabilization release. Key fixes:

- **i18n URL deduplication** — Fixed a routing bug where locale prefixes were doubled in generated URLs (e.g., `/en/en/blog/...` → `/en/blog/...`).
- **Skills cache RWMutex** — Replaced a plain mutex with a read-write mutex on the skills cache, improving throughput for read-heavy workloads.
- **SEO improvements** — Added `BreadcrumbList` structured data and correct `og:locale` values to all blog and docs pages.
- **Regression guard tests** — Added integration tests covering the i18n URL dedup fix and the skills cache to prevent regressions.

## Upgrade

```bash
tetora upgrade
```

Single binary. No external dependencies. macOS / Linux / Windows.

[View full changelog on GitHub](https://github.com/TakumaLee/Tetora/releases/tag/v2.2.4)
