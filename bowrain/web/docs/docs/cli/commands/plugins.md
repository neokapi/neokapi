---
sidebar_position: 6
title: plugins
---

# kapi plugin

Manage plugins and bundles for additional formats and tools.

## Synopsis

```bash
kapi plugin <command> [flags]
```

## Concepts

### Plugins vs Bundles

A **plugin** is a standalone format reader/writer or processing tool. A **bundle** is a collection of formats and/or tools distributed as a single installable unit. The Okapi bridge is the canonical bundle — it provides 40+ format filters in one package.

When you install a bundle, its individual capabilities (formats, tools) are registered separately. You can reference individual formats from a bundle (e.g., `okapi-html`) in flows and commands without knowing they came from a bundle.

## Commands

### List installed plugins

```bash
kapi plugin list
kapi plugin list -a              # show all available (installed + registry)
```

### Search for plugins and bundles

```bash
kapi plugin search <query>            # search by name or description
kapi plugin search --bundle           # list all bundles
kapi plugin search --format           # list format plugins (including bundles with formats)
kapi plugin search --tool             # list tool plugins (including bundles with tools)
kapi plugin search --bundle --format  # bundles that contain format capabilities
kapi plugin search --ext .docx        # find plugins that handle .docx files
kapi plugin search --mime text/html   # find plugins that handle HTML
kapi plugin search --type format      # filter by capability type
```

All filter flags are combined with AND logic.

### Install a plugin or bundle

```bash
kapi plugin install <name>                  # install latest version
kapi plugin install <name>@<version>        # install specific version
```

### Update a plugin or bundle

```bash
kapi plugin update <name>       # update specific plugin
kapi plugin update              # check and update all plugins
```

### Remove a plugin or bundle

```bash
kapi plugin remove <name>@<version>   # remove a specific version
kapi plugin remove <name>             # remove all versions
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

The Okapi bridge bundle provides access to 40+ Okapi format filters:

```bash
kapi plugin install okapi
```

Once installed, additional formats (DOCX, XLSX, EPUB, etc.) appear in `kapi formats`.

## Version Pinning

Pin a specific plugin version in `kapi.yaml`:

```yaml
plugins:
  okapi:
    version: "1.47.0"
```

See [Plugin System](https://neokapi.github.io/web/neokapi/docs/developer/plugins) for details on writing plugins and bundles.
