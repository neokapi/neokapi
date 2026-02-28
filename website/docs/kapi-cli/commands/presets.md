---
sidebar_position: 4
title: presets
---

# kapi presets

Manage format and framework presets.

## Synopsis

```bash
kapi presets list
kapi presets show <name>
```

## Description

Presets are pre-configured format and tool settings that simplify common operations. Use `presets list` to see available presets and `presets show` to view details.

## Examples

```bash
# List available presets
kapi presets list

# Show details of a preset
kapi presets show default-json
```

## Subcommands

| Command | Description |
|---------|-------------|
| `list` | List available presets |
| `show <name>` | Show detailed preset configuration |
