---
id: 004-format-system
sidebar_position: 4
title: "ADR-004: Format System"
---

# ADR-004: Tiered format system

**Status:** Accepted

## Context

Localization requires support for many document formats. Okapi provides 40+
filters built over 15 years. Rewriting all of them in Go before shipping would
delay the project indefinitely, but relying entirely on Java bridges adds JVM
overhead and deployment complexity. A single mechanism for registering,
discovering, and instantiating format readers and writers is needed regardless
of where the implementation lives.

## Decision

### Three Implementation Tiers

1. **Native formats** (Go): Full-control, high-performance implementations for
   the most common formats. Currently 15 built-in: HTML, XML, XLIFF, XLIFF 2,
   JSON, YAML, PO, Properties, Plaintext, Markdown, CSV, SRT, VTT, TMX.
   Each lives under `formats/<name>/` with `reader.go`, `writer.go`,
   `config.go`, and roundtrip tests.

2. **Plugin formats** (any language): External executables communicating over
   gRPC via HashiCorp go-plugin. Language-agnostic; crash-isolated. See
   [ADR-005](/docs/adr/005-plugin-system).

3. **Java bridge formats** (Okapi): JVM subprocesses running Okapi filters
   via NDJSON protocol. Provides immediate access to 40+ production-proven
   filters (DOCX, XLSX, EPUB, IDML, PDF, etc.). See
   [ADR-006](/docs/adr/006-java-bridge).

All three tiers register into the same `FormatRegistry`. Callers get a
`DataFormatReader` / `DataFormatWriter` from the registry and do not know
which tier produced it.

### Reader/Writer Separation

Separate interfaces for reading documents and writing reconstructed documents:

- `DataFormatReader`: `Open(ctx, doc)` then `Read(ctx) <-chan PartResult`
- `DataFormatWriter`: `SetOutput(path)`, `Write(ctx, <-chan *Part)`

Readers are stateless per call. Writers are locale-aware and enforce the
target language explicitly.

### Multi-Strategy Format Detection

Cascade approach: MIME type -> file extension -> magic bytes -> content
sniffing. The registry stores a `FormatSignature` per format with MIME types,
extensions, magic byte prefixes, and an optional sniff function. The detector
tries strategies in order until a match is found.

### Skeleton Strategies

Two approaches for document reconstruction:

- **Fragment-based** (HTML, XML, XLIFF): interleaved `SkeletonText` (literal
  markup) and `SkeletonRef` (Block/Data references). Preserves exact structure
  for complex nested formats.
- **Re-parse** (Plaintext, JSON, YAML, PO, Properties): writer re-opens the
  source document and replaces translatable content as encountered. Simpler
  formats don't need skeleton overhead.

## Consequences

- Ship immediately with both native Go formats and full Okapi filter access
- Gradually port high-value Okapi filters to native Go without breaking
  existing workflows
- Plugin tier enables community contributions in any language
- Format detection works uniformly across all tiers
- The bridge tier adds JVM startup latency for first use, mitigated by the
  bridge pool keeping JVMs warm
- New native formats follow a consistent package structure under `formats/`
