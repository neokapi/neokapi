---
sidebar_position: 4
title: Terminology Management
---

# Terminology Management

gokapi includes a built-in terminology management system inspired by the TBX (TermBase eXchange) standard. It supports concept-oriented term management with multi-locale terms, lifecycle statuses, and domain classification.

## Data Model

Terminology in gokapi follows a **concept-oriented** model:

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

- **In-memory** — ephemeral; used in Bowrain for per-project termbases
- **SQLite** — persistent; for long-lived termbases with fuzzy matching

## CLI Usage

### Import Terms

```bash
# Import from CSV (source,target columns with optional domain)
kapi termbase import terms.csv --format csv -s en -t fr --domain general

# Import from CSV with header row
kapi termbase import terms.csv --format csv -s en -t fr --has-header

# Import from JSON (full concept format)
kapi termbase import termbase.json --format json
```

### Export Terms

```bash
# Export as CSV
kapi termbase export --format csv -s en -t fr -o terms.csv

# Export as JSON
kapi termbase export --format json -o termbase.json
```

### Look Up Terms

```bash
# Look up terms in text
kapi termbase lookup "The authentication module uses end-to-end encryption" -s en -t fr

# Fuzzy lookup
kapi termbase lookup "authenticating users" -s en -t fr --fuzzy
```

### Search Concepts

```bash
# Search by term text
kapi termbase search "encryption" -s en

# Filter by domain
kapi termbase search "encryption" -s en --domain security
```

### View Statistics

```bash
kapi termbase stats
```

## Pipeline Integration

Two pipeline tools integrate terminology into the translation workflow:

### Term Lookup Tool

The `term-lookup` tool scans source text in each Block and annotates it with matched terminology:

```bash
kapi flow run --input docs/ --output out/ \
  --tools term-lookup,ai-translate \
  -s en -t fr \
  --termbase project.tb
```

Matched terms are attached as `TermAnnotation` entries on the Block, providing source term, target suggestions, positions, and status information.

### Term Enforce Tool

The `term-enforce` tool checks that translated blocks use the correct terminology:

```bash
kapi flow run --input translated/ --output qa/ \
  --tools term-enforce \
  -s en -t fr \
  --termbase project.tb
```

Violations are reported as block properties (`term-enforce-errors`, `term-enforce-violations`) and as annotations with details about expected vs. actual term usage.

## Bowrain Integration

In Bowrain, each project has its own termbase that persists in the `.kaz` project file:

### Terminology Explorer

Click "Terminology" in the project view to open the full terminology management panel:

- **Browse concepts** with search and locale filtering
- **Add concepts** with terms in multiple locales and status assignments
- **Edit concepts** inline — click any concept to modify its terms, domain, or definition
- **Delete concepts** that are no longer needed
- **Import** terms from CSV files or JSON termbases
- **Export** the full termbase as JSON

### Context Panel

In the translation editor, click "Context" in the toolbar to show a side panel with:

- **TM Matches** for the current block (with scores and apply buttons)
- **Terminology** matches found in the current block's source text

For each matched term, the panel shows:
- Source term with lifecycle status badge
- Target term suggestions for the current target locale
- Domain classification

### Term Enforcement

Use the `term-enforce` tool in a flow to check all translated blocks for correct terminology usage. Violations highlight blocks where expected terms are missing from the translation.

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

gokapi uses a concept-oriented model (inspired by TBX) rather than flat source→target pairs. This enables:

- **Multiple terms per locale**: A concept can have preferred and admitted terms in the same language
- **Lifecycle management**: Terms go through statuses (proposed → approved → preferred → deprecated)
- **Rich metadata**: Domain classification, definitions, usage notes, and grammatical information
- **Multi-locale**: Terms in any number of languages belong to the same concept

### In-Text Discovery

The `LookupAll` function scans running text to find all term occurrences. This powers both the pipeline `term-lookup` tool and the Bowrain editor's context panel. Case-insensitive matching is used by default with exact string matching for maximum precision.

### Separate from TM

Terminology and translation memory are separate systems because they serve different purposes:
- **TM** answers: "How was this sentence translated before?"
- **Terminology** answers: "What is the correct term for this concept?"

Both integrate through the Block's annotation system and are displayed together in the editor's Context panel.
