---
title: config
sidebar_position: 2
---

# kapi bowrain config

View or set bowrain configuration values. Under the plugin's
[no-shadowing rule](/architecture-decisions/010-bowrain-cli-and-project-model),
the built-in `kapi config` keeps the top-level verb and covers the project
recipe keys positionally, so the bowrain plugin's config command is
group-scoped — invoke it as `kapi bowrain config`. Its niche is the **global**
bowrain config file (`--global`); without `--global` it reads and writes the
project recipe, the same as the built-in positional form.

## Usage

```bash
kapi bowrain config [key] [value] [flags]
```

## Description

With no arguments, prints the path to the config file.
With one argument (key), prints the current value.
With two arguments (key value), sets the value.

By default, operates on the project recipe (`<dir-name>.kapi`) — for which the
documented spelling is the built-in positional form, `kapi config server.url …`.
Use `--global` to read or write the global config file
(`~/.config/bowrain/bowrain.yaml`) — for example the default server URL that
`kapi init` offers for new projects.

## Examples

```bash
# Read and set recipe keys — the built-in positional form
kapi config name              # Read the project name
kapi config name "My Project" # Set the project name
kapi config server.url        # Read the compound server URL

# The bowrain global config file (its niche)
kapi bowrain config --global server.url                       # Read the default server URL
kapi bowrain config --global server.url https://bowrain.cloud # Set it (applies to all projects)
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
