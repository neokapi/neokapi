---
sidebar_position: 6
title: Walkthroughs
---

# Walkthroughs

Step-by-step guides for common workflows. The steps are the same in the
browser and the [desktop app](/server/desktop-app); both are clients of the
same server. The first two curate the context a workspace governs content
with; the rest carry content through drafting, memory reuse and review.

## Curate the terms a workspace governs with

Build a terms store so every draft and every check uses the same vocabulary.

### Steps

1. Open **Context → Concepts** in the sidebar to reach the terms explorer
2. Click **Add Concept** to create entries:
   - Source: "dashboard" / Target: "tableau de bord" / Domain: "UI" / Status: "preferred"
   - Source: "login" / Target: "connexion" / Domain: "UI" / Status: "approved"
3. Open your project and open a file in the editor
4. Select a block that contains one of your terms (for example "dashboard")
5. The context panel's Terms section shows the matching terms:
   - Source term with status badge
   - Target suggestion
   - Domain label
6. Use the suggested term for consistency
7. Repeat for other blocks containing managed terms

### What you learned

- Creating concepts with a domain and a lifecycle status
- Automatic term detection in the editor context panel
- Using term suggestions for consistent wording

## Import terms from CSV

Bulk-load terms from a spreadsheet export.

### Steps

1. Prepare a CSV file with columns: `source_term`, `target_term`, `source_locale`, `target_locale`, `domain`, `status`
   ```csv
   source_term,target_term,source_locale,target_locale,domain,status
   login,connexion,en,fr,UI,approved
   password,mot de passe,en,fr,security,preferred
   dashboard,tableau de bord,en,fr,UI,preferred
   settings,paramètres,en,fr,UI,approved
   ```
2. Open **Context → Concepts** in the sidebar
3. Click **Import CSV**
4. Select your CSV file
5. The concepts appear in the concept list
6. Verify the imported terms, statuses, and domains

### What you learned

- CSV format for term import
- Bulk-loading terms into the workspace terms store
- Verifying imported concepts

## Reuse from the content memory

Reuse approved wording to keep drafts consistent and reduce effort.

### Steps

1. Open **Content memory** in the sidebar
2. Click **Add Entry** to add some memory entries:
   - Source: "Welcome to our website" / Target: "Bienvenue sur notre site" (en → fr)
   - Source: "Contact us" / Target: "Contactez-nous" (en → fr)
3. Open your project
4. Open a file, switch to the editor's [file-wide operations](/server/translation-editor#file-wide-operations)
   and choose **Recycle from memory**
5. The system matches source blocks against the memory entries and fills in matches
6. Check the progress bar: matched blocks show as "translated"
7. Back in the editor, toggle the **Context panel** to see per-block memory
   match details:
   - Match score (100% for exact matches)
   - Match type (generalized, structural, or plain)
   - Source and target text
8. For partial matches, click **Apply** in the context panel to accept the suggestion
9. Edit the applied text if needed

### What you learned

- Adding entries to the content memory
- Bulk reuse from memory across an entire file
- Understanding match scores and match types
- Applying memory suggestions from the context panel

## Translate a website from English to French

This walkthrough covers the complete workflow from project creation to file export.

### Steps

1. Open Bowrain and go to the project list
2. Click **New Project**
3. Enter "Website" as the project name
4. Select **English** as the source language
5. Select **French** as the target language
6. Click **Create**; the project opens in the project view
7. Drag your HTML files onto the upload zone (or click **Add Files**)
8. Files appear in the file list with format detection and word counts
9. Click a file name to open it in the editor
10. The editor displays source blocks on the left and empty target cells on the right
11. Click a target cell and type the French text
12. Press **Enter** to save and move to the next block
13. Continue through all blocks, using the progress bar to track completion
14. Click **Export** in the toolbar to download the French HTML file

### What you learned

- Creating a project with source and target languages
- Uploading files with format auto-detection
- Manual block-by-block translation
- Exporting files in their original format

## Use AI to draft a JSON file

Let an AI provider produce the first draft, then review and refine it.

### Steps

1. Create a new project with your JSON file (follow steps 1-8 above)
2. Open the file in the editor
3. Ensure a provider is configured (the workspace's credits, or a key under
   **Settings → Providers**)
4. Click **AI draft** in the toolbar
5. Wait for the provider to process all blocks; the progress bar updates in real time
6. Open the [review session](/server/review) for the file
7. Approve, reject or edit each draft, or approve everything that passes the
   checks and the voice bar in one action
8. Export the finished file

### What you learned

- Drafting a whole file with an AI provider
- Reviewing and refining machine-produced drafts
- Block status workflow (not started → translated → reviewed)

## Multi-language project workflow

Carry a single source file into several target languages.

### Steps

1. Create a new project with multiple target languages (for example French, German, Japanese)
2. Upload your source file
3. Open the file in the editor
4. The **target locale selector** in the toolbar shows "French" (the first target language)
5. Work through all blocks in French
6. Switch the target locale to **German** using the dropdown
7. The editor reloads with empty targets for German
8. Work through all blocks in German (or use **AI draft** for a first pass)
9. Switch to **Japanese** and repeat
10. Export the file for each target locale; each export produces the file with that locale's targets

### What you learned

- Creating projects with multiple target languages
- Switching between target locales in the editor
- Independent progress per locale
- Per-locale file export
