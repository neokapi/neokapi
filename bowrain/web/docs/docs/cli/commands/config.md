---
title: config
sidebar_position: 2
---

# Bowrain settings (`kapi config`)

There is one configuration command, `kapi config`. The bowrain plugin does not
ship its own: it claims the `bowrain.*` key namespace in its manifest
(`capabilities.config_namespaces`), and kapi routes those keys to the plugin's
own config file. The retired `kapi bowrain config` had no capability this does
not cover.

Two scopes share the verb, split by shape:

| Shape | Scope | Stored in |
| --- | --- | --- |
| `kapi config <key> [value]` (positional) | the project recipe | `kapi.yaml`, committed |
| `kapi config set/get/unset <key> [value]` | per-machine app config | `~/.config/kapi/kapi.yaml`, or a plugin's own file for a namespaced key |

## Per-machine bowrain defaults

```bash
kapi config get bowrain.server.url        # Read the default server URL
kapi config set bowrain.server.url https://app.bowrain.cloud
kapi config unset bowrain.server.url      # Restore the built-in default
kapi config path bowrain                  # ~/.config/bowrain/bowrain.yaml
```

`kapi config list` shows every namespace at once, kapi's own keys alongside
each installed plugin's, and takes `-o json` like any other listing.

| Key | Description | Example |
| --- | --- | --- |
| `bowrain.server.url` | Default server URL `kapi init` offers for new projects | `https://app.bowrain.cloud` |

## Project settings

A project's own server binding lives in the recipe, not in per-machine config,
so it is committed and travels with the repository:

```bash
kapi config name                # Read the project name
kapi config name "My Project"   # Set the project name
kapi config server.url          # Read the compound server URL
kapi config server.url https://app.bowrain.cloud/my-team/proj_abc123
kapi config server.stream '$auto'
```

| Key | Description | Example |
| --- | --- | --- |
| `name` | Project name | `My App` |
| `defaults.source_language` | Source locale (BCP 47) | `en` |
| `defaults.target_languages` | Target locales (list) | `[fr, de]` |
| `server.url` | Compound server URL (encodes server / workspace / project) | `https://app.bowrain.cloud/my-team/proj_abc123` |
| `server.stream` | Server stream (`$auto` for auto-detect) | `$auto` |

## Precedence

The per-machine default is a starting point for new projects; the recipe is
authoritative for a project that already exists. Set the default once:

```bash
kapi config set bowrain.server.url https://app.bowrain.cloud
```

and every subsequent `kapi init` offers it, while any individual project can
still point elsewhere:

```bash
kapi config server.url https://staging.bowrain.cloud
```
