// A small corpus chosen to make the content model legible. Each sample's
// filename carries the extension the native WASM readers detect by. The set
// deliberately spans formats whose readers decompose content differently — so a
// learner sees, for instance, that an HTML <strong> becomes a paired inline
// code while a JSON "{name}" stays literal text (format-awareness in action).

export interface LabSample {
  id: string;
  label: string;
  filename: string;
  blurb: string;
  content: string;
}

export const SAMPLES: LabSample[] = [
  {
    id: "messages-json",
    label: "messages.json",
    filename: "messages.json",
    blurb: "Nested JSON — structure becomes layers and groups; values are literal text.",
    content: `{
  "greeting": "Hello, {name}!",
  "cart": {
    "empty": "Your cart is empty",
    "checkout": "Proceed to checkout"
  },
  "farewell": "See you tomorrow"
}
`,
  },
  {
    id: "page-html",
    label: "page.html",
    filename: "page.html",
    blurb: "HTML — inline tags (<strong>, <a>) become paired codes inside a block's runs.",
    content: `<!doctype html>
<html lang="en">
  <head>
    <meta charset="utf-8" />
    <title>Welcome</title>
  </head>
  <body>
    <h1>Welcome aboard</h1>
    <p>Thanks for trying <strong>kapi</strong>. Read the <a href="/docs">documentation</a>.</p>
  </body>
</html>
`,
  },
  {
    id: "app-properties",
    label: "app.properties",
    filename: "app.properties",
    blurb: "Java properties — comments become non-translatable Data between Blocks.",
    content: `# Application strings
# Shown on first launch
app.title = Welcome aboard
app.greeting = Hello, World!

# Shopping cart
cart.empty = Your cart is empty
cart.checkout = Proceed to checkout
`,
  },
  {
    id: "app-xliff",
    label: "app.xliff",
    filename: "app.xliff",
    blurb: "XLIFF 2.x — a bilingual exchange format with explicit source (and target) segments.",
    content: `<?xml version="1.0" encoding="UTF-8"?>
<xliff xmlns="urn:oasis:names:tc:xliff:document:2.2" version="2.2" srcLang="en" trgLang="fr">
  <file id="app.json" original="app.json">
    <unit id="greeting"><segment><source>Hello, World!</source></segment></unit>
    <unit id="farewell"><segment><source>See you tomorrow</source></segment></unit>
    <unit id="cart.empty"><segment><source>Your cart is empty</source></segment></unit>
  </file>
</xliff>
`,
  },
  {
    id: "article-md",
    label: "article.md",
    filename: "article.md",
    blurb:
      "Markdown — headings, a list, an inline-styled paragraph, and a fenced code block (with a language). A rich source for cross-format conversion.",
    content: `# Release notes

The **2.0** release adds format-aware conversion. See the [docs](/docs) for details.

## Highlights

- Convert between formats
- Preserve structure and inline styling
- Keep code blocks intact

\`\`\`go
fmt.Println("hello, world")
\`\`\`
`,
  },
  {
    id: "report-doclang",
    label: "report.dclg.xml",
    filename: "report.dclg.xml",
    blurb:
      "DocLang — a structured document with roles, a table, and geometry. Convert it to markdown/html to see the structure re-expressed.",
    content: `<?xml version="1.0" encoding="UTF-8"?>
<doclang xmlns="https://www.doclang.ai/ns/v0" version="0.6">
  <heading level="1">Quarterly Report</heading>
  <text>Revenue grew across <bold>every</bold> region this quarter.</text>
  <table>
    <ched/>Region<ched/>Revenue<nl/>
    <fcel/>North<fcel/>1200<nl/>
    <fcel/>South<fcel/>980<nl/>
  </table>
</doclang>
`,
  },
  {
    id: "support-reply",
    label: "support-reply.json",
    filename: "support-reply.json",
    blurb:
      "A support reply carrying a brand name (Acme Corp) and a person name (Jane Doe) — good targets for redaction — plus US spelling (color) that a search-replace normaliser can settle to British English before translation.",
    content: `{
  "subject": "Your Acme Corp order",
  "body": "Hi, we love the color of your order. Jane Doe will follow up tomorrow.",
  "footer": "Thanks for choosing Acme Corp"
}
`,
  },
];

export function sampleById(id: string): LabSample | undefined {
  return SAMPLES.find((s) => s.id === id);
}
