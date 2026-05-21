---
sidebar_position: 8
title: plugins
---

# kapi plugins

Manage plugins and bundles for additional formats and tools.

## Synopsis

```bash
kapi plugins <command> [flags]
```

## Concepts

### Plugins vs Bundles

A **plugin** is a standalone format reader/writer or processing tool. A **bundle** is a collection of formats and/or tools distributed as a single installable unit. The Okapi bridge is the canonical bundle — it provides many format filters in one package.

When you install a bundle, its individual capabilities (formats, tools) are registered separately. You can reference individual formats from a bundle (e.g., `okapi-html`) in flows and commands without knowing they came from a bundle.

## Commands

### List installed plugins

```bash
kapi plugins list
kapi plugins list -a              # show all available (installed + registry)
```

### Search for plugins and bundles

```bash
kapi plugins search <query>            # search by name or description
kapi plugins search --bundle           # list all bundles
kapi plugins search --format           # list format plugins (including bundles with formats)
kapi plugins search --tool             # list tool plugins (including bundles with tools)
kapi plugins search --bundle --format  # bundles that contain format capabilities
kapi plugins search --ext .docx        # find plugins that handle .docx files
kapi plugins search --mime text/html   # find plugins that handle HTML
kapi plugins search --type format      # filter by capability type
```

All filter flags are combined with AND logic.

### Install a plugin or bundle

```bash
kapi plugins install <name>                  # install latest version
kapi plugins install <name>@<version>        # install specific version
```

### Update a plugin or bundle

```bash
kapi plugins update <name>       # update specific plugin
kapi plugins update              # check and update all plugins
```

### Remove a plugin or bundle

```bash
kapi plugins remove <name>@<version>   # remove a specific version
kapi plugins remove <name>             # remove all versions
```

## Search Flags

| Flag            | Description                                        |
| --------------- | -------------------------------------------------- |
| `--bundle`      | Show only bundles (collections of formats/tools)   |
| `--format`      | Show only plugins providing format capabilities    |
| `--tool`        | Show only plugins providing tool capabilities      |
| `--type <type>` | Filter by capability type (e.g., "format", "tool") |
| `--mime <type>` | Filter by MIME type (e.g., "text/html")            |
| `--ext <ext>`   | Filter by file extension (e.g., ".docx")           |

## Plugin Directory

Plugins are stored in `~/.config/kapi/plugins/`. Multiple versions can be installed side-by-side:

```
~/.config/kapi/plugins/
  okapi/
    1.46.0/
      version.json
      neokapi-okapi-bridge.jar
    1.47.0/
      version.json
      neokapi-okapi-bridge.jar
```

## Okapi Bridge Bundle

The Okapi bridge bundle provides access to the Okapi format filters:

```bash
kapi plugins install okapi
```

Once installed, additional formats (DOCX, XLSX, EPUB, etc.) appear in `kapi formats`.

## Version Pinning

Pin a specific plugin version in `kapi.yaml`:

```yaml
plugins:
  okapi:
    version: "1.47.0"
```

See [Plugin System](/developer/plugins) for details on writing plugins and bundles.
