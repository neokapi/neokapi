# Create content with kapi as the checker

When you are **authoring new content** — there is no frozen source file to edit,
you are writing the document — kapi is still the checker. You write; kapi parses
what you wrote, holds it to the brand voice and terminology, and tells you what
to fix. No model provider is involved; the loop is just you and kapi.

This is the **author → parse → check** loop. It is the create-side counterpart of
the read → edit → write loop in [edit.md](edit.md): there the source is fixed and
you edit blocks; here you produce the content and the first check is parsing it
back.

## 1. Author in a generative format

Write the document in a format kapi can produce from content alone — Markdown,
HTML, JSON, YAML, and the other **generative** formats. Confirm a target format
is generative before authoring into it:

```bash
kapi formats list --json | jq -r '.formats[] | select(.generative) | .name'
```

**Binary office formats (`.docx`, `.pptx`, `.xlsx`) cannot be authored from
scratch** — they are editable but not generative. To produce one, author the
content in a generative format and, if a binary deliverable is required, start
from an existing binary file and edit it in place ([edit.md](edit.md)).

## 2. Parse it as the first check

Reading the file back is the first verification that what you wrote is
well-formed and says what you intend. `kapi stats` summarizes it; `kapi inspect`
shows it block by block, the same structured view a reader or RAG pipeline sees:

```bash
kapi stats draft.md --json
kapi inspect draft.md --jsonl
```

If a block is missing, merged, or carries text you didn't intend, fix the source
and parse again.

## 3. Gate on brand and terminology

Run the content rules. For a one-off file, `kapi check`; in a project,
`kapi verify` runs every bound gate (brand voice, terminology, QA) together:

```bash
kapi check draft.md --profile-file brand.yaml --json   # one-off
kapi verify --json                                       # in a project
```

The check exits 0 when the gate passes and 3 when it fails, with one finding per
block — its location, the rule, and a suggested fix. Load the voice guide and the
approved wording **before** writing so the first draft is already close:

```bash
kapi brand guide                       # the voice to follow (no flag inside a project)
kapi termbase lookup "dashboard" -t en  # the approved term
```

## 4. Revise and repeat

Fix what the check flagged, then re-run it. Two ways to revise, both
provider-free:

- **Edit the source directly** and re-check — natural while you are still
  drafting.
- **Route the fix through `kapi apply`** as a `content` entry — when you want the
  edit guarded by the faithful round-trip and the drift/inline-code checks, or
  when the fix travels alongside an asset change (below). See [edit.md](edit.md)
  for the content-entry shape and the guards.

Iterate until the gate is green. A clean check, not a written file, is the finish
line.

## Close the loop: fix the content and the rule together

When a check flags a term, the durable fix is usually two changes: correct **this
draft**, and record the rule so **future** drafts are checked against it. Both
are typed entries in **one** `kapi apply` change-set, and they land atomically —
content and asset through the single write verb:

```jsonl
{"kind":"content","file":"draft.md","id":"p4","content_hash":"a1b2…","text":"Open the dashboard."}
{"kind":"term","op":"upsert","term":"dashboard","locale":"en","status":"preferred","replaces":"control panel"}
```

```bash
kapi apply changeset.jsonl
```

- The **content** entry rewrites the block through the faithful round-trip.
- The **term** entry upserts the glossary term: it is written into the project's
  committed termbase source (`.klftb`) and the existing import compiles it into
  the `.kapi/termbase.db` cache. `git diff` shows the one new term; the next
  `kapi verify` enforces it.

The asset kinds `kapi apply` accepts — `term`, `tm`, `brand`, `recipe` — and
their fields are summarized in [edit.md](edit.md); the brand-vocabulary case is
detailed in [brand.md](brand.md). Asset entries require a `.kapi` project (the
committed source and recipe live there).

After applying, run `kapi verify` (or `kapi check`) once more to confirm the
draft is clean and the new rule passes.
