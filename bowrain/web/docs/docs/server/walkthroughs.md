---
sidebar_position: 6
title: Walkthroughs
---

# Walkthroughs

Step-by-step guides for common translation workflows. The steps are the same in
the browser and the [desktop app](/server/desktop-app) — both are clients of the
same server.

## Translate a Website from English to French

This walkthrough covers the complete workflow from project creation to file export.

### Steps

1. Open Bowrain and navigate to the **Translate** view
2. Click **New Project** on the dashboard
3. Enter "Website Translation" as the project name
4. Select **English** as the source language
5. Select **French** as the target language
6. Click **Create** — the project opens in the project view
7. Drag your HTML files onto the upload zone (or click **Add Files**)
8. Files appear in the file list with format detection and word counts
9. Click a file name to open it in the translation editor
10. The editor displays source blocks on the left and empty target cells on the right
11. Click a target cell and type the French translation
12. Press **Enter** to save and move to the next block
13. Continue translating all blocks, using the progress bar to track completion
14. Click **Export** in the toolbar to download the translated HTML file

### What You Learned

- Creating a project with source and target languages
- Uploading files with format auto-detection
- Manual block-by-block translation
- Exporting translated files in their original format

---

## Use AI to Translate a JSON File

Accelerate translation by using an AI provider to generate initial translations, then review and refine them.

### Steps

1. Create a new project with your JSON file (follow steps 1-8 above)
2. Open the file in the translation editor
3. Ensure an AI provider is configured (check with your administrator)
4. Select the AI provider from the toolbar dropdown if multiple are available
5. Click **AI Translate** in the toolbar
6. Wait for the AI to process all blocks — the progress bar updates in real time
7. Review the AI-generated translations block by block
8. Edit any translations that need refinement
9. Mark reviewed blocks by clicking **Reviewed** in the toolbar
10. Export the finished file

### What You Learned

- Configuring and selecting AI translation providers
- Bulk AI translation of an entire file
- Reviewing and refining machine-generated translations
- Block status workflow (not-started → translated → reviewed)

---

## Leverage Translation Memory

Reuse previous translations to maintain consistency and reduce effort.

### Steps

1. Navigate to **Memory** in the sidebar to open the TM Explorer
2. Click **Add Entry** to add some translation memory entries:
   - Source: "Welcome to our website" / Target: "Bienvenue sur notre site" (en → fr)
   - Source: "Contact us" / Target: "Contactez-nous" (en → fr)
3. Return to the **Translate** view and open your project
4. Open a file in the translation editor
5. Click **TM Lookup** in the toolbar
6. The system matches source blocks against TM entries and fills in matches
7. Check the progress bar — matched blocks show as "translated"
8. Toggle the **Context panel** to see per-block TM match details:
   - Match score (100% for exact matches)
   - Match type (generalized, structural, or plain)
   - Source and target text
9. For partial matches, click **Apply** in the context panel to accept the suggestion
10. Edit the applied translation if needed

### What You Learned

- Adding entries to the translation memory
- Bulk TM lookup across an entire file
- Understanding match scores and match types
- Applying TM suggestions from the context panel

---

## Manage Terminology

Build a termbase to enforce consistent vocabulary across translations.

### Steps

1. Navigate to **Termbase** in the sidebar to open the Terminology Explorer
2. Click **Add Concept** to create terminology entries:
   - Source: "dashboard" / Target: "tableau de bord" / Domain: "UI" / Status: "preferred"
   - Source: "login" / Target: "connexion" / Domain: "UI" / Status: "approved"
3. Return to the **Translate** view and open your project
4. Open a file in the translation editor
5. Select a block that contains one of your terms (e.g., "dashboard")
6. Toggle the **Context panel** — the Terminology section shows matching terms:
   - Source term with status badge
   - Target suggestion
   - Domain label
7. Use the suggested term in your translation for consistency
8. Repeat for other blocks containing managed terminology

### What You Learned

- Creating terminology concepts with domain and lifecycle status
- Automatic term detection in the editor context panel
- Using terminology suggestions for consistent translation

---

## Import Terminology from CSV

Bulk-load terminology from a spreadsheet export.

### Steps

1. Prepare a CSV file with columns: `source_term`, `target_term`, `source_locale`, `target_locale`, `domain`, `status`
   ```csv
   source_term,target_term,source_locale,target_locale,domain,status
   login,connexion,en,fr,UI,approved
   password,mot de passe,en,fr,security,preferred
   dashboard,tableau de bord,en,fr,UI,preferred
   settings,paramètres,en,fr,UI,approved
   ```
2. Navigate to **Termbase** in the sidebar
3. Click **Import CSV**
4. Select your CSV file
5. The concepts appear in the termbase list
6. Verify the imported terms, statuses, and domains

### What You Learned

- CSV format for terminology import
- Bulk-loading terminology into the workspace termbase
- Verifying imported concepts

---

## Multi-Language Project Workflow

Translate a single source file into multiple target languages.

### Steps

1. Create a new project with multiple target languages (e.g., French, German, Japanese)
2. Upload your source file
3. Open the file in the translation editor
4. The **target locale selector** in the toolbar shows "French" (the first target language)
5. Translate all blocks into French
6. Switch the target locale to **German** using the dropdown
7. The editor reloads with empty targets for German
8. Translate all blocks into German (or use AI Translate for initial translations)
9. Switch to **Japanese** and repeat
10. Export the file for each target locale — each export produces the file with translations for that locale

### What You Learned

- Creating projects with multiple target languages
- Switching between target locales in the editor
- Independent translation progress per locale
- Per-locale file export
