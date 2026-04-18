---
id: 045-klf-klz-spec
sidebar_position: 45
title: "AD-045: KLF / KLZ Format Specification"
---

# AD-045: KLF / KLZ Format Specification

- **Scope:** neokapi framework (Go library) + `@neokapi/kapi-format` (TypeScript schema, `packages/kapi-format/`). Concerns that belong to an external tool built on top of neokapi — translation management systems (TMSs), CAT tools, custom editors, agency portals — are out of scope. Persistent content stores, interactive editor UIs, servers, sync protocols, and multi-user auth remain the responsibility of those external tools.
- **Affects:** `core/klf`, `core/klz`, `core/formats/jsx`, `packages/kapi-format/`, `neokapi/neokapi-react` (extractor), future extractors
- **Related:** [AD-044](044-klf-klz-integration.md) — neokapi's framework-scoped integration of this spec.

## Summary

Define two user-facing formats backed by one internal cache:

- **`.klf`** — Kapi Localization Format. A single JSON file holding
  one or more *documents*, each containing *blocks* in the canonical
  Block / Run model. Human-readable, git-diffable, schema-versioned,
  safe for PR review.
- **`.klz`** — Kapi Localization arcHive. A ZIP archive bundling
  one or more `.klf` files with skeletons, target overlays,
  vocabulary overrides, annotation sidecars, binary assets, and
  a signed manifest. Pure JSON + skeletons + assets + annotation
  sidecars, no databases. The transport and storage unit
  exchanged between extractors, neokapi tools, CI pipelines, and
  external tools.
- **A runtime acceleration cache** — an internal, content-addressed
  SQLite database `core/klz` builds automatically on first query
  of a given `.klz`, keyed by the SHA-256 of the archive's
  manifest and stored in `$XDG_CACHE_HOME/neokapi/klz/`. It is an
  implementation detail of `core/klz` — not a file format, not a
  user-facing artifact, never transported.

`.klf` and `.klz` are the only file formats this RFC defines.
The runtime cache is documented as an internal contract for
debuggers and framework maintainers, with the same mental model
as Go's build cache, cargo, npm, pip, or Git's pack cache: a
content-addressed local accelerator that exists because a tool
needs fast random access to an artifact it's already looking at.

## Motivation

Today neokapi-react extracts `strings.json` — a flat `{hash: text}`
dictionary. It loses all structural information: inline formatting,
JSX elements inside translatable text, variable types, plurals,
conditional expressions, Storybook links. Kapi then can't render
visual previews for JSX content the way it already does for HTML
and Markdown, can't protect inline codes the way XLIFF-based CAT
tools do, can't round-trip translations back into source.

Every extractor that plugs into kapi (neokapi-react today; Markdown,
fbt, Figma, Vue, Angular, Mozilla Fluent, gettext, and others in
the pipeline) will need the same structural vocabulary. Defining it
once, in a format every tool can produce and consume, eliminates
the O(N×M) integration cost between N extractors and M consumers.

At the same time, **neokapi tools and the external tools that
consume them need fast queries** over that data: "find me the TM
match for this source," "get block tag-chip by id without scanning
every document," "show me every segment whose source matches this
text." An MT pipeline tool that consults translation memory, a CLI
that inspects a specific block, a TMS that backs a translator's
editor by looking up segments on every keystroke — they all need
the same random-access primitives. Those are database operations,
and running them on a pile of JSON doesn't scale.

The solution is a single user-facing exchange format with clean
round-trip semantics, plus an invisible runtime cache that
accelerates queries without growing the public surface. The
exchange format must:

1. Carry enough information for a framework-level preview builder
   to render a translation unit *without* re-reading the original
   source.
2. Protect inline codes (variables, element tags, conditional
   nodes) from accidental translation or deletion.
3. Support plurals and gender via a structured multi-segment
   model, plus ICU-in-target for dynamic cases.
4. Round-trip: the same extractor that produced the archive can
   consume a translated version and reconstruct the source file.
5. Stay git-diffable: PR reviewers must be able to see "this
   source string changed, this German target is now stale" in a
   normal `git diff`.
6. Be producible by any extractor without pulling in native
   database bindings, SQLite schemas, or indexing logic.

The runtime cache must:

1. Answer random-access queries in tens of milliseconds (block by
   id, TM match by hash, similarity search by source text).
2. Scale to archives with 100k+ segments and 20+ locales.
3. Be entirely derivable from the source `.klz` — never hold
   authoritative state the `.klz` doesn't have.
4. Be invisible to users. No CLI to build, no file to gitignore,
   no path to remember, no staleness workflow.
5. Self-heal on archive changes without user intervention.
6. Evolve its schema independently of the exchange format so
   adding new runtime features (vector embeddings, usage counters,
   cached vocabulary expansions) doesn't touch the contract every
   external tool depends on.

## Guide-level explanation

### Language conventions

- **`neokapi`** — the framework library (Go, Apache 2.0).
- **`kapi`** — the CLI binary that ships with neokapi and
  demonstrates framework operations from the command line.
- **`@neokapi/react`** — an extractor package, consumed by
  neokapi through the Extractor interface (§Extractor interface).
- **"an external tool"** or **"a TMS"** (translation management
  system) — any application built on top of neokapi, such as a
  TMS, a CAT tool, an IDE extension, or an agency portal.

### File naming conventions

A single `.klz` is multilingual by default. Each `Block` carries a
shared `source` plus an optional `targets: map[LocaleID]Run[]`, so
one archive accumulates as many target locales as the project
needs. Tool commands (`kapi ai-translate`, `kapi pseudo-translate`,
`kapi qa`, …) read a `.klz`, append the target locale they're
producing, and write back to the **same file** by default — the
writer is locale-additive, so existing targets stay put.

The canonical layout is therefore one file per project:

```
i18n/myproject.klz    # source + every target locale
```

Per-locale bilingual files (`myproject.fr.klz`, `myproject.de.klz`)
remain valid and useful for parallel-translator workflows where
per-locale PRs or CAT-tool scoping matters, but they're opt-in.
When multiple translators edit the same multilingual archive
concurrently a `kapi split` / `kapi merge` helper pair is the
recommended escape valve.

The file naming convention is therefore:

- `<name>.klz` — canonical, multilingual (source + 0..N targets).
- `<name>.<locale>.klz` — bilingual (source + one target locale)
  for parallel-translator workflows.

Extension stays `.klz` in both cases; the manifest inside tells
you which locales are present. There is no separate "template"
extension — a `.klz` with no targets *is* the template.

