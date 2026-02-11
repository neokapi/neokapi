---
sidebar_position: 2
title: Getting Started
---

# Getting Started

This guide walks you through first login, workspace creation, and translating your first file in the gokapi web application.

## Prerequisites

You need a running gokapi server. Choose one of:

- **Local mode**: Run `kapi serve` for single-user, no-auth local development
- **Server mode**: Deploy `gokapi-server` with Docker Compose (see [Self-Hosting](../user-guide/self-hosting.md))

## Starting the Server

### Local Mode

```bash
kapi serve
```

Open `http://localhost:8080` in your browser. No authentication is required — you are automatically signed in as a local user with a pre-created workspace.

### Server Mode

```bash
docker compose up -d
```

Open `http://localhost:8080` (or your configured domain) in your browser.

## Logging In (Server Mode)

The web UI redirects you to your configured identity provider (Dex) for authentication. Depending on your Dex configuration, you will see options like:

- **GitHub** — sign in with your GitHub account
- **Google** — sign in with your Google workspace account
- **LDAP** — sign in with your corporate credentials

After authenticating, you are redirected back to the gokapi web UI with an active session.

## Creating a Workspace

After your first login in server mode, create a workspace to organize your translation projects:

1. Click the **+** button in the workspace rail (left edge of the screen)
2. Enter a **Name** (e.g., "My Team") — the slug is auto-generated
3. Adjust the **Slug** if needed (URL-safe identifier)
4. Click **Create**

You are automatically added as the workspace **owner** and switched to the new workspace.

In local mode, a workspace named "Local" is created automatically.

## Creating a Project

1. From the **Translate** view (the default), click **New Project**
2. Enter the **Project name** (e.g., "Website Translation")
3. Select the **Source language** (e.g., English)
4. Select one or more **Target languages** (e.g., French, German, Japanese)
5. Click **Create**

The project opens in the project view.

## Uploading Files

1. **Drag and drop** files onto the upload zone in the project view, or click **Add Files** to browse
2. The server auto-detects the file format (HTML, XML, JSON, YAML, PO, Markdown, XLIFF, and more)
3. Files appear in the file list with format icon, block count, and word count

Supported formats include all formats registered in the gokapi format registry. See [Formats](../user-guide/formats.md) for the complete list.

## Opening the Editor

Click any file name in the project view to open it in the translation editor. The editor loads all translatable blocks from the file and displays them in a source/target grid.

## Translating

### Manual Translation

1. Click a target cell or press **Enter** on the selected row to start editing
2. Type the translation
3. Press **Enter** to save and advance to the next block, or **Escape** to cancel

### AI Translation

1. Configure an AI provider in the backend (Anthropic, OpenAI, or Ollama)
2. Click **AI Translate** in the toolbar to translate the entire file
3. Review and edit the AI-generated translations as needed

### Machine Translation

1. Configure an MT provider (DeepL, Google, Microsoft, ModernMT, or MyMemory)
2. Select the provider from the toolbar dropdown
3. Click **AI Translate** to translate the file using the selected MT engine

### TM Leverage

1. If translation memory entries exist, click **TM Lookup** in the toolbar
2. The system matches source blocks against the TM and fills in matches
3. High-confidence matches are applied automatically; review others in the context panel

### Pseudo-Translation

Click **Pseudo** in the toolbar to generate pseudo-translations — useful for testing layout and character handling before starting real translation.

## Exporting

Click the **Export** button in the toolbar to download the translated file in its original format with all translations applied.

## What's Next

- [Translation Editor](./translation-editor.md) — layout modes, toolbar, keyboard shortcuts, context panel
- [Translation Memory](./translation-memory.md) — TM Explorer features
- [Terminology](./terminology.md) — term management and enforcement
- [Walkthroughs](./walkthroughs.md) — step-by-step workflows
