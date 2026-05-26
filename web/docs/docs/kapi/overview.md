---
sidebar_position: 0
title: Kapi
description: Kapi keeps content on-brand and terminologically consistent, then localizes it into every language and format. It runs the neokapi engine two ways — the kapi command-line tool and the Kapi Desktop app — sharing the same tools, flows, and .kapi projects.
keywords: [Kapi, kapi CLI, Kapi Desktop, localization, brand guardrails, overview]
---

# Kapi

**Kapi** keeps content **on-brand and terminologically consistent**, then
**localizes it into every language and format**. It is built on the open-source
[neokapi framework](/framework/architecture) and comes in two forms that share
the same engine, tools, flows, and [`.kapi` projects](/kapi/projects):

- **[Kapi CLI](/kapi/cli)** — a single binary that operates directly on files.
  Best for automation, scripting, CI gates, and offline work; no project or
  server required. It also serves brand and terminology tools to your AI
  assistant over [MCP](/reference/mcp).
- **[Kapi Desktop](/kapi/desktop/overview)** — a visual companion app for
  building flows, running tools, managing plugins, and storing AI credentials —
  without writing YAML or remembering flags.

Because both front-ends run the same engine, a workflow built in one works in
the other: the flows and the project file are identical.

## Command-line interface

Run tools and flows straight from the terminal — `kapi pseudo-translate`,
`kapi ai-translate`, `kapi brand check`, `kapi run <flow>`. See the
[Kapi CLI overview](/kapi/cli) for the command surface, and the
[recipes](/kapi/recipes) for task-by-task walkthroughs you can run in the browser.

## Desktop app

Prefer a visual interface? [Kapi Desktop](/kapi/desktop/overview) wraps the same
tools and flows in a native app with a flow editor, live runner, plugin manager,
and credential vault. Start with
[your first project](/kapi/desktop/getting-started).

## Projects vs. ad-hoc

Either front-end can run **ad-hoc** — configured entirely by flags or clicks —
or against a saved **[`.kapi` project](/kapi/projects)** that captures languages,
content patterns, flows, and defaults, portable and shareable via git.

## Install

See [Installation](/kapi/get-started/installation) for the CLI (Homebrew or download)
and Kapi Desktop (macOS cask).