### Lifecycle from a framework view

A developer working on a React app runs:

```bash
kapi extract -p project.kapi       # project-driven
# or, without a .kapi:
vp kapi-react extract --stream | kapi pack --out i18n/myproject.klz
# or, for git-diffable intermediate state:
vp kapi-react extract --out i18n/klf/
kapi pack --in i18n/klf/ --out i18n/myproject.klz
```

`@neokapi/kapi-react` is a neokapi `Extractor`: it walks the
source tree, produces Blocks for every translatable element, and
either writes one `.klf` per source document (default) or
emits NDJSON block records on stdout (`--stream`). The
project's `.kapi` declares the extractor via
`format: { name: exec, config: { command } }`; kapi owns
`.klz` assembly. The archive is self-contained — the developer
can commit it, ship it, email it, feed it to any neokapi tool,
or hand it to a downstream TMS or CAT tool.

**The `.klz` is pure JSON and skeletons.** Nothing in it needs
SQLite; nothing in it is binary except opaque asset files and
extractor-owned skeleton blobs. The extractor never writes a
database. A CI pipeline that verifies the archive never opens a
database. A translation agency receiving it never opens a
database. Trust and portability are straightforward.

When a neokapi tool processes the archive — streaming through it
to run MT, validating placeholders, inspecting a specific block
by id, looking up similar past translations — it uses
`core/klz` directly:

```go
reader, _ := klz.NewReader(input)

// Pure Block iteration — no cache involvement, no SQLite touched.
for block := range reader.Blocks(ctx) { /* ... */ }

// Random-access query — transparently warms a local cache on
// first call and hits it on every subsequent call in this
// process or any other process using the same .klz.
matches, _ := reader.SimilarSources(ctx, "items cart", "de", 5)
block, _   := reader.BlockByID(ctx, "tag-chip")
tm        := reader.TM()
```

A tool that only iterates Blocks (producers, validators,
pseudo-translator, MT stages) never triggers the cache. A tool
that asks for random-access queries warms it on first call. The
user has no way to observe which path ran, and no CLI or Go API
to build or refresh the cache explicitly — warming is always
transparent.

The runtime cache is **content-addressed** by a SHA-256 of the
archive's `manifest.json`. If the `.klz` changes — new
extraction, added target overlay, mutated skeleton — its
manifest hash changes, a fresh cache entry is built on the next
query, and the stale entry ages out under normal cache GC.
Staleness is a non-issue: the cache key is the staleness check.

Extractors feed the archive via `kapi extract -p project.kapi`.
Collections declare extraction through the standard `FormatSpec`
with `name: exec` and a `command` in `config`; kapi runs the
command with NUL-separated paths on stdin, parses NDJSON block
records from stdout, and packs the archive. kapi owns `.klz`
writing; extractors only emit blocks. The protocol is specified
in `core/formats/exec/reader.go`. The developer's package
manager drives invocation — `vp`, `pnpm`, `npm`, `yarn`, or a
direct binary path; kapi makes no assumptions.

An alternative standalone flow produces the same shape without
a `.kapi`:

```bash
vp kapi-react extract --stream | kapi pack --out i18n/ui.klz
```

Or, for debugging / git-diffable intermediate state:

```bash
vp kapi-react extract --out i18n/klf/   # writes per-file *.klf
kapi pack --in i18n/klf/ --out i18n/ui.klz
```

Each translation tool then writes the requested target locale
back into the same archive:

```bash
kapi ai-translate i18n/myproject.klz --target-lang fr
kapi ai-translate i18n/myproject.klz --target-lang de
kapi pseudo-translate i18n/myproject.klz --target-lang qps
```

The writer preserves every existing target and appends (or
updates) the requested one, so the natural workflow is to
accumulate locales in a single `.klz` rather than fanning out
to per-invocation output files. `-o` is available when an
explicit redirect is wanted, but omitting it is the common case.

Later, the developer runs:

```bash
kapi klz merge i18n/myproject.klz --locale de --out src-de/
```

`kapi klz merge` reads `manifest.generator.id` → `@neokapi/react`,
looks it up in the extractor registry, and delegates to the
extractor's `Merge()` method. The extractor loads the stored
skeleton files, replays transformations with the German target
strings, and writes out a translated source tree. Merge never
touches the cache; it only reads the `.klz`.

### Where external tools come in

An external tool (TMS like Bowrain, CAT tool, in-house editor,
agency portal, IDE extension) imports `core/klz` to read
incoming archives, calls its query helpers when it wants
runtime acceleration, wraps neokapi's reader/writer/preview
machinery in whatever editor or server surface it chooses to
offer, and emits a new `.klz` when it wants a portable snapshot.
The framework provides the reader, the writer, the query
primitives, and the CLI; what the external tool builds on top
is its own architecture, and out of scope here.

## Reference-level explanation

### The canonical data model

Defined in `@neokapi/kapi-format/src/block.ts`. Normative for this RFC:

- **Block** — the unit of translation tracking. Holds
  `source: Run[]`, optional `targets: Record<LocaleID, Run[]>`,
  `placeholders: Placeholder[]`, `properties` (file path, jsxPath,
  component), optional `preview` hints. TM lookups, target
  storage, validation, and annotations are all keyed per Block
  (per locale where applicable). Block.type is one of
  `jsx:element` or `jsx:attribute` — there is no separate
  plural-group or select-group Block kind.
- **Run** — one element of a Block's content sequence. A
  discriminated union:
  - `{ text: string }` — a text chunk.
  - `{ ph: … }` — a self-closing placeholder (variable,
    conditional JSX expression, `<br/>`, redaction, icon, …).
  - `{ pcOpen: … }` / `{ pcClose: … }` — the opening and
    closing halves of a paired code (an inline element wrapping
    content). Matching `id` fields link the pair inside the same
    runs scope; `pcClose` repeats `type` and `subType` for
    locality.
  - `{ sub: … }` — a reference to a subblock, used for
    sub-filter output where an outer format captures a field
    whose value is itself a mini-document in another format.
  - `{ plural: { pivot, forms } }` — a structured plural
    construct. `pivot` names the variable that drives selection;
    `forms` is a `Partial<Record<PluralForm, Run[]>>` mapping
    each plural form ('zero' / 'one' / 'two' / 'few' / 'many' /
    'other') to its own run sequence. Inline codes inside a
    plural form are first-class typed runs (pcOpen/pcClose, ph,
    nested plural) with their own ID scope per form.
  - `{ select: { pivot, cases } }` — a structured select
    construct, symmetric to `plural` but keyed by arbitrary
    string values.
