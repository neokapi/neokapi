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

## Building from Source

```bash
cd apps/bowrain
wails3 build
```

For development with hot reload:

```bash
cd apps/bowrain
wails3 dev
```

## Keyboard Shortcuts

| Shortcut | Action |
|----------|--------|
| `Cmd/Ctrl+S` | Save project |
| `Cmd/Ctrl+O` | Open project |
| `Cmd/Ctrl+Enter` | Confirm translation and move to next |
| `Cmd/Ctrl+Shift+Enter` | Copy source to target |
