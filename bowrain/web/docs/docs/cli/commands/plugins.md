---
sidebar_position: 6
title: plugins
---

# kapi plugin

Manage plugins and bundles that add formats and tools. Plugins are a
[neokapi engine](https://neokapi.github.io/web/neokapi/) feature shared by every
kapi installation — this page covers what a Bowrain user typically needs. For
the full command surface and the plugin model, see the neokapi reference:

- [`kapi plugin` command reference](https://neokapi.github.io/web/neokapi/commands/plugin)
- [Plugin system](https://neokapi.github.io/web/neokapi/docs/contribute/plugins)

## Synopsis

```bash
kapi plugin <command> [flags]
```

A **plugin** is a standalone format reader/writer or processing tool. A
**bundle** is a collection distributed as one installable unit; its individual
formats and tools register separately, so you can reference them by name (e.g.
`okapi-html`) without knowing they came from a bundle.

## Common commands

```bash
kapi plugin list                 # installed plugins
kapi plugin list -a              # installed + available in the registry
kapi plugin search <query>       # search by name/description, --ext, --mime, --type
kapi plugin install <name>       # install latest (or <name>@<version>)
kapi plugin update [<name>]      # update one, or all when omitted
kapi plugin remove <name>        # remove all versions (or <name>@<version>)
```

## The Okapi bridge bundle

The Okapi bridge is the canonical bundle: it brings the Okapi Framework's family
of format filters — desktop-publishing, CAT-tool, and document formats — into
kapi in one install.

```bash
kapi plugin install okapi-bridge
```

Once installed, the additional formats appear in `kapi formats`. Rather than
quoting a fixed number, list what your installation provides:

```bash
kapi formats
```

## Version pinning

Pin a plugin version in the project's `.kapi` recipe under the `plugins:` map so
the whole team resolves the same build:

```yaml
plugins:
  okapi-bridge:
    version: "1.47.0"
```

Installed plugins live under the kapi data directory
(`$XDG_DATA_HOME/kapi/plugins`, or the system plugin roots), with multiple
versions side by side. See the
[plugin system](https://neokapi.github.io/web/neokapi/docs/contribute/plugins)
reference for discovery details and for writing your own plugins.
