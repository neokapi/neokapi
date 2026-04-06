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

### Format reference syntax

Use the `-f` flag with the syntax `name[@version][:preset]` to select a specific
format version and/or preset:

| Reference                        | Version | Preset     |
| -------------------------------- | ------- | ---------- |
| `-f okf_openxml`                 | latest  | default    |
| `-f okf_openxml@0.38`            | 0.38    | default    |
| `-f okf_openxml:wellFormed`      | latest  | wellFormed |
| `-f okf_openxml@0.38:wellFormed` | 0.38    | wellFormed |

## Examples

```bash
# List available presets
kapi presets list

# Show details of a preset
kapi presets show default-json

# Use a preset with pseudo-translate
kapi pseudo ~/Downloads/*.docx -f okf_openxml:wellFormed -o "./out/\{name\}_\{lang\}.\{ext\}"

# Pin a specific format version and preset
kapi pseudo input.docx -f okf_openxml@0.38:wellFormed -o ./out/
```

## Subcommands

| Command       | Description                        |
| ------------- | ---------------------------------- |
| `list`        | List available presets             |
| `show <name>` | Show detailed preset configuration |
