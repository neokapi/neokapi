---
sidebar_position: 6
title: "Bowrain UI Components"
---

# Bowrain UI Components

This note provides implementation details for [AD-012](/docs/ad/012-bowrain).

## Translation Editor

Five layout modes:

- **Visual** (default) -- Visual editor with rich formatting and inline editing
- **Grid** -- Table view with source and target columns for all blocks. Efficient for scanning and bulk editing
- **Focus** -- Single-block deep editing with full-width source/target panels and block navigation (prev/next). Term highlights and TM matches are shown inline
- **Split Horizontal / Vertical** -- Side-by-side or stacked editing with a live document preview pane

Block status tracking: `not-started`, `draft` (has target text), `translated` (translation-origin property set), `reviewed` (translation-status is "reviewed"). A status-colored progress bar shows block completion at a glance.

Enhanced toolbar: Copy Source to Target, Mark as Reviewed, Navigate to Prev/Next Untranslated.

## Context Panel

Rich metadata display alongside the translation editor, providing translators with the information they need without leaving the editing view:

- **TM matches** -- Fuzzy and exact matches from Sievepen ([AD-009](/docs/ad/009-translation-memory)) with match scores, entity adaptations, and one-click application to the target field
- **Terminology** -- Recognized terms highlighted inline in the source text with definitions, preferred translations, and status. Click to navigate to the concept in the terminology module ([AD-010](/docs/ad/010-terminology))
- **Display hints** -- Content type, max length, and preview from the source system, carried on Blocks via the content model ([AD-002](/docs/ad/002-content-model))
- **ContentRef** -- Link back to the source item in the connected system, enabling translators to view content in its original context ([AD-002](/docs/ad/002-content-model))

## Flow Editor

Built on React Flow (@xyflow/react v12) for drag-and-drop visual flow building:

- Nodes represent readers, tools, and writers with color-coded type indicators (Transform, Enrich, Validate)
- Edges represent the data flow between nodes
- Add/remove nodes and edges interactively
- Built-in flows available as templates; user-created flows are persisted via FlowStore
- Flow definitions are validated (cycle detection via TopologicalOrder, node reference integrity) before saving

## Workspace Rail Layout

Bowrain uses a Slack-inspired two-panel sidebar layout for workspace navigation:

- **Workspace Rail** (60px, dark background) -- Far-left icon rail showing workspace icons (first letter or custom logo with colored badge). Active workspace has a pill-shaped highlight. Bottom section has "+" button for creating workspaces and a user avatar with account menu
- **Main Sidebar** (220px) -- Navigation panel for the active workspace with sections: Translate (project list), Termbase, Memory (TM), Flows, Connectors, Settings. Below navigation: collapsible project list for quick switching

```
+--------+------------+------------------------------------------+
| Rail   | Sidebar    |  Main Content                            |
| 60px   |  220px     |                                          |
| [WS]   | Translate  |  (ProjectDashboard/Editor/TM/Terms)      |
| [WS]   | Termbase   |                                          |
|        | Memory     |                                          |
|  +     | Flows      |                                          |
| [AV]   | Settings   |                                          |
+--------+------------+------------------------------------------+
```

In local/desktop mode, a "Personal" workspace is created automatically. When connected to a `bowrain-server` instance, the rail shows all workspaces the user belongs to with role-based access ([AD-015](/docs/ad/015-auth-and-workspaces)).

## Shared Component Library (`packages/ui/`)

Core UI components are extracted to `packages/ui/` (`@neokapi/ui-primitives`) for reuse across Bowrain (desktop), Kapi, and the web app. The library includes:

- **Layout**: `WorkspaceRail`, `AppSidebar`, `AccountMenu`, `WorkspaceIcon`, `PageHeader`, `EmptyState`, `SkeletonCard`, `PanelHeader`, `LoadingSpinner`
- **Context**: `AuthContext`, `WorkspaceContext` with React hooks
- **API Adapter**: `ApiAdapter` interface with platform-specific implementations -- `RestApiAdapter` for the web app, Wails bindings for desktop

Bowrain imports from `@neokapi/ui` and provides a Wails-specific API adapter that bridges Go method calls to the shared `ApiAdapter` interface. This ensures the same UI components render identically in both desktop and browser contexts.

## Offline Queue

When the gRPC connection drops, the app transitions to `offline` state and continues working against the local cache:

- **Local cache** -- On project open, blocks are fetched from the server and stored locally. Reads always serve from the local ContentStore (fast). Writes go to the server first; on success, the local cache updates
- **Offline queue** -- On write failure (network error), mutations are queued in a SQLite-backed `OfflineQueue` at `<UserConfigDir>/bowrain-desktop/offline-queue.db`. Operations tracked: `update_block_target`, `update_block_target_coded`, `review_block`, `add_tm_entry`, `update_tm_entry`, `delete_tm_entry`, `add_concept`, `update_concept`, `delete_concept`
- **Reconnection** -- A background goroutine (`reconnectLoop`) pings the server with exponential backoff (2s -> 60s cap). On successful reconnect, pending changes replay in FIFO batches of 10, and the `WatchProject` stream resumes
- **Frontend indicators** -- The header shows "Offline" with a count of pending changes (e.g., "3 pending"). When reconnected, the count clears and the status returns to "Connected"

The offline queue persists across app restarts -- changes are never lost even if the app is closed while offline. Failed replays increment an attempt counter and record the error for debugging.

## Terminology Module

Dedicated terminology management interface ([AD-010](/docs/ad/010-terminology)):

- **Browse and search** -- Faceted search by locale, domain, product, status. Concept graph visualization for related terms. Term concordance showing all Blocks where a term appears in the current project
- **Edit and review** -- Create/edit concepts and terms. Inline term suggestion during translation (right-click selected text to suggest a new term). Moderation queue for proposed terms with approval workflow
- **Import and export** -- CSV, TBX, JSON import with field mapping UI. Export in any supported format. Merge imported terms with existing termbase via conflict resolution UI
- **Analytics** -- Term usage statistics across project content. Coverage (percentage of source terms with approved translations). Consistency (preferred terms vs. variants in target text). Freshness (terms not reviewed since configurable date)
- **Editor integration** -- Recognized terms highlighted inline in the translation editor. Hover to see definition, preferred translation, and status. Click to navigate to the term in the terminology module

## Translation Memory Explorer

- Fuzzy match visualization with score indicators
- TM entry browsing with entity mapping display showing generalized vs. plain matches ([AD-009](/docs/ad/009-translation-memory))
- TMX import/export for interoperability with external TM systems
