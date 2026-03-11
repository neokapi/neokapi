---
sidebar_position: 6
title: termbase
---

# kapi termbase

Manage project terminology. Import terms from CSV or JSON, look up terms in running text, search concepts, and view statistics.

## Synopsis

```bash
kapi termbase import <file> [flags]
kapi termbase export [flags]
kapi termbase lookup <text> [flags]
kapi termbase search <query> [flags]
kapi termbase stats [flags]
kapi termbase list
```

## Description

A termbase is a glossary of approved terms stored as a SQLite database. Use these commands to import, look up, and manage terms for consistent translations.

## Resource Location

All termbase commands (except `list`) accept these mutually exclusive flags for specifying which termbase to use:

| Flag | Resolves to | Example |
|------|------------|---------|
| `--name <n>` | `~/.config/kapi/termbases/<n>.db` | `--name project-terms` |
| `--local` | `./termbase.db` (current directory) | `--local` |
| `--file <path>` | Explicit file path | `--file /shared/glossary.db` |
| *(no flag)* | Same as `--local` | |

Databases are created on demand if they don't exist.

## Commands

### import

Import terms from CSV or JSON into a termbase:

```bash
# Import into a named termbase in KAPI_HOME
kapi termbase import terms.csv --name project-terms --format csv -s en -t fr

# Import into default local termbase (./termbase.db)
kapi termbase import terms.csv --format csv -s en -t fr --domain general

# Import from JSON
kapi termbase import terms.json --format json

# Import into a specific file
kapi termbase import terms.csv --file /shared/glossary.db --format csv -s en -t fr
```

### export

Export termbase to CSV or JSON:

```bash
kapi termbase export --name project-terms --format csv -o terms.csv -s en -t fr
kapi termbase export --format json -o terms.json
```

### lookup

Look up terms in running text:

```bash
# Exact lookup using a named termbase
kapi termbase lookup "encryption" --name project-terms -s en -t fr

# Fuzzy lookup
kapi termbase lookup "authenticating users" -s en -t fr --fuzzy
```

### search

Search concepts in the termbase:

```bash
kapi termbase search "encryption" --name project-terms -s en
kapi termbase search "auth" -s en --limit 50
```

### stats

Show termbase statistics:

```bash
kapi termbase stats --name project-terms
kapi termbase stats                          # uses ./termbase.db
```

### list

List all named termbases in KAPI_HOME:

```bash
kapi termbase list
```

## Use in Flows

Terminology can be integrated into translation flows using the `--termbase` flag:

```bash
kapi flow run ai-translate -i input.html -o output.html -s en -t fr \
  --termbase project-terms
```

See [Terminology features](/docs/features/terminology) for details.
