# Edit content in any format

Edit the text inside a file an editor can't open directly — a Word document, a
PowerPoint deck, a JSON catalog, an XLIFF file, Markdown — and write it back in
the same format, byte-for-byte except for the text you changed. You do the
editing; kapi parses the format, enforces a faithful round-trip, and is the
checker. No model provider is involved.

This is the **read → edit → write → verify** loop. It is the deliberate,
reviewed counterpart to a `ksed` find-and-replace (see [toolbox.md](toolbox.md)):
reach for this loop when you are rewriting block text by hand (an on-brand fix, a
clarity pass, a terminology correction), and for `ksed` when a regex
substitution expresses the change.

## 1. Read the blocks

`kapi inspect` parses any format into one record per content block — the text,
the block's structural role, a stable `id`, and a `content_hash`:

```bash
kapi inspect report.docx --jsonl
```

```json
{"file":"report.docx","number":1,"id":"p1","content_hash":"a1b2c3…","role":"heading","level":1,"text":"Quarterly summary"}
{"file":"report.docx","number":2,"id":"p2","content_hash":"d4e5f6…","role":"body","text":"Revenue rose, see the <x id=\"1/\"/> dashboard."}
```

Two fields anchor an edit:

- **`text`** renders inline codes (links, bold spans, placeholders) as
  `<x id="…"/>` tokens. **Keep every token, unchanged, in your edited text** —
  they are the markup the round-trip reconstructs. A placeholder is
  `<x id="1/"/>`; a paired span opens with `<x id="1"/>` and closes with
  `<x id="/1"/>`. Reorder or drop one and the edit is rejected (see §3).
- **`content_hash`** is the block's canonical identity (a hash of its plain
  source text, not of the placeholder `text`). Send it back with the edit so
  kapi can tell the block is still the one you read.

## 2. Write the edits

Produce one `content` entry per block you changed — JSONL, one per line — naming
the block's `file`, `id`, the `content_hash` you saw, and your new `text`:

```json
{"kind":"content","file":"report.docx","id":"p2","content_hash":"d4e5f6…","text":"Revenue climbed; see the <x id=\"1/\"/> dashboard."}
```

Then apply it. `kapi apply` is the one write verb — it reads the change-set from
a file, an argument, or stdin, and writes each named file in place:

```bash
kapi inspect report.docx --jsonl | edit-the-text > edits.jsonl
kapi apply edits.jsonl --diff          # preview the content changes, write nothing
kapi apply edits.jsonl                  # apply in place
kapi apply edits.jsonl --in-place=.bak  # apply, keeping a .bak of each file
```

`kapi apply` is the sole write verb — it covers every case (one file or many,
content alone or content mixed with asset edits; see
[Mixed change-sets](#mixed-change-sets)). kapi never sends content to a model to
rewrite it; you write the new text and `kapi apply` round-trips it back.

## 3. The two guards

`apply` writes a block only when both guards pass; a blocked edit leaves that
block untouched and is reported, so nothing is silently corrupted:

- **Drift guard.** If a block's current `content_hash` no longer matches the one
  in your entry, the source changed since you inspected it. The edit is marked
  **stale** and skipped.
- **Inline-code guard.** If your edited `text` drops, invents, duplicates, or
  unbalances an `<x id="…"/>` token, the edit is **rejected** rather than written
  back with broken markup.

Either outcome exits on the **gate code (3)**, distinct from an operational
error. Treat it as a signal to **re-inspect the affected blocks and retry** with
fresh hashes — the same loop a failing check drives. `apply` is idempotent: an
entry whose text already matches the block is a no-op, so re-running a partly
applied change-set is safe.

## 4. Verify

A written file is not the finish line — a clean check is. In a project, run
`kapi verify`; for a one-off file, `kapi check`:

```bash
kapi check report.docx --json     # one-off: deterministic content rules
kapi verify --json                 # in a project: brand + terminology + QA gates
```

Read the findings, fix the flagged blocks through another `apply` pass, and
re-run until the gate is green (exit 0).

## Which formats can I edit?

`kapi formats list` reports an **Edit** column and the JSON adds `editable` and
`round_trip`:

```bash
kapi formats list --json | jq -r '.formats[] | select(.editable) | .name'
```

A format is **editable** when it has both a reader and a writer and is not a
bilingual interchange format — this **includes binary office formats** (`.docx`,
`.pptx`, `.xlsx`): the faithful round-trip is exactly what makes editing a binary
container safe. `round_trip` (shown as `faithful` in the table) means the writer
reconstructs from a skeleton, so only your edited text changes and the rest is
byte-for-byte preserved. A read-only format (PDF is extraction-only) is not
editable — to localize it, extract to a bilingual format and merge (see
[localize.md](localize.md)).

Binary formats can be **edited in place but not authored from scratch**; to
create new content, author in a generative format — see [create.md](create.md).

## Mixed change-sets

A `content` edit and the asset change that justifies it (a glossary `term`, a
`brand` rule) can land **atomically in one `kapi apply`** — every reviewed
change, content or asset, is one typed entry routed through the single write
verb. See [create.md → close the loop](create.md) for the asset entry shapes;
for the brand-vocabulary case specifically, [brand.md](brand.md).
