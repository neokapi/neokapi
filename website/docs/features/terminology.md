---
sidebar_position: 4
title: Terminology Management
---

# Terminology Management

neokapi includes a built-in terminology management system inspired by the TBX (TermBase eXchange) standard. It supports concept-oriented term management with multi-locale terms, lifecycle statuses, and domain classification.

## Data Model

Terminology in neokapi follows a **concept-oriented** model:

```
Concept (e.g., "cloud storage")
├── Domain: "infrastructure"
├── Definition: "Remote file storage accessed via internet"
├── Term: "cloud storage" (en, preferred)
├── Term: "stockage cloud" (fr, preferred)
├── Term: "stockage en nuage" (fr, admitted)
├── Term: "Cloud-Speicher" (de, preferred)
└── Term: "クラウドストレージ" (ja, preferred)
```

A **concept** groups related terms across languages. Each concept can have:
- **Domain**: Subject area classification (e.g., "security", "ui", "marketing")
- **Definition**: Clear description of the concept
- **Terms**: Multiple terms per locale, each with a lifecycle status

### Term Lifecycle Statuses

| Status | Meaning |
|--------|---------|
| `preferred` | The recommended term to use |
| `approved` | Accepted for use |
| `admitted` | Allowed but not recommended |
| `deprecated` | Should be avoided; being phased out |
| `proposed` | Under review, not yet approved |
| `forbidden` | Must not be used |

## Storage Backends

Three storage tiers support progressive complexity:

1. **In-memory** (`core/termbase/`) — fast, ephemeral. Used for session-scoped batch processing.
2. **CLI SQLite** (`cli/storage/termbase/`) — persistent file-based storage for kapi and bowrain CLI. No project_id or stream columns — designed for single-user, file-based workflows.
3. **Server SQLite/PostgreSQL** (`bowrain/termbase/`) — persistent storage for Bowrain Server with project scoping, terminology streams, and workspace isolation.

### kapi vs Bowrain

| Aspect | kapi CLI | Bowrain Server |
|--------|---------|---------------|
| Storage | SQLite files on disk | SQLite or PostgreSQL |
| Location | Named in KAPI_HOME, local dir, or file path | Server-managed per workspace |
| Scope | Single user, single machine | Multi-user, multi-workspace |
| Features | CRUD, import/export, lookup, search | + streams, project scoping, REST API |

## CLI Usage

### Resource Location

All termbase commands (except `list`) accept these mutually exclusive flags:

| Flag | Resolves to | Example |
|------|------------|---------|
| `--name <n>` | `~/.config/kapi/termbases/<n>.db` | `--name project-terms` |
| `--local` | `./termbase.db` (current directory) | `--local` |
| `--file <path>` | Explicit file path | `--file /shared/glossary.db` |
| *(no flag)* | Same as `--local` | |

Databases are created on demand if they don't exist.

### Import Terms

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

### Export Terms

```bash
kapi termbase export --name project-terms --format csv -o terms.csv -s en -t fr
kapi termbase export --format json -o terms.json
```

### Look Up Terms

```bash
# Exact lookup using a named termbase
kapi termbase lookup "encryption" --name project-terms -s en -t fr

# Fuzzy lookup
kapi termbase lookup "authenticating users" -s en -t fr --fuzzy
```

### Search Concepts

```bash
kapi termbase search "encryption" --name project-terms -s en
kapi termbase search "auth" -s en --limit 50
```

### View Statistics

```bash
kapi termbase stats --name project-terms
kapi termbase stats                          # uses ./termbase.db
```

### List Named Termbases

```bash
kapi termbase list
```

## Pipeline Integration

Two pipeline tools integrate terminology into the translation workflow:

### Term Lookup Tool

The `term-lookup` tool scans source text in each Block and annotates it with matched terminology. Matched terms are attached as `TermAnnotation` entries on the Block, providing source term, target suggestions, positions, and status information.

### Term Enforce Tool

The `term-enforce` tool checks that translated blocks use the correct terminology.

Violations are reported as block properties (`term-enforce-errors`, `term-enforce-violations`) and as annotations with details about expected vs. actual term usage.

## Import/Export Formats

### CSV Format

Simple two-column format for quick import/export:

```csv
source,target,domain
cloud storage,stockage cloud,infrastructure
encryption,chiffrement,security
authentication,authentification,security
```

Options: `--delimiter` (default `,`), `--has-header`, `--domain`, `-s` (source locale), `-t` (target locale).

### JSON Format

Full concept-oriented format preserving all metadata:

```json
{
  "name": "Project Terms",
  "version": "1.0",
  "concepts": [
    {
      "id": "c1",
      "domain": "security",
      "definition": "Encryption where only endpoints can decrypt",
      "terms": [
        {"text": "end-to-end encryption", "locale": "en", "status": "preferred"},
        {"text": "chiffrement de bout en bout", "locale": "fr", "status": "preferred"}
      ]
    }
  ]
}
```

## Design Decisions

### Concept-Oriented vs. Flat Glossary

neokapi uses a concept-oriented model (inspired by TBX) rather than flat source→target pairs. This enables:

- **Multiple terms per locale**: A concept can have preferred and admitted terms in the same language
- **Lifecycle management**: Terms go through statuses (proposed → approved → preferred → deprecated)
- **Rich metadata**: Domain classification, definitions, usage notes, and grammatical information
- **Multi-locale**: Terms in any number of languages belong to the same concept

### In-Text Discovery

The `LookupAll` function scans running text to find all term occurrences. This powers the pipeline `term-lookup` tool and can be used by editors to show per-block terminology suggestions. Case-insensitive matching is used by default with exact string matching for maximum precision.

### Separate from TM

Terminology and translation memory are separate systems because they serve different purposes:
- **TM** answers: "How was this sentence translated before?"
- **Terminology** answers: "What is the correct term for this concept?"

Both integrate through the Block's annotation system, making them available to any downstream tool or editor.
