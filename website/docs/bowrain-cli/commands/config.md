---
title: config
sidebar_position: 2
---

# bowrain config

View or set configuration values for the current project or global settings.

## Usage

```bash
bowrain config [key] [value] [flags]
```

## Description

With no arguments, prints the path to the config file.
With one argument (key), prints the current value.
With two arguments (key value), sets the value.

By default, operates on the project config file (`.bowrain/config.yaml`).
Use `--global` to read/write the global config file (`~/.config/kapi/kapi.yaml`).

## Examples

```bash
# Show path to project config
bowrain config

# Read a project config value
bowrain config project.name
bowrain config server.url

# Set a project config value
bowrain config project.name "My Project"

# Read global config
bowrain config --global server.url

# Set global config (applies to all projects)
bowrain config --global server.url https://bowrain.example.com
```

## Options

| Flag | Description |
|------|-------------|
| `--global` | Use global config file (`~/.config/kapi/kapi.yaml`) instead of project config |

## Config Keys

### Project Config (`.bowrain/config.yaml`)

| Key | Description | Example |
|-----|-------------|---------|
| `project.name` | Project name | `My App` |
| `project.source_locale` | Source locale (BCP 47) | `en-US` |
| `server.url` | Bowrain Server URL | `https://bowrain.example.com` |
| `server.project_id` | Server project ID | `proj_abc123` |
| `server.workspace` | Workspace slug | `my-team` |

### Global Config (`~/.config/kapi/kapi.yaml`)

| Key | Description | Example |
|-----|-------------|---------|
| `server.url` | Default server URL for all projects | `https://bowrain.example.com` |
| `plugin_directory` | Plugin directory path | `/home/user/.bowrain/plugins` |

## Global vs Project Config

Global config provides defaults that apply to all projects. Project config
overrides global values for the current project.

For example, set the server URL globally so all `bowrain init` commands use it:

```bash
bowrain config --global server.url https://bowrain.example.com
```

Then override it for a specific project if needed:

```bash
bowrain config server.url https://staging.bowrain.example.com
```
