---
sidebar_position: 2
title: Getting Started
---

# Getting Started with Bowrain

## Installation

### macOS (Homebrew)

```bash
brew install --cask gokapi/tap/bowrain
```

### Binary Download

Download the latest release from [GitHub Releases](https://github.com/gokapi/gokapi/releases):

- **macOS**: Universal DMG (Intel + Apple Silicon)
- **Windows**: ZIP archive
- **Linux**: Tarball (amd64)

## First Project

1. Launch Bowrain
2. Create a new project or open an existing `.kaz` archive
3. Add source files to the project
4. Configure source and target languages
5. Run a translation flow or edit translations manually

![Editor showing 100% translated blocks](/img/bowrain/editor-translated.png)

## Editor Layout Modes

The translation editor supports four layout modes, accessible from the toolbar:

- **Grid**: Default table view showing all blocks with source and target columns
- **Focus**: Single-block editing with full-width source and target panels. Use toolbar navigation to move between untranslated blocks.
- **Split Horizontal**: Block grid on top, live document preview on bottom
- **Split Vertical**: Block grid on left, live document preview on right

## Block Status

Each translation block has a status that is automatically tracked:

| Status | Indicator | Condition |
|--------|-----------|-----------|
| Not Started | Gray | No target text |
| Draft | Yellow | Has target text but no translation origin |
| Translated | Blue | Translation origin is set (AI, TM, etc.) |
| Reviewed | Green | Manually marked as reviewed |

The progress bar at the top of the editor shows the distribution of block statuses.

## Keyboard Shortcuts

| Shortcut | Action |
|----------|--------|
| `Cmd/Ctrl+S` | Save project |
| `Cmd/Ctrl+O` | Open project |
| `Cmd/Ctrl+Enter` | Confirm translation and move to next |
| `Cmd/Ctrl+Shift+Enter` | Copy source to target |
| `Cmd/Ctrl+Shift+R` | Mark block as reviewed |
