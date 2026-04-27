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

By default, operates on the project recipe (`<dir-name>.kapi`).
Use `--global` to read/write the global config file (`~/.config/kapi/kapi.yaml`).

## Examples

```bash
# Show path to the project recipe
bowrain config

# Read a recipe value
bowrain config name
bowrain config server.url

# Set a recipe value
bowrain config name "My Project"

# Read global config
bowrain config --global server.url

# Set global config (applies to all projects)
bowrain config --global server.url https://bowrain.example.com
```

## Options

| Flag       | Description                                                                   |
| ---------- | ----------------------------------------------------------------------------- |
| `--global` | Use global config file (`~/.config/kapi/kapi.yaml`) instead of project config |

## Config Keys

### Project Recipe (`<dir-name>.kapi`)

| Key                          | Description                                                | Example                                            |
| ---------------------------- | ---------------------------------------------------------- | -------------------------------------------------- |
| `name`                       | Project name                                               | `My App`                                           |
| `defaults.source_language`   | Source locale (BCP 47)                                     | `en-US`                                            |
| `defaults.target_languages`  | Target locales (list)                                      | `[fr-FR, de-DE]`                                   |
| `server.url`                 | Compound server URL (encodes server / workspace / project) | `https://bowrain.example.com/my-team/proj_abc123`  |
| `server.stream`              | Server stream (`$auto` for auto-detect)                    | `$auto`                                            |

### Global Config (`~/.config/kapi/kapi.yaml`)

| Key                | Description                         | Example                       |
| ------------------ | ----------------------------------- | ----------------------------- |
| `server.url`       | Default server URL for all projects | `https://bowrain.example.com` |
| `plugin_directory` | Plugin directory path               | `/home/user/.config/kapi/plugins` |

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
