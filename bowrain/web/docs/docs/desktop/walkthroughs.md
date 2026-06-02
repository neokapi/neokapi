---
sidebar_position: 6
title: Walkthroughs
---

# Walkthroughs

Short, follow-along guides for common desktop workflows. They assume you are
[signed in to a workspace](/desktop/getting-started) and editing a project that
lives on the server. Edits commit to the server as you work (and queue locally
when offline) — there is no separate "save to a local file" step.

The examples use the **BowMart** workspace and its cast: **Maya** (translator,
fr-FR), **Jonas** (reviewer), and **Priya** (project admin).

## Leverage translation memory

**Goal**: fill in missing translations from the workspace's shared memory.

1. Maya opens the `store-frontend` project and clicks a file to open the
   translation editor.
2. The progress bar shows the mix of statuses — some blocks translated (blue),
   some not started (gray).
3. Click **TM Lookup** in the toolbar. The desktop queries the project's
   server-hosted translation memory for every untranslated block and fills the
   exact matches automatically; the progress bar updates.
4. Click **Context** to open the side panel, then navigate between blocks. For a
   filled block the panel shows the TM match — source, target, score (100% for
   exact), and match type.
5. On a block with only fuzzy matches (below 100%), click **Apply** to insert
   the closest one and edit as needed.

Filled translations commit to the server and appear for the rest of the
workspace; the local mirror updates in step.

## Manage terminology

**Goal**: browse the shared termbase and see term suggestions while translating.

1. From the navigation panel, open **Termbase** to browse the workspace's
   concept-oriented terminology. Each concept shows its domain, definition, and
   terms with lifecycle-status badges (preferred, approved, admitted,
   deprecated, proposed, forbidden).
2. Search by term text to filter concepts.
3. Add a concept, or import terms from a CSV (source/target pairs) or a full
   JSON termbase. Imported terms become server-side concepts visible to the
   whole workspace.
4. Back in the editor, open the **Context** panel and navigate to a block whose
   source contains a known term. The terminology section lists the matched term,
   target suggestions, domain, and status — so Maya keeps usage consistent with
   what Priya has approved.

## Use the Context panel: TM and terminology together

**Goal**: combine memory matches and terminology while translating a block.

1. With a file open, click **Context** to open the side panel. It has two
   sections: **TM matches** and **Terminology**.
2. Navigate to a block with both. TM matches show source, target, a score badge
   (green for 100%, yellow for fuzzy), and match type; terminology matches show
   the matched source term, target suggestions, a domain badge, and a status
   badge.
3. Click **Apply** on a TM match to fill the target, then check the result
   against the preferred terms shown below.
4. For blocks without a TM match, use the terminology suggestions as a starting
   point for a manual translation.

The panel updates automatically as you move between blocks.

## Review and hand off

**Goal**: review translated work across target locales and approve it.

1. Jonas opens the same project and switches to the **Review** surface to work
   through blocks by status.
2. Switch the target locale to review each language in turn; open the **Context**
   panel to cross-reference TM and terminology.
3. In **Visual** view, the live document preview sits beside the inline editing
   card — click a block in the preview to jump to it, which helps catch
   formatting issues.
4. Mark blocks reviewed with `Cmd/Ctrl+Shift+R`. Because the project is hosted
   server-side, Maya sees the review state update live, and the correction loop
   feeds back into the workspace's shared checks (see
   [Brand voice](/server/brand-voice)).

## What these show

- TM Lookup batch-fills exact matches; the Context panel surfaces fuzzy matches
  and terminology for manual review.
- TM and terminology are **shared and server-hosted**, so everyone in the
  workspace works from the same memory and the same approved terms.
- Block status tracks progress; review and approval happen live across the
  desktop and web clients.
