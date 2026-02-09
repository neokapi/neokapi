---
title: Content Store
sidebar_position: 10
---

# Content Store

The Content Store is gokapi's central persistence layer. It stores your translation projects, blocks, and version history in a local SQLite database.

## Key Features

- **Project management**: Organize content into named projects with source and target locales
- **Block deduplication**: Identical source text is automatically deduplicated via content-addressable hashing
- **Version snapshots**: Create named versions and track changes between them
- **KAZ export/import**: Export projects as portable KAZ archives

## CLI Commands

### Managing Projects

```bash
# List all projects
kapi store projects

# Create a version snapshot
kapi store version --project proj-1 --label "v1.0" --description "Initial release"

# List versions
kapi store versions --project proj-1
```

### Export and Import

```bash
# Export a project as KAZ
kapi store export --project proj-1 --output project.kaz

# Import a KAZ file
kapi store import --input project.kaz
```

## How It Works

When you pull content from a connector or process files through a flow, blocks are stored in the Content Store with:

1. **Content hash**: SHA-256 of the normalized source text, used for deduplication
2. **Context hash**: SHA-256 of block metadata (name, type, properties)
3. **Targets**: Translations keyed by locale
4. **Properties**: Key-value metadata

### Versions

Create a version to snapshot the current state of a project:

```bash
kapi store version --project proj-1 --label "before-review"
# ... make changes ...
kapi store version --project proj-1 --label "after-review"
```

Versions enable tracking what changed between snapshots, which is useful for review workflows and audit trails.
