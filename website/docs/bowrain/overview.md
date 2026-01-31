---
sidebar_position: 1
title: Overview
---

# Bowrain

Bowrain is a cross-platform desktop application for translating documents. It supports a wide range of file formats, AI-powered translation, translation memory, and live document preview.

## Screenshots

### Dashboard

The project dashboard shows all open projects at a glance with language and file summaries.

![Bowrain dashboard with sample projects](/img/bowrain/dashboard.png)

### Project View

Each project displays its source files with format detection, word counts, and a drop zone for adding new files.

![Project view showing files, stats, and drop zone](/img/bowrain/project-view.png)

### Translation Editor

The editor shows source and target text side by side with a toolbar for translation actions.

![Translation editor with source blocks and target column](/img/bowrain/editor.png)

### Editor with Document Preview

Toggle the split layout to see a live document preview alongside the translation grid. Clicking a segment in either pane selects it in the other.

![Split layout with translation grid and document preview](/img/bowrain/editor-preview.png)

### Settings

Configure AI providers, manage plugins, and view system information from the settings page.

![Settings page with AI provider configuration](/img/bowrain/settings.png)

## Features

- **Translation editor** with inline tag support, tag validation, and document preview
- **AI translation** using Anthropic, OpenAI, or Ollama providers
- **Translation Memory** with fuzzy matching
- **Drag-and-drop flow editor** for building multi-step translation workflows
- **Plugin support** for extending with custom tools
- **Batch file management** with per-file language and format configuration
- **Progress tracking** with real-time progress bars

## Project Format

Bowrain uses the `.kaz` archive format as its native project format. Projects can be opened from the command line:

```bash
bowrain project.kaz
```

Bowrain runs as a single native application on macOS, Windows, and Linux — no additional runtimes or dependencies required.
