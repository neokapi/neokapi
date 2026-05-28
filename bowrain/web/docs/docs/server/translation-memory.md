---
sidebar_position: 4
title: Translation Memory
---

# Translation Memory

The Translation Memory (TM) Explorer provides full access to the workspace translation memory. TM entries are pairs of source and target text segments that can be reused across projects to maintain consistency and speed up translation.

## Accessing the TM Explorer

There are two ways to open the TM Explorer:

1. **From a project** — click the **Translation Memory** button in the project view header. This opens the TM Explorer with the project's source and target locales pre-selected.
2. **From the sidebar** — click **Memory** in the main sidebar to access the workspace-scoped TM Explorer without locale pre-filtering.

## Browsing Entries

The TM Explorer displays entries in a paginated table with these columns:

| Column            | Description                      |
| ----------------- | -------------------------------- |
| **Source**        | The source language text         |
| **Target**        | The target language translation  |
| **Source Locale** | Language code of the source text |
| **Target Locale** | Language code of the target text |
| **Updated**       | Date the entry was last modified |
| **Actions**       | Edit and Delete buttons          |

The entry count badge in the header shows the total number of entries matching current filters.

## Searching

Type in the search box to filter entries by source or target text. The search uses debounced input (300ms delay) to avoid excessive API calls while you type. Results update automatically as you type.

## Filtering by Locale

Use the locale filter dropdowns to narrow entries:

- **Source locale filter** — show only entries with a specific source language
- **Target locale filter** — show only entries with a specific target language

Filters can be combined with text search. Changing a filter resets to page 1.

## Adding Entries

1. Click **Add Entry** in the header
2. Fill in the form:
   - **Source text** — the original language text
   - **Target text** — the translation
   - **Source locale** — select from available project locales
   - **Target locale** — select from available project locales
3. Click **Add** to save, or **Cancel** to discard

## Editing Entries

1. Click **Edit** on an entry row
2. The target text becomes an inline text input
3. Modify the translation
4. Click **Save** to confirm, or **Cancel** to discard changes

Only the target text can be edited inline. To change the source text or locales, delete the entry and create a new one.

## Deleting Entries

Click **Delete** on an entry row. The entry is removed immediately and the table refreshes.

## Pagination

When there are more than 50 entries, pagination controls appear below the table:

- **Previous** / **Next** buttons to navigate pages
- **Page X of Y** indicator showing current position

## TM in the Translation Editor

The TM is also used within the translation editor:

- **TM Lookup** toolbar button — matches all source blocks against the TM and applies matches above the threshold
- **Context panel** — shows per-block TM matches with scores, match types, and one-click apply
- **TM Translate** — bulk-applies TM matches across the entire file

See [Translation Editor](./translation-editor.md) for details on context panel TM matching.

## Match scores

Each TM match receives a score from 0 to 100% representing how closely the source text matches the stored entry. Matches are color-coded:

- **Green** (100%) — exact match
- **Yellow** (70–99%) — fuzzy match
- Lower scores appear with reduced emphasis

Bowrain's matching accounts for inline formatting differences, so a source segment and a stored entry can match even if their tag structure differs slightly. For more on how translation memory works across projects, see the [Neokapi TM documentation](https://neokapi.github.io/web/neokapi/).