- **Placeholder** — metadata about the variables and element
  tokens referenced anywhere in the block's runs, including
  inside plural / select forms. Drives validation (target must
  preserve every required placeholder) and gives tools jsType /
  sourceExpr / optional / icu-pivot flags that don't fit on a
  Run. Pivot variables for plural / select constructs are
  declared here with kind `icu-pivot`.
- **ExtractedDocument** — top-level wrapper. Declares schema
  version, source locale, source file path, document type, and
  `blocks[]`.

See `@neokapi/kapi-format`'s `src/block.ts` for the full definitions.
This RFC specifies how that model is serialized; the types
specify the in-memory shape.

The run sequence is deliberately flat at the block level.
Paired codes use matching `id`s to identify their halves, the
way XLIFF 2.0's `<pc>` and TMX's `<bpt>/<ept>` work. The only
recursion in the model is inside `plural` and `select` runs,
where each form or case holds its own `Run[]`; this recursion
is scoped to exactly the blocks that contain a plural or
select construct. Simple blocks have perfectly flat runs with
no nesting. Internal processing in `core/klz` is free to
materialize a coded-string form with PUA markers on demand for
hot-path operations, but that form stays inside the hot path
and never becomes part of the wire format.

#### Why structured plural / select instead of ICU-in-text

A plural could be encoded as a single text run containing ICU
MessageFormat syntax — `{count, plural, one {…} other {…}}` —
and the runtime already supports parsing ICU at render time.
That encoding is simpler at the schema level but loses type
information for inline markup inside plural clauses: a `<span>`
inside `one` would become literal characters rather than a
typed `pcOpen`/`pcClose` pair, forfeiting chip rendering,
validation, and CAT-tool protection for that markup.

The structured `plural` / `select` run types preserve typed
inline codes inside their sub-sequences at the cost of one
level of recursion in the run walker, and only in blocks that
actually contain a plural or select construct. For the 99%+ of
blocks that don't, the runs sequence is perfectly flat.

The structured form also aligns with MessageFormat 2.0's
approach (structured variant arrays) and Fluent's AST
(`SelectExpression` nodes with variant children), which is
where serious localization formats are converging.

The wire format can still serialize a structured plural to an
ICU MessageFormat string as an export adapter, and the
flat-runtime-dict compilation step at build time can produce
ICU-text strings for `@neokapi/react`'s existing runtime to
consume. Wire is structured; the runtime consumer form is
whatever the runtime wants.

### `.klf` file format

A `.klf` file is a UTF-8 encoded JSON document with a top-level
object whose shape is:

```jsonc
{
  "schemaVersion": "1.0",
  "kind": "kapi-localization-format",
  "created": "2026-04-15T10:00:00Z",
  "generator": {
    "id": "@neokapi/react",
    "version": "0.7.0",
    "capabilities": ["extract", "merge", "preview-skeleton"]
  },
  "project": {
    "id": "kapi-desktop",
    "sourceLocale": "en"
  },
  "vocabulary": {
    "extends": ["common-formatting", "rich-html", "rich-jsx"]
  },
  "documents": [
    {
      "id": "src/TagChip.tsx",
      "documentType": "jsx",
      "path": "src/TagChip.tsx",
      "sourceHash": "sha256:c9f3…",
      "skeleton": { "ref": "skeletons/src-TagChip-tsx.skl" },
      "blocks": [ /* Block[] */ ]
    }
  ]
}
```

Required top-level fields: `schemaVersion`, `kind`, `generator`,
`project`, `documents`. Others are optional and ignored by
unaware readers.

Two framings of `.klf` are recognized:

1. **Standalone `.klf`** — a single JSON file complete on its own.
   Skeletons MUST be inlined (`skeleton.inline: "..."`). Used for
   small one-shot exchanges, unit test fixtures, and PR review
   diffs.
2. **Part-of-`.klz` `.klf`** — one JSON file inside a `.klz`
   archive. Skeletons MAY be referenced by relative path inside
   the same archive.

A producer MUST declare which framing it is using via the
presence or absence of a containing `.klz`. A consumer MUST
accept both.

### `.klz` exchange archive

A `.klz` file is a ZIP archive (ISO/IEC 21320-1 compliant, DEFLATE
or STORE compression) with the following well-known part layout:

```
project.klz
├── manifest.json                  REQUIRED   archive manifest
├── meta.json                      OPTIONAL   project metadata
├── documents/*.klf                REQUIRED   one source .klf per document
├── skeletons/*                    OPTIONAL   opaque per-extractor skeletons
├── targets/{locale}/*.klf         OPTIONAL   sparse target overlays
├── targets/{locale}/status.json   OPTIONAL   per-block translation status
├── vocabulary/*.json              OPTIONAL   vocabulary entries/overrides
├── annotations/*.klfl             OPTIONAL   non-authoritative analytical sidecars
├── assets/**                      OPTIONAL   screenshots, audio, video
└── signatures/manifest.sig        OPTIONAL   ed25519 signature
```

**What is deliberately NOT inside a `.klz`:** no SQLite files,
no binary indexes, no translation memory databases, no derived
runtime caches. Every part of a `.klz` is either authoritative
(`documents/`, `targets/`, `vocabulary/`, `skeletons/`,
`assets/`), an opaque extractor skeleton (`skeletons/`), or a
non-authoritative analytical sidecar (`annotations/`). The
runtime acceleration cache lives in neokapi's local cache
directory, never inside the archive.

Rules:

- All paths MUST be POSIX relative paths with no leading slash,
  no `..`, no absolute roots. Consumers MUST reject archives
  containing entries that would resolve outside the archive root
  (ZIP slip).
- Path components MUST be NFC-normalized UTF-8. Case-sensitive.
- Part filenames SHOULD use kebab-case with `.klf`/`.skl`/etc.
  extensions that match the part's content type.
- Empty directories MUST NOT be stored; absence of a part is the
  same as its nonexistence.

### `.klz` manifest schema

`manifest.json` is required at the archive root and has this
shape:

