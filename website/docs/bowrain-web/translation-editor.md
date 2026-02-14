---
sidebar_position: 3
title: Translation Editor
---

# Translation Editor

The translation editor is the core workspace for translating documents. It displays source and target content side by side, with tools for AI translation, TM lookup, terminology enforcement, and manual editing.

## Layout Modes

The editor supports four layout modes, accessible from the toolbar:

### Grid Mode

The default view displays all blocks in a table with source and target columns. Each row shows:

- **Status indicator** — color-coded left border (gray = not started, blue = draft, green = translated, purple = reviewed)
- **Source text** — read-only, with inline tags rendered as colored chips
- **Target text** — editable inline; click or press Enter to edit

Grid mode is best for scanning through a file and quickly editing multiple blocks.

### Focus Mode

Focus mode shows a single block at a time with full-width source and target panels. The source panel displays the text with tag visualization, and the target panel provides a large text area for editing.

Use the **Previous** and **Next** buttons (or keyboard shortcuts) to navigate between blocks. Focus mode is ideal for detailed editing of individual blocks, especially those with complex inline tags.

### Split Horizontal

The editor appears on top with a preview panel below. This layout is useful when a `renderPreview` handler is available (currently supported in the Bowrain desktop app).

### Split Vertical

The editor appears on the right with a preview panel on the left. Same preview support as split horizontal.

## Toolbar

The toolbar at the top of the editor provides these tools:

### Translation Tools

| Button | Action |
|--------|--------|
| **Pseudo** | Generate pseudo-translations for the entire file (for layout testing) |
| **AI Translate** | Translate the file using the configured AI provider |
| **TM Lookup** | Match source blocks against translation memory and apply matches |
| **Provider selector** | Choose between configured AI/MT providers |

### Navigation

| Button | Action |
|--------|--------|
| **Untranslated** arrows | Jump to the previous or next untranslated block |
| **Copy Source** | Copy the source text to the target for the selected block |
| **Reviewed** | Mark the selected block as reviewed |

### View Controls

| Button | Action |
|--------|--------|
| **Layout switcher** | Toggle between grid, focus, split-h, and split-v modes |
| **Context panel** | Show/hide the TM and terminology sidebar |
| **Search** | Filter blocks by source or target text |
| **Export** | Download the translated file in its original format |

### Target Locale Selector

When a project has multiple target locales, a dropdown in the toolbar lets you switch between target languages. The editor reloads blocks for the selected locale.

## Editing Blocks

### Inline Editing (Grid Mode)

1. Click a target cell or select a row and press **Enter**
2. Type the translation in the text input
3. Press **Enter** to save and advance to the next block
4. Press **Escape** to cancel editing

### Focus Mode Editing

1. Switch to focus mode from the toolbar
2. The target text area is immediately editable
3. Use the tag palette (if available) to insert inline tags
4. Press **Enter** or click **Save** to confirm

### Inline Tags

Many document formats contain inline markup (bold, links, placeholders, etc.) represented as coded text tags. The editor renders these as colored chips in the source text. When editing:

- Tags must be preserved in the target to maintain document structure
- The tag validation bar warns about missing or mismatched tags
- In focus mode, use **Ctrl+1** through **Ctrl+9** to insert tags from the tag palette

## Context Panel

Toggle the context panel from the toolbar to see per-block linguistic resources. The panel updates automatically as you navigate between blocks.

### TM Matches

When a block is selected, the context panel shows translation memory matches:

- **Score** — match percentage with color coding (green for 100% exact match, yellow for fuzzy)
- **Match type** — generalized, structural, or plain match
- **Source text** — the matched TM source
- **Target text** — the stored translation
- **Apply button** — one-click to copy the TM match into the target

The TM system uses three-tier matching:
1. **Generalized** — ignores inline tags for broader matching
2. **Structural** — considers tag structure but tolerates text changes
3. **Plain** — exact text matching including all tags

### Terminology

Below TM matches, the context panel shows terminology matches for the selected block:

- **Source term** — the term found in the source text
- **Target term** — suggested translation(s)
- **Status badge** — lifecycle status (preferred, approved, admitted, deprecated, proposed, forbidden)
- **Domain badge** — subject area classification

## Progress Tracking

The progress bar at the top of the editor shows translation progress:

- **Gray** — not started
- **Blue** — draft
- **Green** — translated
- **Purple** — reviewed

A percentage and "X/Y translated" counter provide numeric progress. The progress bar updates in real time as you translate blocks.

## Status Bar

The bottom of the editor shows:

- Current block position (Block N of M)
- Source word and character counts
- Target word counts per locale

## Keyboard Shortcuts

| Key | Action |
|-----|--------|
| **Enter** | Start editing / save and advance |
| **Escape** | Cancel editing |
| **Arrow Up/Down** or **j/k** | Navigate between blocks (grid mode) |
| **Ctrl+1** through **Ctrl+9** | Insert tag from palette (focus mode) |

## File Export

Click **Export** in the toolbar to download the translated file. The file is generated in its original format (HTML, XML, JSON, etc.) with all translations applied. In the browser, this triggers a file download. In Bowrain, the file is saved to disk and opened in your system file manager.
