# Bowrain Sample Projects & Data

This directory contains sample project files, termbases, and source content for testing and demonstrating Bowrain's translation features.

## Sample KAZ Projects

Open these `.kaz` files directly in Bowrain to explore TM leverage, terminology management, and the translation editor.

### website-translation.kaz
- **Status**: Half-translated (English → French partially complete, German mostly untranslated)
- **Content**: Corporate website landing page (HTML)
- **TM**: 8 entries covering common phrases and product terminology
- **Termbase**: 6 concepts including product names, brand terms, and technical terminology
- **Use case**: Demonstrates TM leverage — open the project, click "TM Lookup" to auto-fill translations from the TM, then use the Context panel to see per-block matches

### software-ui.kaz
- **Status**: New project, no translations yet
- **Content**: Task management application UI strings (JSON, 33 blocks)
- **TM**: 27 entries from "previous projects" covering standard software UI strings (File, Edit, Save, etc.)
- **Termbase**: 5 concepts covering UI terms and the product brand name in 4 languages (en, fr, de, ja)
- **Use case**: Demonstrates starting a new project with existing TM. Use "TM Lookup" to pre-fill common strings, then translate the remaining content. The Context panel shows relevant terminology for each block.

### marketing-content.kaz
- **Status**: Fully translated (English → French, German, Spanish)
- **Content**: Product marketing landing page (HTML)
- **TM**: 6 entries from this translation
- **Termbase**: 3 concepts (product name, security term, marketing term)
- **Use case**: A completed project demonstrating all translations filled in. Good for testing export and review workflows.

## Standalone Source Files

These files can be added to any Bowrain project using the "Add Files" button or drag-and-drop.

### help-center.html
A multi-section help center page with FAQs. Good for testing HTML format handling with nested elements, definition lists, and inline formatting (`<strong>`, `&amp;`).

### app-strings.json
Application UI strings with placeholder variables (`{remaining}`, `{count}`, `{percent}`). Tests JSON key extraction and string interpolation preservation.

## Sample Termbases

### sample-termbase.json
A comprehensive software development termbase with 10 concepts across 4 domains (development, infrastructure, security, UI/data) in 4 languages (en, fr, de, ja). Import this in the Terminology panel of any project.

### sample-terms.csv
A flat CSV termbase with English→French term pairs. Import using the CSV import feature in the Terminology panel.

## Generating Sample Projects

The `.kaz` files are generated from `generate_samples.go`. To regenerate:

```bash
cd apps/bowrain/samples
go run generate_samples.go
```

## Workflow Walkthrough

1. **Open a project**: Launch Bowrain, click "Open Project" and select `website-translation.kaz`
2. **Explore the project**: See file list with block counts and word counts
3. **Open the editor**: Click on `index.html` to open the translation editor
4. **Enable Context panel**: Click "Context" in the toolbar to show TM matches and terminology
5. **Apply TM matches**: Navigate blocks with arrow keys; click "Apply" on TM matches to insert translations
6. **Bulk TM lookup**: Click "TM Lookup" to auto-fill all blocks that have TM matches
7. **Check terminology**: The Context panel shows matched terms with target suggestions
8. **Explore terminology**: Go back to the project view and click "Terminology" to browse/edit the termbase
9. **Import terms**: In the Terminology panel, click "Import CSV" to add terms from `sample-terms.csv`
10. **Save project**: Click "Save" — TM entries and termbase are persisted in the `.kaz` file
