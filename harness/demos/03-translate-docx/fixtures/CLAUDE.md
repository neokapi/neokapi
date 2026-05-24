# Lumen Notes — launch docs

Launch and marketing documents for Lumen Notes. To translate a document into
another language, use **kapi** — it reads and writes the document format directly,
so the structure, formatting, and any placeholders are preserved and only the text
changes:

    kapi ai-translate <file> --target-lang <lang> -o <name>.<lang>.docx

For Japanese, write the result to `announcement.ja.docx`:
`kapi ai-translate announcement.docx --target-lang ja -o announcement.ja.docx`.
