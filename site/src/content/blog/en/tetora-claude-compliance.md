---
title: "Why Tetora Uses the Official Claude API — and Why It Matters"
lang: en
date: "2026-04-10"
tag: explainer
readTime: "~4 min"
excerpt: "Anthropic's April 2026 enforcement clarified what's allowed with Claude subscriptions. Here's exactly how Tetora integrates with Claude — and why we won't get blocked."
description: "Tetora integrates with Claude via the official claude-code CLI or direct API keys — not OAuth subscription tokens. Here's the technical detail and why it matters for reliability."
---

On February 20, 2026, Anthropic updated its usage policies with a clarification that affected a large portion of the third-party Claude tooling ecosystem: **subscription OAuth tokens cannot be used in third-party applications**. The policy had always implied this; the update made it explicit.

On April 4, Anthropic enforced the policy against OpenClaw, blocking the tool entirely. For every user of that tool, service stopped the same day.

This post explains how Tetora integrates with Claude, exactly, and why that design choice means Tetora is not affected by this enforcement.

## The Two Permitted Integration Paths

Anthropic's permitted use policy specifies two ways to build applications that use Claude:

1. **The official `claude-code` CLI** — a first-party command-line interface published by Anthropic
2. **The Anthropic API with API keys** — direct HTTP calls authenticated with a user-generated API key from console.anthropic.com

Both of these are stable, explicitly supported, and subject to Anthropic's standard API terms. When you use either path, your billing relationship with Anthropic is direct and transparent — you pay for what you use.

What is **not** permitted is using the OAuth token that backs a Claude.ai subscription in a third-party tool. That token authenticates the web session at claude.ai; using it to power external tools was never authorized, even if it was technically possible.

## How Tetora Is Configured

Tetora's integration method is set by a single config field: `claudeProvider`.

For users who have the `claude-code` CLI installed:

```json
{
  "claudeProvider": "claude-code"
}
```

Tetora invokes `claude-code` as a subprocess, the same way you would use it from the terminal. The CLI manages its own authentication, session handling, and communication with Anthropic's servers. Tetora never touches your authentication credentials directly.

For users who prefer direct API access:

```json
{
  "claudeProvider": "anthropic"
}
```

In this mode, Tetora calls the Anthropic API directly using an API key stored in your local Tetora config (or provided via environment variable). The key is issued from your Anthropic console, tied to your account's API billing. You set usage limits, monitor costs, and rotate keys whenever you want — through Anthropic's own tooling.

Neither integration path uses subscription OAuth tokens. Neither path bypasses Anthropic's billing or access controls. Both are exactly the integration patterns Anthropic supports.

## Why This Matters for Reliability

When you build automation on Tetora, you're building on a foundation with two important properties:

**No enforcement risk.** Tetora is not doing anything that violates Anthropic's policies. There is no scenario analogous to what happened to OpenClaw — where a policy enforcement action shuts down your workflows overnight.

**Predictable costs.** API key usage is metered and billed through your Anthropic account. You can set hard spending limits, get alerts at thresholds, and see per-request cost breakdowns in the Anthropic console. Subscription usage doesn't provide this granularity — you pay a flat rate and hope it covers your usage. For automation that runs on a schedule or in response to events, metered billing maps much better to actual consumption patterns.

**Portability.** An API key works the same whether you're running Tetora on your laptop, a home server, or a remote VM. There's no account session to maintain, no browser to keep authenticated, no periodic re-auth flows. A key stays valid until you rotate it.

## The Broader Point About Compliance

There's a temptation to treat compliance as a constraint — a set of rules that limits what you can build. The OpenClaw situation illustrates the opposite framing: compliance is load-bearing infrastructure.

When a tool builds on an unauthorized integration path, it's not just breaking a rule in the abstract. It's building user workflows on top of a dependency that can be terminated at any time, by a single enforcement decision outside the tool's control. Every user of that tool is exposed to that risk, whether they understand it or not.

Tetora's compliance with Anthropic's API terms isn't a marketing point — it's a design requirement. Automation is only useful if it keeps running.

## Checking Your Setup

If you're currently on `claude-code` mode, you can verify your setup with:

```bash
tetora config show | grep claudeProvider
```

If you want to switch to direct API mode, update your config and provide your key:

```bash
tetora config set claudeProvider anthropic
tetora config set anthropicApiKey sk-ant-...
```

Your API key can also be set via the `ANTHROPIC_API_KEY` environment variable, which Tetora reads automatically if the config field is not set.

Both modes are fully supported. The right choice depends on your preference: `claude-code` is simpler to set up if you already use the CLI; `anthropic` gives you more direct cost visibility and control.
