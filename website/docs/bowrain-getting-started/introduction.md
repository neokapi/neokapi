---
sidebar_position: 1
title: Introduction
slug: /bowrain/introduction
---

# Bowrain Platform

Bowrain is the full-stack localization platform built on the gokapi framework. It provides a visual translation editor, project management, team collaboration, and automation — available as a web app, desktop app, and REST server.

## What is Bowrain?

Bowrain brings the power of gokapi's processing engine to teams:

- **Bowrain CLI** — project companion CLI that syncs local files with Bowrain Server (like git for translations)
- **Bowrain Web** — browser-based translation editor with split preview, TM, and terminology
- **Bowrain Desktop** — native cross-platform app with offline support
- **Bowrain Server** — REST API server with workspaces, connectors, automation, and content store

## How It Fits Together

```
Developer (Bowrain CLI)          Translator (Web/Desktop)
     |                              |
     |  brain push                  |  Open editor
     |-------------->               |-------------->
     |               Bowrain Server |
     |<--------------               |<--------------
     |  brain pull                  |  Save translations
```

The developer initializes a `.bowrain/` project, pushes source content to the server, and pulls back translations. Translators work in the web app or desktop app with a visual editor, translation memory, and terminology support.

## Key Features

- **Project model** — `.bowrain/` directories (like `.git/`) manage localization projects
- **Push/pull sync** — Content-addressed incremental sync (only changed blocks transfer)
- **Visual editor** — Split preview, focus view, and grid view for translation
- **Translation memory** — Built-in Sievepen TM with fuzzy matching
- **Terminology** — Concept-oriented termbase with pipeline enforcement
- **AI translation** — LLM-powered translation with Anthropic, OpenAI, Ollama
- **MT services** — DeepL, Google, Microsoft, ModernMT, MyMemory
- **Connectors** — Bidirectional sync with CMS, code repos, and design tools
- **Automation** — Event-driven triggers, quality gates, and webhooks
- **Workspaces** — Multi-tenant team collaboration with role-based access

## Getting Started

- [Installation](/docs/bowrain/installation) — install Bowrain CLI, Bowrain Desktop, or Bowrain Server
- [Quick Start](/docs/bowrain/quickstart) — initialize a project and sync with Bowrain
- [Project Walkthrough](/docs/bowrain/project-walkthrough) — deep dive into the `.bowrain/` project model

## Standalone File Processing

For standalone file processing without a server (format conversion, pseudo-translation, word counting, QA checks), use the [kapi CLI](/docs/getting-started/introduction) instead. Kapi operates directly on files without requiring a project or server.
