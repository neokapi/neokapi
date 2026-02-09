---
title: store
sidebar_position: 8
---

# kapi store

Manage the Content Store: projects, versions, and KAZ export/import.

## Commands

### store projects

List all projects in the store:

```bash
kapi store projects
```

### store version

Create a version snapshot:

```bash
kapi store version --project proj-1 --label "v1.0" --description "Initial release"
```

### store versions

List versions for a project:

```bash
kapi store versions --project proj-1
```

### store export

Export a project as a KAZ archive:

```bash
kapi store export --project proj-1 --output project.kaz
```

### store import

Import a KAZ archive into the store:

```bash
kapi store import --input project.kaz
```

## Options

| Flag | Description |
|------|-------------|
| `--store` | Path to the store database (default: `gokapi.db`) |
| `--project` | Project ID |
| `--label` | Version label |
| `--description` | Version description |
| `--output` | Output file path |
| `--input` | Input file path |
