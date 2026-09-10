---
title: Semantic section edits
description: Inspect heading-delimited content, prepare snapshot-bound patches, and verify a complete section replacement across document formats.
---

# Semantic section edits

A section edit replaces the body beneath an existing heading. The body can
contain several paragraphs, nested headings, lists and code blocks. The selected
heading stays in place. The range ends at the next heading of the same or a
higher level, or at the end of the containing structure. In HTML, heading
sections are bounded by their parent element.

The format reader supplies the document structure and heading identity. Section
inspection presents the body as Markdown so an editor can work on a coherent
piece of content. The format adapter prepares replacements against immutable
source offsets. The writer applies those patches to the original bytes; it does
not rebuild the document from a mutated content tree.

This mechanism complements block edits. See
[E-02: The format system](/contribute/architecture/engine/e-02-format-system)
for the reader and writer contracts, and
[Content fidelity](/contribute/implementation/engine/content-fidelity)
for the distinction between preserved content and rendered appearance.

## Inspect and replace a section

Inspect the file and select the returned section by its title and path. Use the
returned ID and full-file SHA-256 snapshot in the change entry:

```sh
kapi inspect instructions.md --sections
```

The response contains `file`, `format`, `snapshot`, `content_format: "markdown"`
and `sections`. Each section includes its heading identity, title, level, path
and Markdown body. Its `range` identifies the heading and body through native
block IDs, structural addresses, content hashes and advisory source spans.
A section's body includes its descendant sections. Replacing
that body also replaces those descendants.

Write one section entry to a JSONL change file:

```json
{"kind":"section","file":"instructions.md","id":"<returned heading ID>","snapshot":"<returned SHA-256>","text":"Explain the action.\n\n### Next step\n\nGive the reader the next instruction.\n"}
```

Preview the edit as a diff or inspect the actual offset plan:

```sh
kapi apply changes.jsonl --diff
kapi apply changes.jsonl --diff --json
```

The JSON preview includes a `plan` with `format`, `snapshot` and `patches`.
Each patch contains `start`, `end`, `before` and `replacement`. `start` and
`end` are byte offsets into the immutable original source, with an exclusive
end. `before` and `replacement` are UTF-8 strings. For a Word document, `entry`
identifies a package part already recognized by the native reader through
`partPath`. The offsets address that member's uncompressed bytes. The plan also
carries `range`, with the preserved heading and affected native block anchors.
Reader-provided source spans bind those anchors to the writer offsets. Agents
select the heading ID and snapshot; the adapter supplies package locations.
A flat-file patch addresses the file bytes.

Apply the change and inspect the saved document before another revision:

```sh
kapi apply changes.jsonl
kapi inspect instructions.md --sections
kapi check instructions.md --json
```

The preview writes nothing. Applying an entry against a changed snapshot fails;
the editor must inspect again and prepare a new entry. A subsequent check reads
the actual saved file. Its analyzer coverage determines what a passing result
establishes.

## Replacement syntax and preservation

The POC accepts one section entry per change-set, on a regular local file.
Symlinks, ambiguous boundaries, missing source spans and unsupported markup
are rejected. Heading-free documents have no section target.

The shared replacement body uses a bounded Markdown vocabulary: paragraphs,
nested headings, lists, fenced code and basic emphasis. A format adapter maps
that body into its native representation and rejects unsupported structures.
Rich document features outside the supported range require a narrower edit or
an explicit adapter capability.

Markdown and HTML plans preserve the bytes outside the selected body, including
the selected heading. HTML shell attributes and unrelated elements remain part
of the original source. Word plans patch the relevant document member and retain
the other ZIP member payloads. A Word list requires compatible numbering
definitions in the source package. The demonstration supplies those definitions. Word replacement supports bullet
lists with an existing compatible numbering definition; ordered lists and
hyperlinks require broader relationship and numbering support. Tables, images,
raw HTML and block quotes are outside the shared fragment vocabulary.

The Markdown reading projection is an editing view. It does not establish that
every native feature can be reconstructed from that projection. Unchanged ZIP
payloads and unchanged source ranges are separate assertions from the appearance
of the rewritten section in Word or a browser.

## Reproduce the cross-format loop

From the repository root, provide a binary built with section editing and a new
output directory:

```sh
python3 scripts/semantic-edit/run.py --kapi ./bin/kapi --output /tmp/kapi-section-demo
python3 -m unittest discover -s scripts/semantic-edit -p 'test_*.py'
```

Open `/tmp/kapi-section-demo/report.html` in a browser. The standalone report
shows the task, factual context and the complete original section beside both
revisions. Each format includes the actual check findings and execution
coverage. Download links open the original and revised native files.

The script creates the same Pageglass sharing instructions in Markdown, HTML
and a deterministic Word package using Python's standard library. Pageglass is
a fictional tool. The text is authored specifically for this demonstration;
no model generates or reviews it. Its commands are examples inside the content
and are never executed.

The initial section combines loose instructions with forbidden vocabulary.
The first complete replacement organizes sharing, expiry and revocation around
the reader's actions, while deliberately retaining one occurrence of
`utilize`. The bound voice profile rejects that word and suggests `use`. The
script verifies the actual finding before applying the second complete
replacement. Final checks must pass with the rule analyzer recorded as run.
The factual guidance remains explicitly unsupported by the deterministic check.

The loop also verifies these independent properties:

- Both preview modes leave the source bytes unchanged.
- Replaying the returned offset plan independently produces the saved content.
- Reusing a stale snapshot fails without changing the file.
- The surrounding sections retain their source bytes.
- Word styles, numbering, an image payload and a custom ZIP payload remain
  unchanged.

The output directory retains the isolated project, each command's arguments,
exit status and output hashes, inspection results, change entries, offset plans
and full check reports. The runner opts out of project discovery, supplies an
explicit fixture recipe, and isolates configuration, data, caches and plugin
discovery. Existing output directories are rejected to preserve earlier evidence.

This demonstration establishes the deterministic edit/check loop and the stated
preservation properties. It does not measure editorial quality, model skill,
review effort or rendered Word layout.
