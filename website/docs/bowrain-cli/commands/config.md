---
title: config
sidebar_position: 2
---

# brain config

View or set configuration values for the current project or global settings.

## Usage

```bash
brain config [key] [value] [flags]
```

## Description

With no arguments, prints the path to the config file.
With one argument (key), prints the current value.
With two arguments (key value), sets the value.

By default, operates on the project config file (`.bowrain/config.yaml`).
Use `--global` to read/write the global config file (`~/.config/brain/brain.yaml`).

## Examples

```bash
# Show path to project config
brain config

# Read a project config value
brain config project.name
brain config server.url

# Set a project config value
brain config project.name "My Project"

# Read global config
brain config --global server.url

# Set global config (applies to all projects)
brain config --global server.url https://bowrain.example.com
```

## Options

| Flag | Description |
|------|-------------|
| `--global` | Use global config file (`~/.config/brain/brain.yaml`) instead of project config |

## Config Keys

### Project Config (`.bowrain/config.yaml`)

| Key | Description | Example |
|-----|-------------|---------|
| `project.name` | Project name | `My App` |
| `project.source_locale` | Source locale (BCP 47) | `en-US` |
| `server.url` | Bowrain Server URL | `https://bowrain.example.com` |
| `server.project_id` | Server project ID | `proj_abc123` |
| `server.workspace` | Workspace slug | `my-team` |

### Global Config (`~/.config/brain/brain.yaml`)

| Key | Description | Example |
|-----|-------------|---------|
| `server.url` | Default server URL for all projects | `https://bowrain.example.com` |
| `plugin_directory` | Plugin directory path | `/home/user/.bowrain/plugins` |

## Global vs Project Config

Global config provides defaults that apply to all projects. Project config
overrides global values for the current project.

For example, set the server URL globally so all `brain init` commands use it:

```bash
brain config --global server.url https://bowrain.example.com
```

Then override it for a specific project if needed:

```bash
brain config server.url https://staging.bowrain.example.com
```