```jsonc
{
  "kapiLocalizationFormat": "1.0",
  "created": "2026-04-15T10:00:00Z",
  "generator": {
    "id": "@neokapi/react",
    "version": "0.7.0"
  },
  "project": {
    "id": "kapi-desktop",
    "sourceLocale": "en",
    "targetLocales": ["de", "ja", "qps"]
  },
  "parts": [
    {
      "path": "documents/src-TagChip-tsx.klf",
      "sha256": "abc123…",
      "size": 4821,
      "role": "document",
      "attributes": { "documentId": "src/TagChip.tsx" }
    },
    {
      "path": "skeletons/src-TagChip-tsx.skl",
      "sha256": "def456…",
      "size": 1247,
      "role": "skeleton"
    }
  ]
}
```

`parts[].role` is an enum: `document` · `target` · `skeleton` ·
`vocabulary` · `asset` · `signature` · `meta` · `annotation`.
Readers SHOULD use this as a fast-path index before inflating
parts. Annotation parts are non-authoritative and may be
skipped by consumers that don't understand their annotation
type (see §Annotation sidecars).

### Runtime acceleration cache (implementation detail)

This subsection documents the internal SQLite layer `core/klz`
uses to answer random-access queries. It is not a file format:
it is a content-addressed local cache entry, rebuilt on demand
from the source `.klz`. External tools interact with
`core/klz`'s Go API — not with the SQLite file.

The subsection is documented here so that neokapi maintainers
can understand the contract between `core/klz`'s query methods
and the storage layer, and so that debuggers can `sqlite3` a
cache entry directly when something goes wrong.

#### Where the cache lives

Neokapi stores cache entries in a per-user cache directory:

- **Linux:** `$XDG_CACHE_HOME/neokapi/klz/` (or `~/.cache/neokapi/klz/` if unset)
- **macOS:** `~/Library/Caches/neokapi/klz/`
- **Windows:** `%LOCALAPPDATA%\neokapi\klz\`

Inside that directory, entries are sharded by the first two hex
characters of the cache key (git-style) to keep the per-directory
entry count manageable:

```
$XDG_CACHE_HOME/neokapi/klz/
├── ab/
│   └── cd1234…/               # two-char shard + remaining hash
│       ├── db.sqlite          # the runtime DB
│       ├── source.path        # absolute path to the .klz that built this
│       ├── built.at           # RFC3339 timestamp
│       └── lock               # flock for concurrent access
```

#### The cache key

The cache key is `sha256(raw_bytes_of(manifest.json))` — the
SHA-256 of the raw bytes of `manifest.json` as stored in the ZIP,
before any parsing, normalization, or re-serialization. This is
stable, fast to compute, and naturally sensitive to any
authoritative change in the archive (because `manifest.json`
already carries per-part SHA-256s over every authoritative part).

Consequences:

- **Two `.klz` files with identical content share a cache
  entry.** If the same archive is opened from two paths, the
  second open is free.
- **Any modification to the `.klz` causes a new cache entry.**
  Old entries become unreachable and get collected by the GC
  policy.
- **There is no separate staleness check.** The lookup-by-hash
  IS the staleness check. If `core/klz` finds an entry at the
  current hash, it is fresh by construction.

#### Cache lifecycle

```go
// Sketch of what core/klz does under the hood; not public API.

func (r *Reader) queryPath() (*db, error) {
    hash := r.manifestHash()           // sha256(manifest.json bytes)
    dir  := cacheDir(hash)             // $XDG_CACHE_HOME/neokapi/klz/ab/cd…/

    if exists(dir / "db.sqlite") {
        return openExisting(dir)        // fast path
    }

    // Slow path: build atomically under a temp dir, rename into place.
    // Concurrent builds in two processes are safe because of the final
    // rename's atomicity — the first process to finish wins; the second
    // discards its temp and opens the winner.
    tmp := mkdirTemp(cacheRoot, "build-*")
    buildDB(r, tmp / "db.sqlite")
    writeMeta(tmp, r.source, time.Now())
    if err := os.Rename(tmp, dir); err != nil && !errors.Is(err, fs.ErrExist) {
        return nil, err
    }
    return openExisting(dir)
}
```

Key invariants:

- **Cache entries are immutable once written.** Every row in the
  SQLite file is derivable from the source `.klz`. Neokapi never
  mutates an entry after its atomic rename (with the exception
  of consumer-private tables — see below).
- **Atomic builds.** A build that crashes halfway leaves its
  temp dir behind, never a half-written cache entry.
- **Parallel-safe.** Multiple processes querying the same
  `.klz` concurrently may race to build the entry; the rename
  ensures exactly one winner and the losers discard their temp.

#### SQLite schema (internal contract)

The schema is **internal to neokapi**. It evolves with the
framework. External tools MUST NOT depend on it; they use
`core/klz`'s Go API instead. This subsection exists so that
neokapi maintainers and debuggers have a documented reference
when they open a cache entry with `sqlite3`.

```sql
-- Every unique (source_locale, source_hash, context) tuple gets
-- one row. Populated from every Block's source runs on build.
-- A Block is one row regardless of whether it contains plural or
-- select constructs; those are represented inside source_runs
-- JSON as structured runs.
CREATE TABLE sources (
  id INTEGER PRIMARY KEY,
  source_locale TEXT NOT NULL,
  source_hash TEXT NOT NULL,
  source_text TEXT NOT NULL,     -- flattened plain text, for FTS and LLM prompts
  source_runs TEXT NOT NULL,     -- JSON array of Run objects
  context TEXT,                  -- jsxPath or equivalent
  block_type TEXT,               -- 'jsx:element' or 'jsx:attribute'
  created_at INTEGER NOT NULL,
  UNIQUE (source_locale, source_hash, context)
);

-- Each (source, locale) pair can have one target. Populated from
-- every target overlay on build. A target is a whole Block's
-- run sequence; plural/select target content lives inside that
-- sequence as structured runs, independently of the source
-- plural structure.
CREATE TABLE targets (
  id INTEGER PRIMARY KEY,
  source_id INTEGER NOT NULL REFERENCES sources(id) ON DELETE CASCADE,
  locale TEXT NOT NULL,
  target_runs TEXT NOT NULL,     -- JSON array of Run objects
  status TEXT NOT NULL CHECK (
    status IN ('new','translated','reviewed','signed-off','rejected')
  ),
  origin TEXT NOT NULL,          -- 'human' | 'mt' | 'llm' | 'tm' | 'import'
  origin_detail TEXT,            -- JSON: engine, model, confidence, etc.
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL
);

CREATE UNIQUE INDEX idx_targets_active ON targets(source_id, locale);
CREATE INDEX idx_sources_hash         ON sources(source_hash);
CREATE INDEX idx_targets_locale       ON targets(locale);

