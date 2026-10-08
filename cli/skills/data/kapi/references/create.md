# Create content with kapi as the checker

When you are **authoring new content** (there is no frozen source file to edit;
you are writing the document), kapi is still the checker. You write; kapi parses
what you wrote, holds it to the voice profile and terminology, and tells you what
to fix. No model provider is involved; the loop is just you and kapi.

This is the **author → parse → check** loop. It is the create-side counterpart of
the read → edit → write loop in [edit.md](edit.md): there the source is fixed and
you edit blocks; here you produce the content and the first check is parsing it
back.

## 1. Author in a generative format

Write the document in a format kapi can produce from content alone: Markdown,
HTML, JSON, YAML, and the other **generative** formats. Confirm a target format
is generative before authoring into it:

```bash
kapi formats --json | jq -r '.formats[] | select(.generative) | .name'
```

**Binary office formats (`.docx`, `.pptx`, `.xlsx`) cannot be authored from
scratch**: they are editable but not generative. To produce one, author the
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

## 3. Gate on voice and terminology

Run the content rules against the authored file. A project resolves voice and
terms from its path; a one-off check can use an explicit profile:

```bash
kapi check draft.md --profile-file voice.yaml --json   # one-off
kapi check draft.md --json                            # project-scoped guidance
```

The check exits 0 when the gate passes, 3 when it fails, and 4 when it did not
run: no content was checked, or a check reported nothing on the known-bad sample
it runs beside the content. Treat 4 as unverified, never as a pass; `--no-fail`
does not change it. Read `did_not_run_cause`: `checker_invalid` means a checker
is broken and the run cannot be trusted. Findings identify the
location and rule, with a suggested fix where available. Inspect analyzer
coverage and review unsupported guidance separately. Use `kapi check --ship`
when the task includes project release gates. Load the voice guide and the
approved wording **before** writing so the first draft is already close. Inside a
project, the assistant file (`CLAUDE.md`, or an `AGENTS.md` already at the
root) carries a section
saying exactly this; when a project you are standing up binds a voice and the
file has no such section, `kapi voice pointer` writes it
([project.md](project.md)).

```bash
kapi context draft.md                  # the voice and terms that apply to the file
kapi terms lookup "dashboard" -t en  # the approved term
```

## 4. Revise and repeat

Fix what the check flagged, then re-run it. Two ways to revise, both
provider-free:

- **Edit the source directly** and re-check, which is natural while you are still
  drafting.
- **Route the fix through `kapi apply`** as a `replace_text` or `set_content`
  operation, when you want the edit guarded by the faithful round-trip, the
  revision you read and the inline-code checks. See [edit.md](edit.md) for the
  change set's shape and what refuses an edit.

Iterate until the gate is green. A clean check, not a written file, is the finish
line.

## Close the loop: fix the content and the rule together

When a check flags a term, the durable fix is usually two changes: correct **this
draft**, and record the rule so **future** drafts are checked against it.
Correct the draft through `kapi apply`, with the block's `ref` and `rev` from
`kapi inspect draft.md --jsonl`, then record the change you made:

```json
{"ops": [{"op": "replace_text", "at": {"doc": "draft.md", "block": "setup/p#2"}, "if_match": "r:a1b2c3d4e5f60718",
          "edits": [{"find": "control panel", "text": "dashboard"}]}]}
```

```bash
kapi apply change.json
kapi context note --from "control panel" --to "dashboard" --seen-in draft.md --suggest
```

- The **content** operation rewrites the block through the faithful round-trip.
- The **note** records what you changed and, with `--suggest`, the rule it
  implies. `kapi check` reports the rule as a suggestion and fails nothing on
  it until a person keeps it in `kapi context review`, which writes it into the
  project's terms store.

Writing a term into the store directly is a person's decision. A `term`
operation in a `kapi apply` change set does that, so from your shell it is
refused, and the change set with it; when the person has decided, they run
`kapi apply` on it themselves. The asset operations `kapi apply` accepts
(`term`, `memory`, `recipe`) are summarized in [edit.md](edit.md), and a word
rule is detailed in [voice.md](voice.md).

After applying, run `kapi check draft.md --json` again to check the draft.
