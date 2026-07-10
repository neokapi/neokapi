---
title: config
sidebar_position: 2
---

# kapi bowrain config

View or set configuration values for the current project or global settings.

## Usage

```bash
kapi bowrain config [key] [value] [flags]
```

## Description

With no arguments, prints the path to the config file.
With one argument (key), prints the current value.
With two arguments (key value), sets the value.

By default, operates on the project recipe (`<dir-name>.kapi`).
Use `--global` to read/write the global config file (`~/.config/bowrain/bowrain.yaml`).

## Examples

```bash
# Show path to the project recipe
kapi bowrain config

# Read a recipe value
kapi bowrain config name
kapi bowrain config server.url

# Set a recipe value
kapi bowrain config name "My Project"

# Read global config
kapi bowrain config --global server.url

# Set global config (applies to all projects)
kapi bowrain config --global server.url https://bowrain.cloud
```

## Options

| Flag       | Description                                                                   |
| ---------- | ----------------------------------------------------------------------------- |
| `--global` | Use global config file (`~/.config/bowrain/bowrain.yaml`) instead of project config |

## Config Keys

### Project Recipe (`<dir-name>.kapi`)

| Key                          | Description                                                | Example                                            |
| ---------------------------- | ---------------------------------------------------------- | -------------------------------------------------- |
| `name`                       | Project name                                               | `My App`                                           |
| `defaults.source_language`   | Source locale (BCP 47)                                     | `en-US`                                            |
| `defaults.target_languages`  | Target locales (list)                                      | `[fr-FR, de-DE]`                                   |
| `server.url`                 | Compound server URL (encodes server / workspace / project) | `https://bowrain.cloud/my-team/proj_abc123`  |
| `server.stream`              | Server stream (`$auto` for auto-detect)                    | `$auto`                                            |

### Global Config (`~/.config/bowrain/bowrain.yaml`)

| Key                | Description                         | Example                       |
| ------------------ | ----------------------------------- | ----------------------------- |
| `server.url`       | Default server URL for all projects | `https://bowrain.cloud` |
| `plugin_directory` | Plugin directory path               | `/home/user/.config/bowrain/plugins` |

## Global vs Project Config

Global config provides defaults that apply to all projects. Project config
overrides global values for the current project.

For example, set the server URL globally so all `kapi init` commands use it:

```bash
kapi bowrain config --global server.url https://bowrain.cloud
```

Then override it for a specific project if needed:

```bash
kapi bowrain config server.url https://staging.bowrain.cloud
```