-- FTS5 for fuzzy / similar-source lookups used by core/klz's
-- SimilarSources() query helper.
CREATE VIRTUAL TABLE sources_fts USING fts5(
  source_text,
  content='sources',
  content_rowid='id'
);

-- Block-level index for random access by id.
CREATE TABLE blocks (
  id TEXT PRIMARY KEY,
  document_path TEXT NOT NULL,
  hash TEXT NOT NULL,
  type TEXT NOT NULL,
  component TEXT,
  jsx_path TEXT,
  optional_placeholders INTEGER NOT NULL DEFAULT 0,
  required_placeholders INTEGER NOT NULL DEFAULT 0
);

CREATE INDEX idx_blocks_document  ON blocks(document_path);
CREATE INDEX idx_blocks_hash      ON blocks(hash);
CREATE INDEX idx_blocks_component ON blocks(component);

-- Source-hash dedup: map from a content hash to every block that
-- shares it.
CREATE TABLE source_hashes (
  source_hash TEXT PRIMARY KEY,
  block_ids TEXT NOT NULL   -- JSON array of block IDs sharing this hash
);

-- Cache metadata. The schema version lives here so neokapi can
-- detect old-schema cache entries after a framework upgrade and
-- rebuild them on next access. No migration SQL is ever written;
-- stale cache entries are discarded and re-derived.
CREATE TABLE cache_meta (
  key TEXT PRIMARY KEY,
  value TEXT NOT NULL
);
INSERT INTO cache_meta(key, value) VALUES
  ('cache_schema_version', '1'),
  ('source_klz_manifest_sha256', 'abc123…'),
  ('built_at', '2026-04-15T10:00:00Z'),
  ('built_by', 'neokapi v1.0 core/klz');
```

#### Schema evolution

When neokapi bumps `cache_schema_version`:

1. Existing cache entries become unreadable by the new framework.
2. The next query that reads them sees the version mismatch and
   discards the entry.
3. A fresh entry is built from the source `.klz` with the new
   schema.
4. The old entry is left on disk until cache GC sweeps it up.

No migration SQL is ever written. No tool has to handle two
schemas at once. The `.klz` is always authoritative; the cache is
always derivable.

#### Consumer-private tables

A neokapi-based tool MAY add its own tables to a cache entry
under a namespace prefix if it wants to persist per-archive
local state (cursor positions, review notes, filter chips, MT
engine cache, etc.):

```sql
-- Hypothetical tool-private tables, ignored by the framework
-- and by any other consumer. The "acme_" prefix is whatever the
-- owning tool chooses as its unique namespace.
CREATE TABLE acme_editor_state (
  scope TEXT PRIMARY KEY,
  value TEXT NOT NULL
);
```

Consumer-private tables have specific semantics worth calling
out:

1. **Namespacing is mandatory.** A consumer MUST use a prefix
   unique to it (`acme_`, `vscode_`, etc.). Two consumers with
   colliding prefixes corrupt each other.
2. **They're wiped on rebuild.** Because the cache is
   content-addressed, any change to the source `.klz` produces a
   new cache entry with no consumer tables at all. Consumers
   that need state to survive `.klz` changes MUST persist it
   elsewhere — typically alongside the source `.klz` in the
   consumer's own project directory.
3. **The framework never reads them.** Neokapi's own code only
   touches the framework-level tables (`sources`, `targets`,
   `blocks`, `sources_fts`, `source_hashes`, `cache_meta`).
   Consumer-private tables are invisible to the framework.
4. **They're still ephemeral.** Because the whole cache entry can
   be GC'd by neokapi's cache manager at any time, consumer state
   stored here is a hot cache, not a persistence layer.

### Extractor interface

External tools that produce `.klz` implement the `Extractor`
interface. It lives in `@neokapi/kapi-format` on the TypeScript side
and as a small Go interface in `core/format`:

```go
type Extractor interface {
    ID() string
    Version() string
    Capabilities() []string
    Supports(file string) bool
    Extract(ctx context.Context, in ExtractInput) (*ExtractResult, error)
    // Optional — only extractors that can round-trip implement this.
    Merge(ctx context.Context, in MergeInput) (*MergeResult, error)
}
```

Registration happens through `format.RegisterExtractor`. The
`kapi` CLI's `klz merge` subcommand looks up an extractor by
`generator.id` in the archive manifest and delegates.

Extractors emit `.klz` only. They do not know or care about the
runtime cache; they never include SQLite bindings; they never
reason about staleness or rebuild workflows. That's a hard
boundary: if an extractor needs SQLite, it is doing something
wrong.

### Skeleton ownership

Skeletons inside `.klz` are opaque bytes to the framework. The
`klz.Reader` and `klz.Writer` store and retrieve them with
per-file integrity checks but never parse them. Interpretation is
the responsibility of the owning extractor's `Merge()`
implementation.

For neokapi-react the skeleton SHOULD be either (a) the original
TSX source file verbatim, or (b) a serialized list of
`TransformOp` (`{offset, deleteCount, insert}`) that can be
replayed against the source. The choice is up to the extractor.

### Annotation sidecars

Annotations are **non-authoritative analytical overlays** on a
Block/Run graph. They describe properties of content — protected
terms, glossary matches, review status, MT confidence, QA flags,
translator notes, provenance records — without changing how the
content is stored, edited, or served back. A consumer that
doesn't understand an annotation type MUST ignore it and process
the authoritative content correctly.

Annotations live as sidecar files under the archive's
`annotations/` directory:

```
project.klz
└── annotations/
    ├── @neokapi-term-detector.klfl
    ├── bowrain-review-status.klfl
    ├── deepl-mt-confidence.klfl
    └── acme-glossary-v2.klfl
