# Bowrain Sample Data

This directory contains sample source files, termbases, and terminology data for testing and demonstrating Bowrain's translation features.

## Source Files

These files can be added to any Bowrain project using the "Add Files" button or drag-and-drop.

### help-center.html

A multi-section help center page with FAQs. Good for testing HTML format handling with nested elements, definition lists, and inline formatting (`<strong>`, `&amp;`).

### app-strings.json

Application UI strings with placeholder variables (`{remaining}`, `{count}`, `{percent}`). Tests JSON key extraction and string interpolation preservation.

## Sample Termbases

### sample-termbase.json

A comprehensive software development termbase with 10 concepts across 4 domains (development, infrastructure, security, UI/data) in 4 languages (en, fr, de, ja). Import this in the Terminology panel of any project.

### sample-terms.csv

A flat CSV termbase with English-French term pairs. Import using the CSV import feature in the Terminology panel.

## Workflow Walkthrough

1. **Create a project**: Launch Bowrain and create a new translation project
2. **Add files**: Drag and drop `help-center.html` or `app-strings.json` into the project
3. **Open the editor**: Click on a file to open the translation editor
4. **Enable Context panel**: Click "Context" in the toolbar to show TM matches and terminology
5. **Apply TM matches**: Navigate blocks with arrow keys; click "Apply" on TM matches to insert translations
6. **Bulk TM lookup**: Click "TM Lookup" to auto-fill all blocks that have TM matches
7. **Check terminology**: The Context panel shows matched terms with target suggestions
8. **Explore terminology**: Go back to the project view and click "Terminology" to browse/edit the termbase
9. **Import terms**: In the Terminology panel, click "Import CSV" to add terms from `sample-terms.csv`
