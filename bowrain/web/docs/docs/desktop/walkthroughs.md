---
sidebar_position: 4
title: Walkthroughs
---

# Walkthroughs

Step-by-step guides for common Bowrain workflows, designed to be followed along with the sample projects.

## Walkthrough 1: TM Leverage — Auto-Fill Translations from Memory

**Goal**: Open a half-translated project and use translation memory to fill in missing translations.

**Sample project**: Website Translation

### Steps

1. **Launch Bowrain** and click "Open Project"
2. **Select** the Website Translation sample project
3. **Review the project**: You'll see `index.html` with its block count — some blocks already have French translations, German is mostly empty
4. **Click** on `index.html` to open the translation editor
5. **Notice** the progress bar: some blocks are translated (blue), some are not started (gray)
6. **Click "TM Lookup"** in the toolbar
   - Bowrain queries the project's translation memory for every untranslated block
   - Blocks with exact TM matches are filled automatically
   - The progress bar updates to reflect the new translations
7. **Click "Context"** in the toolbar to open the Context panel
8. **Navigate to a translated block** using the arrow keys or by clicking
   - The Context panel shows the TM match: source text, target text, match score (100% for exact), and match type
   - For blocks filled by TM Lookup, you'll see the source of the match
9. **Navigate to a partially matching block** — the Context panel may show fuzzy matches (< 100%) that you can review and apply manually
10. **Click "Apply"** on a fuzzy match to insert it, then edit as needed
11. **Save the project** with Cmd/Ctrl+S — TM entries are persisted in the project database

### What You Learned

- TM Lookup batch-applies matches to all untranslated blocks
- The Context panel provides per-block TM matches for manual review
- Exact matches are applied automatically; fuzzy matches require manual review
- TM data persists in the project database

---

## Walkthrough 2: Terminology Management — Build and Use a Termbase

**Goal**: Import terminology, browse concepts, and see term suggestions while translating.

**Sample project**: Software UI

### Steps

1. **Open** the Software UI sample project — this is a new project with 33 UI string blocks and a pre-loaded termbase
2. **Click "Terminology"** in the project view to open the Terminology Explorer
3. **Browse concepts**: You'll see concepts like "Task" (en/fr/de/ja), "Dashboard" (en/fr/de/ja), etc.
   - Each concept shows its domain, definition, and terms with status badges
   - Status colors: green for preferred, blue for approved, yellow for admitted
4. **Search** for "save" — the search filters concepts by term text
5. **Click "Import CSV"** to import additional terms
   - Select `sample-terms.csv` from the samples directory
   - Set source locale to `en`, target locale to `fr`
   - The imported terms are added as new concepts
6. **Click "Add Concept"** to create a custom concept:
   - Domain: "ui"
   - Definition: "Action to remove an item permanently"
   - Add terms: "delete" (en, preferred), "supprimer" (fr, preferred)
7. **Go back to the project view** and click on a file to open the editor
8. **Click "Context"** in the toolbar
9. **Navigate** to a block containing a known term (e.g., "Dashboard", "Settings")
   - The Terminology section shows matched terms with target suggestions
   - Each match shows: source term, target term(s), domain, and lifecycle status
10. **Use the term suggestions** to ensure consistent terminology across your translations
11. **Save** — the termbase persists in the project database

### What You Learned

- The Terminology Explorer provides full CRUD for concepts and terms
- CSV import adds flat term pairs as concepts
- The Context panel shows per-block terminology suggestions during editing
- Terms have lifecycle statuses that guide translators on which term to use

---

## Walkthrough 3: Context Panel — TM + Terminology Side-by-Side

**Goal**: Use the Context panel to leverage both TM matches and terminology while translating.

**Sample project**: Website Translation

### Steps

1. **Open** the Website Translation sample project and click on `index.html`
2. **Click "Context"** in the toolbar to open the side panel
3. **The panel has two sections**:
   - **TM Matches**: Shows translation memory matches for the current block
   - **Terminology**: Shows terms found in the current block's source text
4. **Navigate to a block** with both TM and term matches:
   - TM matches show source, target, score badge (green for 100%, yellow for fuzzy), and match type
   - Terminology matches show the matched source term, target suggestions, domain badge, and status badge
5. **Click "Apply"** on a TM match — the target text is filled with the TM match
6. **Review terminology** — check that the applied translation uses the preferred terms
7. **Navigate to another block** — the panel updates automatically
8. **For blocks without TM matches**, use terminology suggestions as a starting point for manual translation

### What You Learned

- The Context panel shows both TM and terminology for each block
- TM matches provide full sentence translations; terminology provides term-level guidance
- The panel updates as you navigate between blocks
- TM and terminology work together to improve translation quality and consistency

---

## Walkthrough 4: Complete Translation Workflow

**Goal**: Translate a new project end-to-end using all available resources.

**Sample project**: Software UI

### Steps

1. **Open** the Software UI sample project — 33 untranslated blocks with a 27-entry TM
2. **Click "TM Lookup"** — blocks with TM matches are auto-filled (look for progress bar change)
3. **Open the editor** and enable the **Context panel**
4. **Work through remaining blocks**:
   - For each untranslated block, check the Context panel for TM fuzzy matches and terminology
   - Apply TM matches when the score is high enough
   - Use terminology suggestions for consistent term usage
   - Manually translate blocks that have no matches
5. **Mark completed blocks as reviewed** using Cmd/Ctrl+Shift+R
6. **Check progress** — the progress bar shows not-started (gray), draft (yellow), translated (blue), and reviewed (green)
7. **Browse the Terminology Explorer** to verify your translations use preferred terms
8. **Save the project** — all translations, TM entries, and termbase are saved

### What You Learned

- The recommended workflow: TM Lookup first, then manual translation with Context panel
- Block status tracking gives clear visibility into translation progress
- Terminology enforcement ensures consistency across the project

---

## Walkthrough 5: Export and Review

**Goal**: Review a completed translation and export the results.

**Sample project**: Marketing Content

### Steps

1. **Open** the Marketing Content sample project — fully translated in 3 target languages (fr, de, es)
2. **Open the editor** — all blocks show as translated (blue progress bar)
3. **Switch target locale** to review each language
4. **Enable the Context panel** to cross-reference translations with TM and terminology
5. **Switch to Visual view** to see the live document preview alongside the inline editing card
6. **Click on blocks** in the preview to jump to them
7. **Mark reviewed blocks** as you go through them

### What You Learned

- Completed projects can be reviewed across multiple target locales
- The split layout with live preview helps catch formatting issues
- The Context panel is useful for review as well as initial translation