```

One file per *producer namespace*. Multiple files coexist
without coordination — each producer owns its own file, writes
freely, and ignores all others. No central registry, no
cross-file merge semantics.

#### File format

Each annotation file is **JSON Lines** with UTF-8 encoding. The
first line is a header record; every subsequent line is one
annotation record. This framing is streaming-friendly (producers
append as they discover annotations; consumers stream line by
line with bounded memory), grep-friendly (one annotation per
line), and diff-friendly (changed annotations show as changed
lines in git).

The header record declares the annotation type, the producer,
and the archive state the annotations were produced against:

```json
{
  "type": "header",
  "annotationType": "@neokapi/term-detector",
  "annotationVersion": "1.0.0",
  "producer": { "id": "@neokapi/term-detector", "version": "1.2.3" },
  "created": "2026-04-15T10:30:00Z",
  "targetArchive": "sha256:abc123…"
}
```

`targetArchive` is the SHA-256 of the source `.klz`'s
`manifest.json` at the time the annotations were produced.
Consumers compare this against the current manifest hash to
detect potentially-stale annotations.

`annotationType` is **namespaced** (`@neokapi/term-detector`,
`acme/glossary-v2`, `bowrain/review-status`, …). There is no
central registry; tools that understand a namespace consume the
file, tools that don't skip it. Namespacing is the only
coordination mechanism.

Subsequent lines are annotation records:

```json
{
  "type": "annotation",
  "id": "term-1",
  "anchor": { "kind": "range", "block": "welcome", "path": [3], "offset": 9, "length": 6 },
  "data": { "kind": "protected-term", "term": "iPhone", "action": "do-not-translate" }
}
```

Each record has an `id` (stable within the file, not required to
be globally unique), an `anchor` locating the target in the
archive's blocks, and a `data` object carrying the
annotation-type-specific payload. The framework imposes no
schema on `data`; producers and consumers negotiate via the
`annotationType` namespace.

#### Anchor shapes

Four anchor kinds, discriminated by `kind`, cover the practical
targeting needs:

**`block`** — targets a whole block. Used for metadata that
applies to the block as a unit:

```json
{ "kind": "block", "block": "tag-chip" }
```

Typical payloads: review status, MT confidence, "contains PII"
flags, cross-cutting classifications.

**`run`** — targets a specific `ph`, `pcOpen`, or `sub` run
inside a block by walking a path through the block's runs and
confirming the run's `id`:

```json
{
  "kind": "run",
  "block": "tag-chip",
  "path": [2],
  "runId": "2"
}
```

`path` is an array of steps (see below). `runId` is the target
run's `id` field, used by validators to confirm that the anchor
still points at the same run after potential re-extraction.

Typical payloads: "this placeholder is a brand name", "this
inline link is a glossary match", "this element should not be
reordered in translation".

**`range`** — targets a character range inside a text run:

```json
{
  "kind": "range",
  "block": "welcome",
  "path": [3],
  "offset": 9,
  "length": 6
}
```

`path` must land on a `text` run. `offset` and `length` are in
UTF-16 code units into the run's `text` field. Range anchors
are the most fragile (any text edit shifts offsets), so
producers are expected to re-run after content changes.

Typical payloads: term-detector matches, URL detections, regex
hits, named-entity tags.

**`form`** — targets a specific plural form or select case
within a block's plural/select run:

```json
{
  "kind": "form",
  "block": "shopping-cart-plural",
  "path": [0],
  "key": "other"
}
```

`path` must land on a `plural` or `select` run; `key` picks one
form (for plural) or case (for select).

Typical payloads: per-form review status, per-form MT
confidence, per-form QA flags.

#### Run paths

A `RunPath` is an array of `RunPathStep`s describing how to
navigate from a block's `source` (or a target locale's run
sequence) to a specific run. Steps come in three shapes:

- **`number`** — index into a `Run[]` sequence. The top of the
  path starts at `block.source`, so `[0]` means "the first run
  in the block's source".
- **`{ "plural": "<form>" }`** — step into a `plural` run's
  form. Must follow a path step that landed on a plural run.
- **`{ "select": "<value>" }`** — step into a `select` run's
  case.

Example: `[0, { "plural": "one" }, 2]` reads as "the 3rd run
inside the 'one' form of the first top-level run of the block's
source, which must be a plural run". If any step is out of
bounds or the wrong kind, the anchor is orphaned (see
"Validation" below).

Empty path `[]` refers to the containing context — for a
`block` anchor it's the block itself; it's not meaningful for
the other kinds and MUST be rejected.

#### Example payloads

Protected term (range anchor):

```json
{
  "type": "annotation",
  "id": "term-17",
  "anchor": { "kind": "range", "block": "welcome", "path": [3], "offset": 9, "length": 6 },
  "data": {
    "kind": "protected-term",
    "term": "iPhone",
    "termbaseEntry": "brand-apple-iphone",
    "action": "do-not-translate",
    "confidence": 1.0
  }
}
```

Review status (block anchor):

```json
{
  "type": "annotation",
  "id": "review-42",
  "anchor": { "kind": "block", "block": "tag-chip" },
  "data": {
    "kind": "review",
    "locale": "de",
    "status": "approved",
    "reviewer": "alice@example.com",
    "approvedAt": "2026-04-15T11:00:00Z"
  }
}
```

MT confidence on a specific plural form (form anchor):

```json
{
  "type": "annotation",
  "id": "mt-99",
  "anchor": {
    "kind": "form",
    "block": "shopping-cart-plural",
    "path": [0],
    "key": "other"
  },
  "data": {
    "kind": "mt-confidence",
    "locale": "de",
    "engine": "deepl",
    "model": "v2",
    "confidence": 0.87
  }
}
```

QA flag on a specific placeholder (run anchor):

```json
{
  "type": "annotation",
  "id": "qa-3",
  "anchor": {
    "kind": "run",
    "block": "tag-chip",
    "path": [2],
    "runId": "2"
  },
  "data": {
    "kind": "qa",
    "severity": "warning",
    "rule": "placeholder-overused",
    "message": "label is referenced twice; ensure consistent translation"
  }
}
```

#### Lifecycle rules

1. **Non-authoritative.** Losing an annotation file costs only
   regeneration. The authoritative content in `documents/` is
   independent of every annotation.
2. **Layered.** Multiple producers coexist in the same archive.
   No merge semantics between annotation files; each is its own
   concern.
3. **Never mutate authoritative content.** Annotation producers
   are read-only on `documents/`, `targets/`, `vocabulary/`,
   and `skeletons/`. They write only to their own file in
   `annotations/`.
4. **Namespaced by producer.** Annotation type identifiers use
   a namespace prefix (`@vendor/name`, `org/name`). Consumers
   match by prefix and skip unknown namespaces.
5. **Rebuildable by re-running producers.** When a `.klz`
   changes, producers re-run against the new archive and
   overwrite their file. No migration, no partial updates.
6. **Stale detection via `targetArchive` + anchor resolution.**
   Consumers compare the file's `targetArchive` field against
   the current archive's manifest hash, and can fall back to
   re-resolving every anchor to detect which records still
   point at valid content.

#### Validation

Validators that process annotation files SHOULD:

1. Parse the header; reject malformed headers.
2. For each annotation record, resolve the anchor against the
   target block:
   - `block` anchor: block exists?
   - `run` anchor: path resolves to a run with the recorded
     `runId`?
   - `range` anchor: path resolves to a text run, and
     `offset + length` is within the text?
   - `form` anchor: path resolves to a plural/select run, and
     `key` matches a present form/case?
3. Flag unresolvable anchors as **orphans**. Orphans are not
   necessarily errors — they may be normal after a source
   edit — but they should be reported so producers can re-run.
4. Refuse to apply annotation data in a way that mutates
   authoritative content. Annotations are consultative.
5. When a `.klz` is being regenerated, orphaned annotation
   records MAY be dropped silently (the producer will emit
   fresh ones on its next run).

#### Security

Annotation files follow the same trust rules as every other
`.klz` part:

- ZIP slip rejection (paths must stay inside the archive).
- Part-size limits apply.
- No executable content — annotation payloads are data, not
  code.
- `targetArchive` SHOULD be verified against the current
  manifest hash before accepting an annotation file as fresh.

Annotations are always applied in an advisory capacity. A
producer cannot compromise a consumer by writing annotations
that claim to modify content; consumers that honor this rule
are safe by construction.

#### What this doesn't cover (future extensions)

- **Inline annotations.** Adding a `mrkOpen`/`mrkClose` run pair
  to the Run union, analogous to `pcOpen`/`pcClose`, would let
  annotations live inline in the authoritative runs (like
  XLIFF 2.0's `<mrk>`). That's a possible future extension,
  specified in a follow-up RFC if/when a use case calls for
  annotations that travel with the content rather than as
  sidecars.
- **Cross-block annotations.** Annotations that link multiple
  blocks together (e.g., "these three blocks are alternative
  phrasings of the same concept") aren't expressible with the
  four anchor kinds above. If needed, a future anchor kind
  could support multi-block references.
- **Annotation merge rules.** When two producers want to
  collaborate on the same concern (e.g., two term detectors
  contributing to the same glossary overlay), they need
  per-producer files with manual reconciliation today. Future
  work could define a merge protocol, but the current spec
  treats each file as independent.

### Versioning

Three independent version numbers, all serving different audiences:

1. **`schemaVersion`** (in `.klf`) and **`kapiLocalizationFormat`**
   (in `.klz` manifest) — the exchange format's version, visible
   to everyone. Of the form `MAJOR.MINOR`. Major bumps break wire
   compatibility; minor bumps are additive. A consumer MUST
   reject unknown major versions cleanly and MUST accept unknown
   minor versions of its major.
2. **`generator.version`** — the producing extractor's version.
   Independent of the format version. Consumers that care about
   extractor-specific quirks may branch on this; the format
   library does not.
3. **`cache_schema_version`** (in the runtime cache's
   `cache_meta` table) — **entirely internal to neokapi**, not
   part of any public contract. Bumped whenever the SQLite schema
   changes. Because cache entries are always rebuildable from
   the source `.klz`, a schema bump just invalidates existing
   entries on the local machine; there is no migration story.

Forward-compatibility contract for the exchange format: unknown
fields in any JSON object MUST be ignored by readers. Writers
MUST NOT re-emit fields they did not understand (so unknown
fields from a newer producer do NOT leak into older-version
re-exports).

### Security

The security story is straightforward because only one artifact
crosses trust boundaries.

#### `.klz` trust model

`.klz` files cross trust boundaries freely — extractor → CI →
neokapi → agency → back. The format is designed to be safely
inspectable from an untrusted source:

- **ZIP slip.** Parts with `..` components or absolute paths MUST
  be rejected. Consumers MUST normalize paths before extracting.
- **Part size limits.** Consumers SHOULD enforce a maximum
  inflated part size (default: 128 MiB per part, 2 GiB total) to
  prevent decompression bombs.
- **No executable content.** Parts are JSON, opaque skeleton
  blobs, and asset files. Skeleton blobs are never executed —
  they are consumed only by the extractor named in the manifest,
  and only via its own `Merge()` entry point.
- **Signatures.** `signatures/manifest.sig` is an optional
  ed25519 signature over the canonicalized bytes of
  `manifest.json`. Verifying key discovery is out of scope for
  v1 (deferred to a future RFC); the slot exists so adding
  signing later doesn't require a format bump.
- **Trusted generators.** Neokapi MAY maintain a
  trusted-generator allowlist per project; unknown generators
  are still readable but merge operations MAY be gated behind a
  user confirmation.

#### Runtime cache trust model

The cache is structurally impossible to transport:

- It lives in a per-user cache directory keyed by a content
  hash. Moving it between machines achieves nothing — the target
  machine's `core/klz` will just rebuild it from the local
  `.klz` on next access.
- Neokapi MUST refuse to treat a cache entry as authoritative.
  Any query that can't be satisfied from the entry's invariants
  MUST fall back to reading the source `.klz` and rebuilding.
- A user who suspects a cache entry is corrupt can delete the
  entry (or the entire cache) and neokapi rebuilds on next use.
  Neokapi exposes this via `kapi cache clear` (see
  `neokapi-integration.md`).

Because the cache is never transported and never authoritative,
an attacker who wants to compromise a user's translation
pipeline must compromise a `.klz` the user trusts — the cache
is not a new attack surface.

## Drawbacks

1. **The SQLite dependency still exists for consumers that want
   query acceleration.** `core/klz`'s query methods pull in a
   SQLite binding. Mitigated by: producers don't need one (they
   only write `.klz`); CI validation tools that only run
   `placeholder-check` don't need one; and neokapi already uses
   SQLite in its existing `core/storage` layer for TM and
   terminology, so the binding is already in the framework's
   dependency graph.
2. **Cache size grows over time.** A user who touches many
   `.klz` archives accumulates cache entries. Mitigated by a
   `kapi cache gc` subcommand with LRU + size-limit semantics,
   documented in `neokapi-integration.md`.
3. **First query latency.** The first query against a fresh
   `.klz` pays the build cost (tens to low hundreds of ms for
   typical projects). Mitigated by the fact that the build only
   happens once per `.klz` content, and subsequent queries are
   O(milliseconds).
4. **Not all concerns fall cleanly on one side.** Things like
   vocabulary overrides and preview hints are authoritative (so
   they live in `.klz`) but are also frequently read during
   query-intensive work (so they also get materialized into the
   cache). That's a small duplication — the `.klz` stays the
   source of truth.

## Rationale and alternatives

### Pure JSON, no archive

Works for tiny projects, falls over when you need per-locale
edit isolation, bundled assets, or multi-document grouping.
Rejected.

### Pure SQLite as the transport format

Great for random access and writes, kills git diffability and
PR review, and ships SQLite into every producer. The current
design adopts the "SQLite where it earns its keep" idea but
keeps it out of transport *and* out of the user's vocabulary.

### XLIFF 2.0 (XML)

Industry standard for CAT tool interop. Adopted as an export
adapter in a future RFC, not as the internal exchange format,
because (a) JSON round-trips through JS/TS toolchains trivially
and XML does not; (b) our Run model is functionally equivalent
to XLIFF 2.0's inline code model (`<ph>`, `<pc>`, paired IDs) in
a JSON-native shape, and XLIFF export is a straightforward
rewrite rather than a semantic translation; (c) LLM workflows
consume JSON much more naturally than XML.

### JLIFF (JSON XLIFF draft)

Early-stage, sparse tooling, and its shape mirrors XLIFF
closely enough that it inherits some of the same awkwardness.
Rejected as the primary format; possible future export adapter.

### Protocol Buffers / FlatBuffers

Schema rigidity makes iterative development harder, codegen is
a build-time tax, and binary format eliminates human review.
Rejected.

### MessagePack / CBOR

Binary-JSON variants save bytes but at the cost of
human-readability. The byte savings don't justify the cost at
realistic project sizes.

### Okapi framework's OmegaT project

The "extract-to-XLIFF, merge-via-skeleton" pattern is directly
borrowed here — it's the right mental model. Different
serialization, same lifecycle.

## Prior art

- **XLIFF 2.0** (OASIS, 2014) — the most mature XML-based
  localization interchange format. Inspires the
  authoritative-exchange-format philosophy.
- **Go's `$GOCACHE`.** A content-addressed build cache keyed by
  the hash of a compilation's inputs. Invisible to users; the
  tool transparently warms and hits it. Directly inspires the
  runtime cache design here.
- **Cargo's `~/.cargo/`, npm's `~/.npm/`, pip's
  `~/.cache/pip/`.** All are content-addressed package/wheel
  caches that users never name. Same mental model.
- **Git's pack files + `.git/index`.** Git keeps pack files +
  object database as the canonical, portable, diffable truth
  (crosses trust boundaries on push/pull), and `.git/index` +
  the working directory as local derived state (never
  transported).
- **Okapi framework** — introduces the extract/skeleton/merge
  pattern with a file-format plugin registry. Inspires the
  Extractor interface.
- **OOXML / `.docx`** — ZIP-of-XML-parts with a root manifest.
  Inspires the `.klz` archive shape.
- **EPUB 3** — ZIP with a strict manifest + content documents.
- **gettext `.po` / `.pot`** — the ancestor. Too lossy for rich
  content, but its `msgctxt` + `msgid_plural` shape informs the
  plural segment model here.
- **Fluent `.ftl`** — Mozilla's rich localization format, with
  first-class plurals and select.
- **SQLite as an application file format** — used by Fossil,
  Apple, and many others. Shows that SQLite is a respectable
  backing store when human diffability isn't required. Here,
  it's scoped to the transparent cache where that trade is
  worth it.

## Unresolved questions

1. ~~**Ownership of `@neokapi/kapi-format`.**~~ Resolved: the TypeScript
   schema lives in this monorepo at `packages/kapi-format/` alongside
   `core/klf` (Go). One release cadence; cross-language changes
   land atomically.
2. **JS-to-Go sync.** Hand-port serializers, codegen from TS
   types, or shell out to a Node subprocess from Go? Lean:
   hand-port initially, shared golden fixtures enforce parity
   in CI.
3. ~~**Vocabulary home.**~~ Resolved: JSON in neokapi
   (`core/model/vocabularies/rich-jsx.json`), loaded the same way
   as existing vocabularies; the TS side mirrors the entries in
   `packages/kapi-format/src/vocabulary.ts` for the reference renderer.
4. **Cache GC policy.** LRU, size-limit, age-limit, or some
   combination? What's the default limit? Lean: max total size
   (default 2 GiB) with LRU eviction, overridable via config.
5. **Streaming writer API shape.** What does the
   `@neokapi/kapi-format` writer look like when a producer wants to
   emit one block at a time without holding everything in
   memory?
6. **Target overlay file shape.** A target overlay is a sparse
   `.klf` with `documents[].blocks[]` containing only the blocks
   whose targets changed — but what about partially-edited
   blocks? Lean: target overlays are block-granular; any
   consumer that needs finer granularity tracks intermediate
   state in its own consumer-private cache tables or in its own
   storage outside neokapi.
7. **Cross-archive references.** Should a `.klz` be able to
   reference parts of another `.klz` (e.g. a shared glossary
   archive)? Deferred to a future RFC.
8. **LLM-friendly flat export.** Derive a `.klf → flat.json`
   shape with just `{hash: renderedText}` for LLM routing and
   quick previews? Lean: provide a helper in `@neokapi/kapi-format`,
   not a separate file format.

## Future possibilities

- **`.klfl` — JSON Lines variant of `.klf`.** For individual
  documents that become too big for atomic JSON parsing (10k+
  blocks in one file, typically auto-generated). Same schema,
  one block per line. `.klz` could bundle `.klfl` files
  interchangeably with `.klf`.
- **Remote skeletons / lazy loading.** Skeletons stored at a URL
  rather than in the archive, downloaded at merge time.
- **Incremental archive diffs.** A `.klz.patch` format that
  records only changed parts, letting CI sync archives across
  branches without transferring everything.
- **XLIFF 2.0 and JLIFF export adapters** for handoff to
  external CAT tools or translation agencies.
- **Peer-to-peer sync.** A `.klz` is already a single-file unit
  suitable for sync via git-annex, IPFS, rsync, or a dedicated
  protocol.
- **Vector-TM extensions.** A future cache schema version might
  add a `segment_embeddings` table for semantic TM search. This
  is exactly the kind of runtime-only change the transparent
  cache is designed to absorb — it requires no public API
  change and no user-visible migration, just a schema bump and
  automatic rebuild.
- **Cross-process cache warmth.** Pre-warming the cache as part
  of a CI build step so interactive access is instant when the
  translator opens the project. Same binary, no new concept,
  just a `kapi cache warm <klz>` admin command.
