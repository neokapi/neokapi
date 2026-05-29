---
sidebar_position: 3
title: Translation Editor
---

# Translation Editor

The translation editor is the core editing surface for a file. It is one of
three per-file surfaces, each with a single concern:

- **Translate** — edit blocks with AI/TM assistance and terminology insertion.
- **[Review](/server/review)** — work through blocks by status, run QA, and
  approve or reject translations.
- **[Pre-process](/server/pre-process)** — file-wide source-prep (pseudo-translate,
  bulk TM leverage) before editing begins.

A switcher at the top of each surface moves between the three. The same switcher
appears in the web app and the desktop app.

## Real-time collaboration

A project is a shared, live workspace. When teammates open the same project you
see their names and avatars on the blocks they are editing, their edits and
reviews appear in your editor without a refresh, and the progress bar moves as
the team works. Concurrent edits merge on the server, so two people in the same
file converge on one consistent state instead of overwriting each other. The web
and desktop apps are equal real-time clients of the same server, and the desktop
app keeps working offline — edits queue locally and replay on reconnect. See
[Real-time collaboration](/server/collaboration) for the full picture.

## Two views

The Translate editor has two views, toggled from the header:

### Visual

The default view places an inline editing card over a formatted document
preview, so you edit each block in the context it appears in. The card shows the
source with its natural formatting (bold is bold, links are underlined,
placeholders render as chips), the target editor, optional reference locales,
and per-block context — translation-memory matches and terminology. Navigate
block to block with the card's arrows or `j`/`k`; the preview scrolls to keep
the active block in view. The preview can render source, target, or pseudo
content.

The Visual view is best for careful, in-context editing — especially blocks with
inline tags, where seeing the surrounding layout matters.

### Table

The Table view lists every block in a two-column grid (source and target) with a
status accent on the left edge. Click or double-click a target cell to edit it
inline with the same editor the Visual view uses; press **Enter** to save and
advance, **Escape** to cancel. A search box filters blocks by source or target
text.

The Table view is best for scanning a file and editing many blocks quickly.

Both views share the same data, the same inline editor, and the same chip
rendering — switching views never changes what you can edit, only how the file
is laid out.

## Inline tags

Many document formats contain inline markup (bold, links, placeholders, etc.)
that the editor handles automatically. In the default **formatted view**, text
appears with its natural formatting applied. You can switch to the **code view**
(click the `</>` button on the source) to see abstract tag chips.

When editing translations with inline tags:

- **Flexible tags** (bold, italic, links) can be freely removed, duplicated, or
  rearranged.
- **Required tags** (variables, placeholders, line breaks) must be kept in the
  translation — the editor prevents accidental deletion and shows them with
  dashed borders.
- The **tag palette** above the editor shows source tags as clickable buttons.
- The **validation bar** warns in real time about missing required tags or
  duplicated non-cloneable tags.
- Use **Ctrl+1** through **Ctrl+9** to insert tags from the palette.
- Click the **tag summary badge** in the card header to expand the inline code
  legend, which lists every tag type with its constraints.

The editor provides the same experience regardless of file format — HTML,
Markdown, XLIFF, and all other formats present tags identically because they
share the same vocabulary system. See
[Inline Formatting](https://neokapi.github.io/web/neokapi/docs/features/inline-formatting)
for more details.

## Per-block context

The Visual card surfaces the linguistic context for the selected block:

### TM matches

Translation-memory matches for the block, each with a score (green for a 100%
exact match, yellow for fuzzy), the match type, the stored source and target,
and an **Apply** button that copies the match into the target.

### Terminology

Terms found in the block appear in the term sidebar with their source term,
suggested target term(s), lifecycle status (preferred, approved, admitted,
deprecated), and domain. Clicking a target term inserts it into the active
target.

## Entities

Select text in the source and press **⌘E** (Cmd+E / Ctrl+E) to mark it as an
entity — for example a product name to leave untranslated. Marked entities are
highlighted in the source and listed in the block context.

## Progress tracking

The progress bar shows translation progress with colour-coded segments — gray
(not started), yellow (draft), blue (translated), green (reviewed) — plus a
percentage, an `X/Y translated` counter, and a per-status breakdown. It updates
in real time as you translate.

## Status bar

The bottom of the editor shows the current block position (Block N of M), source
word and character counts, and target word counts.

## Keyboard shortcuts

| Key                           | Action                            |
| ----------------------------- | --------------------------------- |
| **Enter**                     | Start editing / save and advance  |
| **Escape**                    | Cancel editing                    |
| **Arrow Up/Down** or **j/k**  | Navigate between blocks           |
| **⌘E**                        | Mark selected source text as an entity |
| **Ctrl+1** through **Ctrl+9** | Insert tag from the palette       |

## File export

Click **Export** in the header to download the translated file in its original
format (HTML, XML, JSON, etc.) with all translations applied. In the browser
this triggers a file download; in the desktop app the file is saved to disk and
opened in your system file manager.
