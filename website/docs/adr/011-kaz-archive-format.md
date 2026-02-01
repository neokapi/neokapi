---
id: 011-kaz-archive-format
sidebar_position: 11
title: "ADR-011: KAZ Archive"
---

# ADR-011: KAZ archive format for project packaging

**Status:** Accepted

## Context

Localization projects involve multiple source files, translation state, block
indices for UI editors, and preview renderings. Shipping these as loose files
complicates sharing, version control, and editor state management. Okapi uses
XLIFF files but has no native project container.

## Decision

Define the `.kaz` archive format as a ZIP file with a structured layout:

```
manifest.yaml              # project metadata, source/target locales, items
blocks/<item>.json         # block index per source item (translatable segments)
preview/<item>.html        # HTML preview for editor display
items/<file>               # original source files (optional)
```

### Manifest

The `manifest.yaml` contains source and target locales, item list with format
and block count and status, and project-level metadata.

### Block Index

Per-item block indices store segment-level translation state (source text,
targets per locale, match origin, confidence) without requiring document
re-parsing. The Bowrain editor loads block indices lazily per selected item.

### Preview

Pre-rendered HTML previews for fast display. Strategies vary by format:
HTML renders as-is, Markdown converts to HTML, other formats use generic
highlighting.

## Alternatives Considered

- **XLIFF as container**: XLIFF is an interchange format for translatable
  content, not a project container; it has no native support for bundling
  previews, original source files, or project-level metadata alongside
  translation data.
- **Directory-based project**: harder to share; no atomic save.
- **Custom binary format**: harder to inspect and debug; ZIP is universally
  supported.
- **SQLite project file**: considered but ZIP is simpler for the file-based
  content and enables standard archive tools for inspection.

## Consequences

- Single file for sharing projects (email, cloud storage, version control)
- Editor loads block indices lazily per item; no need to re-parse documents
- Previews display instantly without format-specific rendering
- Standard ZIP tooling can inspect and extract .kaz files
- The Bowrain desktop app uses .kaz as its native project format
