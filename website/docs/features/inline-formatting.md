---
sidebar_position: 7
title: Inline Formatting
---

# Working with Inline Formatting

When you translate documents, some text contains formatting like **bold**, *italic*, [links](https://example.com), or embedded values like names and numbers. gokapi handles all of this automatically — you see the formatting naturally and the editor guides you to preserve it correctly.

## How Formatting Appears

### The Formatted View (Default)

In the default view, text appears with its natural formatting applied:

- **Bold text** appears bold
- *Italic text* appears italic
- Links appear underlined
- `Code` appears in a monospace font
- Line breaks appear as actual line breaks

This is the same experience whether your file is HTML, Markdown, XLIFF, or any other format. gokapi normalizes the underlying markup so you always see clean, readable text.

### The Code View (Advanced)

For advanced users who need to see the underlying structure, toggle the code view with the `</>` button. This shows colored tag chips that represent each formatting element:

- Opening tags like `B>` (bold start) and `a>` (link start)
- Closing tags like `/B` (bold end) and `/a` (link end)
- Self-contained elements like `br` (line break) and `img` (image)

Each tag type has its own color, making it easy to identify matching pairs.

## Translating Formatted Text

When editing a translation, the editor helps you preserve formatting:

### The Tag Palette

Below the text field, a strip of clickable buttons shows all the tags from the source text. Click a tag (or use **Ctrl+1** through **Ctrl+9**) to insert it at the cursor position.

Tags are grouped by their matching pairs — an opening bold tag and its closing counterpart appear together, making it easy to wrap text in formatting.

### What You Can and Cannot Change

Different types of formatting have different rules:

**Flexible tags** (like bold, italic, underline):
- You can remove them if the target language doesn't need them
- You can duplicate them to apply formatting to more text
- You can rearrange their position in the sentence

**Required elements** (like line breaks, variables, placeholders):
- They must appear in your translation — the editor prevents accidental deletion
- They appear with a dashed border to indicate they're required
- The tag palette blocks you from inserting them twice

**Variables and placeholders** (like `{userName}` or `{count}`):
- These represent dynamic values that get filled in at runtime
- They must be kept exactly as they are — don't translate them
- You can move them to a different position in the sentence to match your target language grammar

### Validation

As you type, the editor validates your translation in real time:

- **Missing tags** — a red bar appears if you've forgotten a required element
- **Extra tags** — a yellow bar appears if you've duplicated something that shouldn't be duplicated
- **Unpaired tags** — an error appears if you have an opening tag without its matching close

## The Inline Code Legend

Click the tag summary badge in the editor header to open the inline code legend. This panel shows:

- All tag types in the current segment, grouped by category
- What each tag represents (Bold, Hyperlink, Variable, etc.)
- Which tags are required, which can be duplicated, and which have fixed positions
- A quick reference for the constraint icons

## Working with Different File Formats

### HTML Files

HTML documents contain tags like `<b>`, `<a href="...">`, and `<br/>`. In the editor, these appear as natural formatting — bold text looks bold, links are underlined, and line breaks separate paragraphs.

**Example source:**
> Click **here** to visit our *website* for more information.

The bold and italic formatting is fully flexible — you can keep, remove, or rearrange it as needed for your target language.

### Markdown Files

Markdown uses `**`, `*`, backticks, and `[]()` syntax. In the editor, this looks identical to HTML — `**bold**` and `<b>bold</b>` both appear the same way.

**Example source:**
> Run `kapi init` to set up your project. See the [documentation](https://docs.example.com) for details.

The inline code (`kapi init`) and link are preserved exactly, while you translate the surrounding text.

### JSON/YAML Localization Files

i18n files often contain variables and ICU message format expressions:

**Example source:**
> Hello {userName}, you have {count} new messages.

The variables `{userName}` and `{count}` appear as orange tag chips and are marked as required. You cannot delete them, but you can rearrange them to match your target language's word order:

> Bonjour {userName}, vous avez {count} nouveaux messages.

### XLIFF Exchange Files

XLIFF files use `<pc>`, `<ph>`, and `<sc>`/`<ec>` elements for inline codes. gokapi maps these to the same visual representations as HTML and Markdown, so you work with the same familiar editor experience regardless of the exchange format.

## Tips for Translators

1. **Trust the formatted view.** The editor shows you exactly what the output will look like. You don't need to understand the underlying markup.

2. **Watch for the dashed borders.** Tags with dashed borders are required — make sure they appear in your translation.

3. **Use keyboard shortcuts.** Press **Ctrl+1** through **Ctrl+9** to insert tags quickly instead of clicking.

4. **Check the validation bar.** If you see red or yellow messages below the editor, review your translation for missing or extra tags.

5. **Don't translate variables.** Elements like `{userName}`, `{count}`, or `{0}` are placeholders for dynamic content. Keep them exactly as they are.

6. **Feel free to rearrange.** If your target language puts the verb before the subject, you can move tags to different positions — unless they have a "fixed position" constraint.
