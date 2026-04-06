---
sidebar_position: 2
title: Supported Formats
---

# Supported Formats

neokapi includes 42 built-in format readers and writers covering document, localization, subtitle, office, and specialized text formats.

## Built-in Formats

### Localization & Translation Formats

| Format                   | ID              | Extensions              | Description                                    |
| ------------------------ | --------------- | ----------------------- | ---------------------------------------------- |
| **XLIFF 1.2**            | `xliff`         | `.xlf`, `.xliff`        | XLIFF 1.2 bilingual exchange format            |
| **XLIFF 2.0**            | `xliff2`        | `.xlf`, `.xliff`        | XLIFF 2.0/2.1 with segment support             |
| **PO (Gettext)**         | `po`            | `.po`, `.pot`           | GNU gettext translation files                  |
| **TMX**                  | `tmx`           | `.tmx`                  | Translation Memory eXchange                    |
| **Qt TS**                | `ts`            | `.ts`                   | Qt Linguist translation files                  |
| **ICU MessageFormat**    | `messageformat` | `.mf`, `.messageformat` | ICU MessageFormat for pluralization and gender |
| **Trados TagEditor TTX** | `ttx`           | `.ttx`                  | Trados TagEditor translation format            |
| **Trados XML**           | `txml`          | `.txml`                 | Trados native XML format                       |
| **Translation Table**    | `transtable`    | `.tab`, `.tsv`          | Two-column source/target translation table     |

### Document & Markup Formats

| Format        | ID         | Extensions                | Description                                         |
| ------------- | ---------- | ------------------------- | --------------------------------------------------- |
| **HTML**      | `html`     | `.html`, `.htm`, `.xhtml` | HTML with configurable element/attribute rules      |
| **XML**       | `xml`      | `.xml`                    | Generic XML with configurable translatable elements |
| **Markdown**  | `markdown` | `.md`, `.markdown`        | Markdown with inline code preservation              |
| **Wiki**      | `wiki`     | `.wiki`, `.mediawiki`     | MediaWiki and DokuWiki markup                       |
| **TeX/LaTeX** | `tex`      | `.tex`, `.latex`          | TeX/LaTeX document extraction                       |
| **DTD**       | `dtd`      | `.dtd`                    | XML Document Type Definition                        |
| **RTF**       | `rtf`      | `.rtf`                    | Rich Text Format documents                          |

### Data & Configuration Formats

| Format               | ID           | Extensions                               | Description                                     |
| -------------------- | ------------ | ---------------------------------------- | ----------------------------------------------- |
| **JSON**             | `json`       | `.json`                                  | Highly configurable JSON with regex-based rules |
| **YAML**             | `yaml`       | `.yaml`, `.yml`                          | YAML with code finder and key path filtering    |
| **Java Properties**  | `properties` | `.properties`                            | Java properties files                           |
| **CSV**              | `csv`        | `.csv`                                   | Comma-separated values with column control      |
| **TSV**              | `tsv`        | `.tsv`                                   | Tab-separated values                            |
| **Fixed-Width**      | `fixedwidth` | `.txt`, `.dat`, `.fixed`                 | Fixed-width column text files                   |
| **PHP Content**      | `phpcontent` | `.php`, `.phpcnt`                        | PHP source files with okapi directives          |
| **Regex Extraction** | `regex`      | `.strings`, `.ini`, `.info`, `.rls`      | Regex-based line-by-line extraction             |
| **Doxygen Comments** | `doxygen`    | `.c`, `.cpp`, `.h`, `.java`, `.m`, `.py` | Doxygen/Javadoc comment extraction              |

### Office & Desktop Publishing Formats

| Format                    | ID        | Extensions                     | Description                         |
| ------------------------- | --------- | ------------------------------ | ----------------------------------- |
| **Office Open XML**       | `openxml` | `.docx`, `.xlsx`, `.pptx`, ... | Microsoft Word, Excel, PowerPoint   |
| **Open Document**         | `odf`     | `.odt`, `.ods`, `.odp`         | LibreOffice/OpenOffice documents    |
| **ICML (Adobe InCopy)**   | `icml`    | `.icml`, `.wcml`               | Adobe InCopy Markup Language        |
| **IDML (Adobe InDesign)** | `idml`    | `.idml`                        | Adobe InDesign Markup Language      |
| **Adobe FrameMaker MIF**  | `mif`     | `.mif`                         | FrameMaker Maker Interchange Format |
| **EPUB**                  | `epub`    | `.epub`                        | EPUB 2/3 e-book extraction          |
| **PDF**                   | `pdf`     | `.pdf`                         | PDF text extraction                 |

### Subtitle Formats

| Format     | ID     | Extensions       | Description                       |
| ---------- | ------ | ---------------- | --------------------------------- |
| **SRT**    | `srt`  | `.srt`           | SubRip subtitle format            |
| **WebVTT** | `vtt`  | `.vtt`           | Web Video Text Tracks             |
| **TTML**   | `ttml` | `.ttml`, `.dfxp` | Timed Text Markup Language / DFXP |

### Plain Text Variants

| Format                   | ID              | Extensions      | Description                                  |
| ------------------------ | --------------- | --------------- | -------------------------------------------- |
| **Plain Text**           | `plaintext`     | `.txt`, `.text` | Line or paragraph segmentation               |
| **Moses Text**           | `mosestext`     | `.txt`          | Moses MT plain text (one segment per line)   |
| **Paragraph Plain Text** | `paraplaintext` | `.txt`          | Text split by blank-line paragraphs          |
| **Spliced Lines**        | `splicedlines`  | `.txt`          | Backslash-continued lines merged into blocks |
| **Versified Text**       | `versifiedtext` | `.txt`, `.ver`  | Poetry/scripture with verse markers          |
| **R Vignette**           | `vignette`      | `.Rmd`, `.Rnw`  | R documentation vignettes                    |

### Container Formats

| Format          | ID        | Extensions | Description                                   |
| --------------- | --------- | ---------- | --------------------------------------------- |
| **ZIP Archive** | `archive` | `.zip`     | ZIP extraction with glob-based file filtering |

## Okapi Bridge Formats

With the [Okapi bridge plugin](/docs/kapi-cli/commands/plugins) installed, you get access to 40+ additional Java-based format filters from the Okapi Framework. This includes formats like DITA, Versified Text, and many others not covered by the native formats above.

## Format Detection

neokapi automatically detects formats using a cascade strategy:

1. Explicit MIME type (if provided)
2. File extension mapping
3. Magic bytes / content sniffing

You can override detection with the `--format` flag on any command.

## Listing Formats

```bash
kapi formats
```

Use `--mime` or `--ext` to filter:

```bash
kapi formats --mime text/html
kapi formats --ext .docx
```

## Interactive Format Reference

See the [Format Reference](/formats) page for interactive documentation of all formats with configurable parameters.
