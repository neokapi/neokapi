---
sidebar_position: 1
title: Introduction
slug: /
---

# Bowrain Platform

Bowrain is a full-stack localization platform that turns your source content into production-ready translations. It combines a CLI for developers, a visual editor for translators, and a server for automation — all powered by the neokapi framework's 41+ format readers and 46+ processing tools.

## One Command, Full Cycle

```bash
kapi sync
```

Push your source content, wait for AI translation and QA, pull back the results. One command, zero context switching. Under the hood, `kapi sync` runs three phases (push, wait-for-translation, pull) so you can go from code change to translated files in a single terminal invocation.

## Five Pillars

### 1. One Command, Full Cycle — `kapi sync`

The [`kapi sync`](/cli/commands/sync) command pushes changed content to the server, waits for all triggered flows (translation, QA, terminology) to complete, and pulls back the results. In CI, this means a single step replaces a multi-job pipeline. Locally, it means you type one command and get translations back.

### 2. Format Intelligence — 41+ Formats

Bowrain understands your content structure. Whether it is JSON, XLIFF, Markdown, YAML, PO, HTML, OpenXML, InDesign IDML, SRT subtitles, or any of 41+ supported formats, the platform extracts translatable blocks while preserving structure, inline formatting, and metadata. No manual configuration of what to translate — the format reader handles it.

### 3. Composable Automation

Define automation rules at the top level of your `.kapi` recipe for CLI-driven workflows:

```yaml
automations:
  - name: qa-before-push
    trigger: pre-push
    actions:
      - type: run_flow
        config:
          flow: qa-check
```

Or use the visual rule editor on Bowrain Server for event-driven automation — trigger flows when content arrives, quality gates pass, or connectors sync. Local rules and server rules complement each other: local for developer guardrails, server for team-wide workflows.

### 4. Source-First Quality

Catch issues in source content before they multiply across target languages. QA checks, terminology enforcement, consistency validation, and pattern matching run on source strings at push time. A misspelled term fixed once in the source prevents the same error in every target locale.

### 5. AI + Human-in-the-Loop

LLM-powered translation (Anthropic, OpenAI, Ollama) and five MT services (DeepL, Google, Microsoft, ModernMT, MyMemory) produce initial translations. The visual editor gives translators split preview, translation memory, and terminology context to review and refine. AI handles volume; humans ensure quality.

## How It Fits Together

```
Developer (Bowrain CLI)          Translator (Web/Desktop)
     |                              |
     |  kapi sync                |  Open editor
     |----------------------------->|----------------------------->
     |               Bowrain Server |
     |  push → translate → pull     |  Review, approve, save
     |<-----------------------------|<-----------------------------
```

The developer initializes a `.kapi` project, runs `kapi sync` to push source content and pull back translations. Translators work in the web app or desktop app with a visual editor, translation memory, and terminology support. Automation rules on the server orchestrate the processing between push and pull.

## Components

- **Bowrain CLI** — project companion CLI that syncs local files with Bowrain Server
- **Bowrain Web** — browser-based translation editor with split preview, TM, and terminology
- **Bowrain Desktop** — native cross-platform app with offline support
- **Bowrain Server** — REST API server with workspaces, connectors, automation, and content store

## What's Next

- [Installation](/installation) — install Bowrain CLI, Bowrain Desktop, or Bowrain Server
- [Quick Start](/quickstart) — initialize a project and sync with Bowrain
- [Walkthrough](/walkthroughs/bowrain-getting-started) — complete guide from init to automated CI/CD
- [`kapi sync` command](/cli/commands/sync) — the flagship one-command workflow

## Standalone File Processing

For standalone file processing without a server (format conversion, pseudo-translation, word counting, QA checks), use the [kapi CLI](https://neokapi.github.io/web/neokapi/docs/getting-started/introduction) instead. Kapi operates directly on files without requiring a project or server.
