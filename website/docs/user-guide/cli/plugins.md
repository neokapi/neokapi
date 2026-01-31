---
sidebar_position: 6
title: plugins
---

# kapi plugins

Manage plugins for additional formats and tools.

## Synopsis

```bash
kapi plugins <command> [flags]
```

## Commands

### List installed plugins

```bash
kapi plugins list
```

### Install a plugin

```bash
kapi plugins install <name> [--version <version>]
```

### Update a plugin

```bash
kapi plugins update <name>
```

### Remove a plugin

```bash
kapi plugins remove <name> [--version <version>]
```

## Plugin Directory

Plugins are stored in `~/.config/gokapi/plugins/`. Multiple versions can be installed side-by-side:

```
~/.config/gokapi/plugins/
  okapi/
    1.46.0/
      version.json
      gokapi-okapi-bridge.jar
    1.47.0/
      version.json
      gokapi-okapi-bridge.jar
```

## Java Bridge Plugin

The Okapi Java bridge plugin provides access to 40+ Okapi format filters:

```bash
kapi plugins install okapi
```

Once installed, additional formats (DOCX, XLSX, EPUB, etc.) appear in `kapi formats list`.

## Version Pinning

Pin a specific plugin version in `gokapi.yaml`:

```yaml
plugins:
  okapi:
    version: "1.47.0"
```

See [Plugin System](/docs/developer/plugins) for details on writing plugins.
