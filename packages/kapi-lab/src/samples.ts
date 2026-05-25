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
    blurb: "XLIFF — a bilingual exchange format with explicit source (and target) segments.",
    content: `<?xml version="1.0" encoding="UTF-8"?>
<xliff version="1.2" xmlns="urn:oasis:names:tc:xliff:document:1.2">
  <file original="app.json" source-language="en" target-language="fr" datatype="plaintext">
    <body>
      <trans-unit id="greeting"><source>Hello, World!</source></trans-unit>
      <trans-unit id="farewell"><source>See you tomorrow</source></trans-unit>
      <trans-unit id="cart.empty"><source>Your cart is empty</source></trans-unit>
    </body>
  </file>
</xliff>
`,
  },
];

export function sampleById(id: string): LabSample | undefined {
  return SAMPLES.find((s) => s.id === id);
}
