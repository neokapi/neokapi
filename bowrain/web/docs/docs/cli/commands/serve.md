---
title: serve
sidebar_position: 10
---

# kapi serve

Start a local web-based project editor — like `jupyter notebook` for translation
projects. No authentication or server setup required.

## Usage

```bash
kapi serve [directory] [flags]
```

## Examples

```bash
# Open a project directory
kapi serve ./my-project/

# Use a custom port
kapi serve ./my-project/ --port 4000

# Don't auto-open the browser
kapi serve ./my-project/ --no-open
```

## What Happens

1. Creates a temporary SQLite store and imports your project content
2. Starts a local web server on `http://localhost:3000`
3. Opens your browser to the translation editor
4. You edit translations using the same UI as the Bowrain desktop app
5. On exit (Ctrl+C), changes are saved back to the project file

## Options

| Flag        | Description                      | Default |
| ----------- | -------------------------------- | ------- |
| `--port`    | Port to listen on                | `3000`  |
| `--no-open` | Don't open browser automatically | `false` |

## When to Use

`kapi serve` is ideal for:

- **Quick edits** to a translation project without installing Bowrain
- **Remote editing** via SSH port forwarding
- **Team reviews** where a colleague needs to view translations in a browser
- **CI previews** to inspect translation output in a pipeline

For full multi-user collaboration with workspaces and access control, use
[`bowrain-server`](/developer/server) instead.

## Comparison

| Feature       | `kapi serve`     | `bowrain-server`       |
| ------------- | ------------------- | ---------------------- |
| Auth required | No                  | Yes (SSO)              |
| Workspaces    | No (single project) | Yes (multi-workspace)  |
| Binding       | localhost only      | 0.0.0.0 (configurable) |
| Use case      | Local editing       | Team deployment        |
| Users         | Single user         | Multiple users         |
