---
sidebar_position: 2
title: Getting Started
---

# Getting Started with Bowrain

## Installation

### macOS (Homebrew)

```bash
brew install --cask neokapi/tap/bowrain
```

### Binary Download

Download the latest release from [GitHub Releases](https://github.com/neokapi/neokapi/releases):

- **macOS**: DMG (Apple Silicon)
- **Windows**: signed installer (`bowrain-X.Y.Z-windows-amd64-setup.exe` or `-arm64-setup.exe`); a portable `.zip` is also published
- **Linux**: tarball (amd64, arm64)

## First Project

1. Launch Bowrain
2. Create a new project or open a sample project
3. Add source files to the project
4. Configure source and target languages
5. Run a translation flow or edit translations manually

![Editor showing 100% translated blocks](/img/bowrain/dark/editor-translated.png)

### Quick Start with Sample Projects

Bowrain ships with ready-to-use sample projects in `bowrain/apps/bowrain/samples/`:

1. Open the Website Translation sample project — a half-translated website with TM entries and terminology
2. Click on `index.html` to open the translation editor
3. Click **"TM Lookup"** in the toolbar to auto-fill blocks from translation memory
4. Click **"Context"** to open the side panel showing TM matches and terminology per block
5. Navigate blocks and click **"Apply"** on TM matches to insert translations
6. Go back to project view and click **"Terminology"** to browse the termbase
7. Click **"Save"** — TM and terminology are persisted in the project database

## Editor Layout Modes

The translation editor supports four layout modes, accessible from the toolbar:

- **Grid**: Default table view showing all blocks with source and target columns
- **Focus**: Single-block editing with full-width source and target panels. Use toolbar navigation to move between untranslated blocks.
- **Split Horizontal**: Block grid on top, live document preview on bottom
- **Split Vertical**: Block grid on left, live document preview on right

## Block Status

Each translation block has a status that is automatically tracked:

| Status      | Indicator | Condition                                 |
| ----------- | --------- | ----------------------------------------- |
| Not Started | Gray      | No target text                            |
| Draft       | Yellow    | Has target text but no translation origin |
| Translated  | Blue      | Translation origin is set (AI, TM, etc.)  |
| Reviewed    | Green     | Manually marked as reviewed               |

The progress bar at the top of the editor shows the distribution of block statuses.

## Keyboard Shortcuts

| Shortcut               | Action                               |
| ---------------------- | ------------------------------------ |
| `Cmd/Ctrl+S`           | Save project                         |
| `Cmd/Ctrl+O`           | Open project                         |
| `Cmd/Ctrl+Enter`       | Confirm translation and move to next |
| `Cmd/Ctrl+Shift+Enter` | Copy source to target                |
| `Cmd/Ctrl+Shift+R`     | Mark block as reviewed               |

## Using Translation Memory

Each project has its own TM that persists in the project database:

1. **TM Explorer**: From the project view, click "Translation Memory" to browse, search, add, edit, or delete entries
2. **TM Lookup**: In the editor toolbar, click "TM Lookup" to batch-apply TM matches to all untranslated blocks
3. **Context Panel**: Click "Context" in the editor toolbar to see per-block TM matches with score, source, target, and match type. Click "Apply" to insert a match.

TM entries are automatically saved when you save the project.

## Using Terminology

Each project has its own concept-oriented termbase:

1. **Terminology Explorer**: From the project view, click "Terminology" to browse, search, add, edit, or delete concepts
2. **Import Terms**: In the Terminology panel, import terms from CSV files (source/target pairs) or full JSON termbases
3. **Context Panel**: In the editor, the Context panel shows terminology matches for the current block's source text — matched terms, target suggestions, domain, and lifecycle status

Terminology concepts support multiple terms per locale with lifecycle statuses: preferred, approved, admitted, deprecated, proposed, forbidden.
