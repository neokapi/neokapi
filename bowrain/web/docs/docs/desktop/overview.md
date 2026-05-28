---
sidebar_position: 1
title: Overview
---

# Bowrain Desktop

Bowrain Desktop is a cross-platform desktop editor for translating documents. It connects to a Bowrain server for collaborative, governed work, and supports a wide range of file formats, AI-assisted translation, translation memory, and live document preview.

## Screenshots

### Dashboard

The project dashboard shows all open projects at a glance with language and file summaries.

![Bowrain dashboard with sample projects](/img/bowrain/dark/dashboard.png)

### Project View

Each project displays its source files with format detection, word counts, and a drop zone for adding new files.

![Project view showing files, stats, and drop zone](/img/bowrain/dark/project-view.png)

### Translation Editor

The editor shows source and target text side by side with a toolbar for translation actions. Four layout modes are available: Grid (table view), Focus (single-block editing), Split Horizontal, and Split Vertical (with live preview). Block status is visualized with a color-coded progress bar showing not-started, draft, translated, and reviewed states.

![Translation editor with source blocks and target column](/img/bowrain/dark/editor.png)

### Editor with Document Preview

Toggle the split layout to see a live document preview alongside the translation grid. Clicking a segment in either pane selects it in the other.

![Split layout with translation grid and document preview](/img/bowrain/dark/editor-preview.png)

### Editor Focus View

Focus mode provides single-block deep editing with full-width source and target panels and block navigation (previous/next). Use it for detailed editing of individual translation segments.

![Focus view with single-block source and target panels](/img/bowrain/dark/editor-focus.png)

### Flow Editor

The visual flow editor provides a drag-and-drop workflow builder powered by React Flow. Create multi-step translation workflows by connecting reader, tool, and writer nodes on an interactive canvas. Five built-in flow templates are included: AI Translate, AI Translate + QA, Pseudo Translate, QA Check, and TM Leverage. User-created flows are saved and can be reused across projects.

![Flow editor with connected reader, tool, and writer nodes](/img/bowrain/dark/flow-editor.png)

### Settings

Configure AI providers, manage plugins, and view system information from the settings page.

![Settings page with AI provider configuration](/img/bowrain/dark/settings.png)

### Context Panel

In the translation editor, the Context panel provides instant per-block linguistic resources. TM matches show source, target, score, and match type with one-click apply. Terminology matches show source terms, target suggestions, domain, and lifecycle status. The panel updates automatically as you navigate between blocks.

![Context panel showing TM matches and terminology](pathname:///img/bowrain/dark/context-panel.png)

### Terminology Explorer

The Terminology panel provides full concept-oriented term management. Browse, search, add, edit, and delete concepts with multi-locale terms. Import from CSV or JSON, export the full termbase. Each term has a lifecycle status (preferred, approved, admitted, deprecated, proposed, forbidden) and domain classification.

![Terminology explorer with concept browser](pathname:///img/bowrain/dark/term-explorer.png)

### Translation Memory Explorer

The TM Explorer provides full access to the translation memory. Browse, search, add, edit, and delete entries. Import and export TMX files. Each entry preserves inline markup through the content-aware matching system.

![TM Explorer with entry browser](pathname:///img/bowrain/dark/tm-explorer-full.png)

## Features

- **Translation editor** with four layout modes (grid, focus, split-h, split-v), inline tag support, block status tracking, and live document preview
- **Context panel** with per-block TM matches and terminology suggestions, one-click apply
- **Terminology management** with concept-oriented termbases, multi-locale terms, lifecycle statuses, CSV/JSON import/export
- **Translation Memory** with content-aware tiered matching (generalized, structural, plain), fuzzy matching, and TM explorer
- **Flow editor** with drag-and-drop visual workflow builder and built-in flow templates
- **AI translation** using Anthropic, OpenAI, Google Gemini, or Ollama providers, with streaming progress for live thinking updates
- **Action tools** including segmentation, QA check, TM leverage, term lookup, and term enforcement
- **Plugin support** for extending with custom formats and tools
- **Batch file management** with per-file language and format configuration
- **Progress tracking** with status-colored progress bars
- **Sample projects** included for immediate testing and evaluation

## Project Format

Bowrain stores projects in a local SQLite database backed by the Content Store. Each project contains:

- **Source documents** in their original formats
- **Translation blocks** with per-locale target segments
- **Translation memory** entries
- **Terminology** concepts
- **Preview HTML** for live document preview

TM and terminology are saved automatically when you save the project and restored when you open it. This means each project carries its own linguistic resources — no external database setup required.

Bowrain runs as a single native application on macOS, Windows, and Linux — no additional runtimes or dependencies required.

## Sample Projects

Bowrain ships with sample projects for immediate testing:

| Project             | Content                        | Status                         | Use Case                                            |
| ------------------- | ------------------------------ | ------------------------------ | --------------------------------------------------- |
| Website Translation | Corporate website (HTML)       | Half-translated (en→fr,de)     | TM leverage demo — auto-fill translations           |
| Software UI         | Task manager UI strings (JSON) | New, with 27-entry TM          | Start with existing TM, translate remaining strings |
| Marketing Content   | Marketing landing page (HTML)  | Fully translated (en→fr,de,es) | Review and export workflows                         |

Sample files are located in `bowrain/apps/bowrain/samples/`. Each project includes its own TM entries and termbase concepts.
