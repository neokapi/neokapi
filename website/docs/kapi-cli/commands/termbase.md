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
```

## Description

A termbase is a glossary of approved terms stored as a JSON file. Use these commands to import, look up, and manage terms for consistent translations.

## Commands

### import

Import terms from CSV or JSON into a termbase:

```bash
# Import from CSV
kapi termbase import terms.csv --format csv -s en -t fr --domain general

# Import from JSON
kapi termbase import terms.json --format json -s en -t fr
```

### export

Export termbase to CSV or JSON:

```bash
kapi termbase export --format csv -o terms.csv
kapi termbase export --format json -o terms.json
```

### lookup

Look up terms in running text:

```bash
# Exact lookup
kapi termbase lookup "The authentication module uses end-to-end encryption" -s en -t fr

# Fuzzy lookup
kapi termbase lookup "authenticating users" -s en -t fr --fuzzy
```

### search

Search concepts in the termbase:

```bash
# Search by term
kapi termbase search "encryption" -s en

# Filter by domain
kapi termbase search "encryption" -s en --domain security
```

### stats

Show termbase statistics:

```bash
kapi termbase stats
```

## Use in Flows

Terminology can be integrated into translation flows:

```bash
# Use term-lookup and term-enforce tools in a flow
kapi flow run ai-translate -i input.html -o output.html --source-lang en --target-lang fr
```

See [Terminology features](/docs/features/terminology) for details.
