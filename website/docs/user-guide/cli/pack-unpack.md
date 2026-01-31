---
sidebar_position: 5
title: pack & unpack
---

# kapi pack / kapi unpack

Package and extract KAZ project archives.

## Pack

### Synopsis

```bash
kapi pack <directory> -o <output.kaz> [flags]
```

### Description

Package a directory of source files into a `.kaz` archive. The archive includes a manifest, block indices for each file, and optional HTML previews.

### Examples

```bash
# Pack a project directory
kapi pack ./project -o project.kaz -s en -t fr

# Pack with preview generation
kapi pack ./project -o project.kaz -s en -t fr --preview
```

## Unpack

### Synopsis

```bash
kapi unpack <archive.kaz> -o <directory> [flags]
```

### Description

Extract a `.kaz` archive to a directory. Restores source files, block indices, and previews.

### Examples

```bash
# Unpack a project archive
kapi unpack project.kaz -o ./extracted

# List archive contents without extracting
kapi unpack project.kaz --list
```

## KAZ Archive Format

The `.kaz` format is a ZIP file with a structured layout:

```
manifest.yaml              # Project metadata, locales, items
blocks/<item>.json         # Block index per source item
preview/<item>.html        # HTML preview for editor display
items/<file>               # Original source files
```

See [ADR-011](/docs/adr/011-kaz-archive-format) for the design rationale.
