# KLF / KLZ walkthrough

Implementation notes for [AD-045](/docs/ad/045-klf-klz-spec). An
end-to-end framework-level lifecycle of the `.klf` / `.klz` formats
— what a developer, a CI pipeline, and a neokapi tool see as a
`.klz` flows through the system. Uses the three example Blocks in
[`packages/kapi-format/examples/`](https://github.com/neokapi/neokapi/tree/main/packages/kapi-format/examples)
so every file and SQL result is real, not pseudocode.

The walkthrough stays at the framework layer throughout: it
shows `@neokapi/react`, the `kapi` CLI, and neokapi Go tools. It
does not show an interactive editor, a server upload, or a
translator typing in a UI — those belong to an external tool
(TMS, CAT tool, custom editor) consuming the format, which is
out of scope for this repo.

## Scenario

A small React + TypeScript app shipped by a team of six. Three
translatable components live in `src/`:

- `src/FilesHeading.tsx` — a section heading with an inline `<span>` and a variable
- `src/TagChip.tsx` — a chip with conditional icons (optional `badge` + `required` marker)
- `src/ShoppingCart.tsx` — a line that says "0 / 1 / N items in your cart"

The team translates to German (`de`) and pseudo-English (`qps`)
using `@neokapi/react` as a neokapi extractor, the `kapi` CLI,
and a small CI pipeline.

## Step 1 — Developer runs extract

```bash
$ cd my-app
$ npx neokapi-react extract --out dist/project.klz

[@neokapi/react] scanning src/**/*.{tsx,jsx}
[@neokapi/react] extracted 3 documents / 3 blocks / 6 placeholders
[@neokapi/react] packing dist/project.klz
  documents/src-FilesHeading-tsx.klf    1.2 KB
  documents/src-TagChip-tsx.klf         1.8 KB
  documents/src-ShoppingCart-tsx.klf    1.9 KB
  skeletons/src-FilesHeading-tsx.skl    312 B   (TransformOp list)
  skeletons/src-TagChip-tsx.skl         487 B
  skeletons/src-ShoppingCart-tsx.skl    411 B
  vocabulary/rich-jsx.json              2.1 KB
  manifest.json                         1.4 KB
  meta.json                             218 B
[@neokapi/react] wrote dist/project.klz (7.8 KB, JSON + skeletons only)
```

What just happened:

1. `neokapi-react` is registered as a neokapi `Extractor`
   implementation (generator id `@neokapi/react`).
2. It walked the source tree via its own SWC-based AST walker,
   producing Blocks for every translatable JSX element.
3. It emitted one `.klf` per source file, each holding the Blocks
   for that file.
4. It captured the per-file TransformOp list as the skeleton (so
   its `merge()` can replay the transform with translated strings
   later).
5. It packed everything into a ZIP. No SQLite anywhere — `.klz`
   is JSON + skeleton bytes + vocabulary JSON + a manifest.

Crucially, the extractor never writes a database and never talks
to a persistent store. It writes a file. That's the full scope of
its framework contract.

## Step 2 — Inspect the archive

A `.klz` is a ZIP. Every developer's existing tooling just works:

```bash
$ unzip -l dist/project.klz
Archive:  dist/project.klz
  Length      Date    Time    Name
---------  ---------- -----   ----
     1412  2026-04-15 10:00   manifest.json
      218  2026-04-15 10:00   meta.json
     1247  2026-04-15 10:00   documents/src-FilesHeading-tsx.klf
     1832  2026-04-15 10:00   documents/src-TagChip-tsx.klf
     1921  2026-04-15 10:00   documents/src-ShoppingCart-tsx.klf
      312  2026-04-15 10:00   skeletons/src-FilesHeading-tsx.skl
      487  2026-04-15 10:00   skeletons/src-TagChip-tsx.skl
      411  2026-04-15 10:00   skeletons/src-ShoppingCart-tsx.skl
     2134  2026-04-15 10:00   vocabulary/rich-jsx.json
---------                     -------
     9974                     9 files

$ unzip -p dist/project.klz manifest.json | jq .
{
  "kapiLocalizationFormat": "1.0",
  "created": "2026-04-15T10:00:00Z",
  "generator": {
    "id": "@neokapi/react",
    "version": "0.8.0"
  },
  "project": {
    "id": "my-app",
    "sourceLocale": "en",
    "targetLocales": ["de", "qps"]
  },
  "parts": [
    { "path": "documents/src-FilesHeading-tsx.klf",
      "sha256": "d4b9f…", "size": 1247, "role": "document",
      "attributes": { "documentId": "src/FilesHeading.tsx" } },
    { "path": "documents/src-TagChip-tsx.klf",
      "sha256": "a18c7…", "size": 1832, "role": "document",
      "attributes": { "documentId": "src/TagChip.tsx" } },
    { "path": "documents/src-ShoppingCart-tsx.klf",
      "sha256": "77ff0…", "size": 1921, "role": "document",
      "attributes": { "documentId": "src/ShoppingCart.tsx" } }
  ]
}
```

The archive is a pure JSON bundle. Runtime queries, TM matching,
and random access by block id are served by neokapi's internal
per-archive cache, which `core/klz` builds on demand and keys by
the manifest's SHA-256. The user never names or manages it.

One of the documents:

```bash
$ unzip -p dist/project.klz documents/src-TagChip-tsx.klf | jq .
{
  "schemaVersion": "1.0",
  "kind": "kapi-localization-format",
  "created": "2026-04-15T10:00:00Z",
  "generator": {
    "id": "@neokapi/react",
    "version": "0.8.0",
    "capabilities": ["extract", "merge", "preview-skeleton"]
  },
  "project": { "id": "my-app", "sourceLocale": "en" },
  "vocabulary": { "extends": ["common-formatting", "rich-html", "rich-jsx"] },
  "documents": [
    {
      "id": "src/TagChip.tsx",
      "documentType": "jsx",
      "path": "src/TagChip.tsx",
      "sourceHash": "sha256:18cd2…",
      "skeleton": { "ref": "skeletons/src-TagChip-tsx.skl" },
      "blocks": [
        {
          "id": "tag-chip",
          "hash": "2GcSuQ",
          "translatable": true,
          "type": "jsx:element",
          "source": [
            { "ph": { "id": "1", "type": "jsx:node", "subType": "logical-and",
                      "data": "index !== undefined && <span className=\"badge\">{index}</span>",
                      "equiv": "badge", "disp": "⟨badge⟩" } },
            { "text": " " },
            { "ph": { "id": "2", "type": "jsx:var", "subType": "string",
                      "data": "{label}", "equiv": "label", "disp": "label" } },
            { "text": " " },
            { "ph": { "id": "3", "type": "jsx:node", "subType": "logical-and",
                      "data": "!deletable && <span className=\"required\">*</span>",
                      "equiv": "required", "disp": "⟨required⟩" } }
          ],
          "placeholders": [
            { "name": "badge", "kind": "node", "jsType": "ReactNode",
              "sourceExpr": "index !== undefined && <span className=\"badge\">{index}</span>",
              "optional": true },
            { "name": "label", "kind": "variable", "jsType": "string",
              "sourceExpr": "label" },
            { "name": "required", "kind": "node", "jsType": "ReactNode",
              "sourceExpr": "!deletable && <span className=\"required\">*</span>",
              "optional": true }
          ],
          "properties": {
            "file": "src/TagChip.tsx",
            "line": 3,
            "component": "TagChip",
            "jsxPath": "TagChip > span[data-tag-chip]",
            "element": "span",
            "locNote": "Tag chip shown in the sidebar list of filters."
          },
          "preview": {
            "storyId": "components-tagchip--default",
            "sampleValues": { "label": "react", "index": 3, "deletable": true }
          }
        }
      ]
    }
  ]
}
```

Key framework-level observations:

- `source` is a flat `Run[]` directly on the Block. No Variant
  wrapper, no array-of-variants, no plural-group block type.
  The 99%+ of blocks that aren't plural groups look exactly
  like this — a sequence of text chunks and placeholder runs.
- The two conditional JSX expressions become `jsx:node`
  placeholders; `{label}` is a `jsx:var`. Text runs between
  them carry the literal whitespace from the source.
- `label` is a required placeholder (no `optional` flag); the
  two conditional JSX expressions carry `optional: true` in
  the block's `placeholders` list, because a language may
  legitimately drop them.
- `preview.storyId` is advisory metadata a consumer can use if
  it wants to drive live renders. The framework itself just
  stores it.

## Step 3 — CI gate via `kapi klz verify`

The team's CI runs a neokapi-framework check on every PR:

```bash
# .github/workflows/i18n.yml (snippet)
- run: npx neokapi-react extract --out dist/project.klz
- run: kapi klz verify dist/project.klz --strict
```

`kapi klz verify` is a stateless one-shot command that lives in
the `kapi` CLI binary. It:

```
1. Opens dist/project.klz (ZIP reader, no SQLite)
2. Parses manifest.json, verifies SHA-256 over every listed part
3. For each documents/*.klf:
     - parses as klf.Document (strict schema check)
     - iterates every block's source fragment
     - runs validateTargetAgainstSource() for each target locale
4. Exits 0 if all checks pass, non-zero otherwise
```

Output on success:

```
$ kapi klz verify dist/project.klz --strict
[klz verify] manifest: 3 documents, 3 skeletons, 1 vocabulary, 0 signatures
[klz verify] integrity: 9/9 parts SHA-256 match
[klz verify] schema: 3/3 documents parse, 3/3 blocks well-formed
[klz verify] placeholders: 6 required, 2 optional, all consistent
OK
```

Output on failure (a hand-edited `.klf` that dropped a required placeholder in a target):

```
$ kapi klz verify dist/project.klz --strict
[klz verify] targets/de/src-TagChip-tsx.klf:
  block "tag-chip": target is missing required placeholder "label"
FAIL
```

CI can gate PRs on this without ever pulling in SQLite, a server,
or any persistent state. `kapi klz verify` is ~6 MB statically
linked Go binary; drops straight into a GitHub Actions runner or
an Alpine container.

## Step 4 — A term-detector produces an annotation sidecar

Before MT runs, the team wires in a protected-terms detector —
a small read-only tool that scans every source block, looks for
brand names and glossary terms, and writes its findings to the
archive as an annotation sidecar.

```bash
$ kapi klz annotate dist/project.klz \
    --producer @neokapi/term-detector \
    --glossary ./glossary.json
[annotate] scanned 3 documents, 14 blocks
[annotate] matches: 2 protected terms in tag-chip, 1 in files-heading
[annotate] wrote annotations/neokapi-term-detector.klfl (3 records)
```

The tool reads the archive via `core/klz`, iterates blocks, and
for each glossary hit writes an annotation record keyed to the
exact run (or character range) it matched. Authoritative content
under `documents/` is untouched — the detector is read-only on
everything the extractor owns.

Inside the archive:

```
dist/project.klz
├── manifest.json
├── documents/...
├── skeletons/...
├── vocabulary/...
└── annotations/
    └── neokapi-term-detector.klfl          ← new sidecar
```

Opening `annotations/neokapi-term-detector.klfl` (JSON Lines,
one record per line, formatted for readability):

```json
{"type":"header","annotationType":"@neokapi/term-detector",
 "annotationVersion":"1.0.0",
 "producer":{"id":"@neokapi/term-detector","version":"0.3.1"},
 "created":"2026-04-15T12:00:00Z",
 "targetArchive":"sha256:9f2c…"}
{"type":"annotation","id":"term-1",
 "anchor":{"kind":"run","block":"tag-chip","path":[2],"runId":"2"},
 "data":{"kind":"protected-term","term":"label",
         "termbaseEntry":"ui-terminology:label",
         "action":"preserve-placeholder","confidence":1.0}}
{"type":"annotation","id":"term-2",
 "anchor":{"kind":"range","block":"files-heading","path":[0],
           "offset":0,"length":5},
 "data":{"kind":"glossary-match","term":"Files",
         "termbaseEntry":"ui-terminology:files",
         "targetLocaleSuggestions":{"de":"Dateien","ja":"ファイル"}}}
```

Key properties of this step:

- **Namespaced.** The file is named for the producer, so any
  number of independent tools can write annotations side by side
  without stepping on each other.
- **Non-authoritative.** A later consumer that doesn't recognise
  `@neokapi/term-detector` skips the file entirely and processes
  `documents/` correctly.
- **Rebuildable.** If the file is deleted or gets out of sync, a
  re-run reproduces it; no ground truth is lost.
- **Orphan-aware.** On the next CI run, `kapi klz verify` will
  resolve every anchor against the current block graph. If a
  block changed enough that an anchor no longer lands on its
  recorded run/range, the annotation is flagged as a stale
  orphan — the producer re-runs to refresh.

The MT pipeline in Step 5 reads these annotations and uses them
to tell the engine not to translate the matched runs — the
protected terms flow through unchanged. The review tool in
Step 8 reads them too, to highlight glossary hits in the diff.

## Step 5 — A pipeline tool runs MT against the archive

The framework's existing flow executor wires `.klz` into an MT
pipeline using nothing but `core/klz` and existing neokapi tools.
There is **no separate "build the runtime database" step** — the
pipeline tool calls query helpers on the reader, and `core/klz`
warms a local cache transparently on first call.

```go
// tools/mt-pipeline/main.go — a tiny Go program using neokapi
package main

import (
    "context"
    "os"

    "github.com/neokapi/neokapi/core/klz"
    "github.com/neokapi/neokapi/core/flow"
    "github.com/neokapi/neokapi/providers/mt/deepl"
    "github.com/neokapi/neokapi/core/tools/validators"
)

func main() {
    ctx := context.Background()
    reader := must(klz.NewReader(os.Stdin))
    defer reader.Close()

    writer := must(klz.NewWriter(os.Stdout))
    defer writer.Close()

    // reader.TM() returns a query handle backed by the transparent
    // runtime cache. If this is the first time any tool has queried
    // this .klz on this machine, the cache warms now (~100 ms for
    // a typical project). Every subsequent query hits the cache.
    // The cache location, build, and lifecycle are all invisible.
    tm := reader.TM()

    pipeline := flow.NewBuilder().
        Source(reader).
        Tool(validators.PlaceholderCheck{Mode: "strict"}).
        Tool(deepl.Translate{Locales: []string{"de"}, TM: tm}).
        Tool(validators.PlaceholderCheck{Mode: "strict"}).
        Sink(writer).
        Build()

    if err := pipeline.Run(ctx); err != nil {
        panic(err)
    }
}
```

Run it:

```bash
$ go run ./tools/mt-pipeline < dist/project.klz > dist/project-de.klz
```

The pipeline streams:

1. `klz.Reader` inflates parts lazily, emitting Blocks onto a
   channel. Pure Block iteration — the cache is untouched.
2. `PlaceholderCheck` verifies every incoming Block's source
   fragment has the expected placeholders. Passes through.
3. `deepl.Translate` calls the DeepL API for each Block,
   consulting the `TM` handle first to avoid redundant calls.
   The TM lookup is the first query that needs random access —
   if the cache is cold, neokapi warms it here. If another
   process has already warmed the cache for this same `.klz`
   content, this process hits the existing entry for free.
4. `PlaceholderCheck` runs again on the MT output to catch any
   lossy translations that dropped required placeholders.
5. `klz.Writer` pulls Blocks from the final channel and writes
   them into new `targets/de/*.klf` overlays in the output
   archive.

Every stage operates on `model.Block`. Neither the MT tool nor
the validators know or care that the input was a `.klz` —
they're generic neokapi tools reading from
`format.DataFormatReader`. The cache is an implementation detail
of `core/klz`'s query helpers; no tool has to know it exists.

## Step 6 — TM lookup during subsequent runs

A week later, a new component is added that uses the same
"N items in your cart" pattern. When the pipeline runs again:

1. `neokapi-react extract` produces a new `project.klz` with the
   additional block. Its `manifest.json` contents — and therefore
   its manifest SHA-256 — are different from the previous
   archive's.
2. The MT pipeline tool opens the new `.klz` and calls
   `reader.TM().SimilarSources(...)`.
3. `core/klz` computes the manifest hash, looks up the cache
   directory for this hash, finds no entry, and builds a fresh
   one from the source `.klz` (~100 ms). The previous cache
   entry for the old `.klz` is still on disk but unreachable
   from this query — it'll age out under normal cache GC.
4. The new component's TagChip block is now in `segments`; the
   existing German translation for the similar string is still
   in `targets` (because the extractor copied the untouched
   target overlay from the previous archive).
5. The `SimilarSources` query runs against the freshly-built
   cache and returns matches.

**No rebuild command was run. No user was asked to refresh
anything. No tool had to check whether the cache was stale.**
The content address makes drift self-correcting.

Under the hood, `core/klz` routes TM lookups through a typed Go
helper:

```go
matches, err := reader.SimilarSources(ctx, klz.SimilarQuery{
    Text:   "{count} items in your cart",
    Locale: "de",
    Limit:  5,
})
```

For debuggers who want to understand what that query did, the
equivalent SQL (as it runs against the internal cache file) is:

```sql
SELECT s.source_text, t.target_runs,
       bm25(sources_fts) AS score
FROM sources_fts
JOIN sources s ON s.id = sources_fts.rowid
JOIN targets t ON t.source_id = s.id
WHERE sources_fts MATCH 'items cart'
  AND t.locale = 'de'
ORDER BY score
LIMIT 5;
```

The matches flow through the pipeline the same way any tool
output does. Whether, when, and how a human reviews them is a
concern for whatever external tool consumes this pipeline — the
framework just provides the lookup primitive.

## Step 7 — Merge back into source

The developer wants to ship a German build. The framework routes
the merge through the extractor that produced the archive:

```bash
$ kapi klz merge dist/project-de.klz --locale de --out dist/src-de/

[klz merge] reading dist/project-de.klz
[klz merge] generator: @neokapi/react v0.8.0
[klz merge] loading extractor (registered via format.RegisterExtractor)
[klz merge] calling @neokapi/react.Merge() for 3 documents
[neokapi/react] loading skeletons/src-TagChip-tsx.skl (TransformOp list)
[neokapi/react] applying targets/de/src-TagChip-tsx.klf to skeleton
[neokapi/react] wrote dist/src-de/src/TagChip.tsx
… (same for FilesHeading.tsx and ShoppingCart.tsx)
OK
```

The `kapi klz merge` command:

1. Opens the `.klz` (no runtime cache needed — merge only reads
   authoritative parts from the archive).
2. Reads `manifest.generator.id` → `@neokapi/react`.
3. Looks up the extractor in `format.RegisterExtractor`'s
   registry.
4. Calls the extractor's `Merge()` method with the skeleton bytes
   and target overlays.
5. The extractor reconstructs translated source files; `kapi
   klz merge` writes them under `--out`.

Neokapi itself doesn't know anything about JSX transforms,
TransformOps, or how a skeleton turns into a `.tsx` file. That's
owned entirely by the extractor named in the manifest.

## Step 8 — PR review

The developer commits `dist/project.klz` and
`dist/project-de.klz` alongside the source. Because the `.klz`
archives are plain JSON inside ZIP, a PR that updates a
translatable string produces a readable diff:

```diff
 documents/src-TagChip-tsx.klf
   blocks: [
     {
       id: "tag-chip",
-      hash: "2GcSuQ",
+      hash: "4HrKbL",
       ...
       source: [
         …
         { ph: { id: "2", type: "jsx:var", subType: "string",
-                data: "{label}", equiv: "label" } },
+                data: "{title}", equiv: "title" } },
         …
       ]
```

The reviewer sees immediately that `label` was renamed to
`title` in source. The German target's content hash no longer
matches; `kapi klz verify` flags it as stale on the next CI run.

There are no binary files to commit. The runtime cache lives in
each contributor's `$XDG_CACHE_HOME/neokapi/klz/`, invisible to
the project and independent per machine — the first query
against a fresh or changed `.klz` warms it in ~100 ms. No
cross-contributor sync, no stale state on someone else's
machine, no CI surprises.

## Cache administration (aside)

The cache is designed to be invisible in normal use — users and
tools simply call `core/klz` query helpers and things are fast.
But when something goes wrong, or a user wants to understand
cache size, `kapi` exposes a small administrative subcommand
group:

```bash
# Total cache footprint across all cached archives.
$ kapi cache info
[cache] location: ~/.cache/neokapi/klz/
[cache] entries: 47
[cache] total size: 18.2 MB
[cache] oldest entry: 2026-03-12T09:14:07Z
[cache] newest entry: 2026-04-15T10:02:00Z

# Show the cache directory for a specific .klz (debug aid).
$ kapi cache path dist/project.klz
~/.cache/neokapi/klz/ab/cd1234e5f6.../

# LRU eviction with a size cap.
$ kapi cache gc --max-size=1GB
[cache gc] removed 3 entries (14.1 MB freed)

# Wipe everything.
$ kapi cache clear
[cache clear] removed 47 entries (18.2 MB freed)
```

These are administrative commands. Normal use never runs them.
A debugger who wants to open a cache entry directly can use
`kapi cache path` to find the directory, then `sqlite3` the
`db.sqlite` file inside it — the internal schema is documented
in RFC 0001.

## Recap — who does what

| Lifecycle stage | Actor (framework-level) | Touches |
|---|---|---|
| Extract source | `@neokapi/react` extractor | reads `src/**/*.tsx`; writes `.klz` documents + skeletons + vocabulary + manifest |
| Inspect | `unzip`, `jq`, anyone | reads `.klz` parts individually |
| Verify in CI | `kapi klz verify` | reads `.klz`; SQLite-free; exits non-zero on failure |
| Annotate | `kapi klz annotate` or any namespaced producer | reads `.klz` documents; writes `annotations/<producer>.klfl` sidecar (non-authoritative) |
| Streaming pipeline | a neokapi tool using `core/flow` | reads `.klz` as `format.DataFormatReader`; writes `.klz` via writer |
| TM lookup / block-by-id | a tool calling `reader.TM()` or `reader.BlockByID()` | reads `.klz`; transparently warms a local cache on first query |
| Merge back | `kapi klz merge` → extractor's `Merge()` | reads `.klz`; writes translated source tree |
| PR review | human | reads `.klf` JSON with `git diff` |
| Cache admin (debug) | `kapi cache info` / `kapi cache path` / `kapi cache clear` / `kapi cache gc` | reads/manages `$XDG_CACHE_HOME/neokapi/klz/` |

Every row is a library call, a CLI command, or a registered
tool. Neither `.klz` nor the runtime cache holds
interactive-editor state, multi-user sync metadata, persistent
workspace state, or authentication tokens — those belong to an
external tool consuming the format (TMS, CAT tool, custom
editor) and live in its own storage layer.

An external tool that wants to expose interactive editing reads
the `.klz` via `core/klz`, calls `reader.TM()` / `reader.BlockByID()`
/ `reader.SimilarSources()` when it wants query acceleration
(the cache behind those methods stays invisible), and emits a
new `.klz` whenever it wants a portable snapshot to hand off.
That work is out of scope for this repo.
