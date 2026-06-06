// Shared fixture library for the kapi playground kit.
//
// Each fixture is a small, self-contained sample input in one of the formats
// kapi understands. A `<RunnableSnippet seed={["messages.json"]} />` references
// fixtures by name; on modal open they are written into the in-memory volume
// under the session cwd (see KapiRuntime.ensureSeed).
//
// These live on the JS side (not go:embed in the wasm) so authors can add or
// tweak samples without rebuilding the binary. go:embed is a later
// optimization tracked in the epic.

export interface Fixture {
  /** File name written into the volume (also the lookup key). */
  name: string;
  /** UTF-8 file contents. */
  content: string;
}

const messagesJson = JSON.stringify(
  {
    greeting: "Hello, World!",
    farewell: "See you tomorrow",
    items: { cart: "Your cart is empty" },
  },
  null,
  2,
);

const appXliff = `<?xml version="1.0" encoding="UTF-8"?>
<xliff xmlns="urn:oasis:names:tc:xliff:document:2.2" version="2.2" srcLang="en" trgLang="fr">
  <file id="app.json" original="app.json">
    <unit id="greeting">
      <segment>
        <source>Hello, World!</source>
      </segment>
    </unit>
    <unit id="farewell">
      <segment>
        <source>See you tomorrow</source>
      </segment>
    </unit>
    <unit id="cart.empty">
      <segment>
        <source>Your cart is empty</source>
      </segment>
    </unit>
  </file>
</xliff>
`;

const pageHtml = `<!doctype html>
<html lang="en">
  <head>
    <meta charset="utf-8" />
    <title>Welcome</title>
  </head>
  <body>
    <h1>Welcome aboard</h1>
    <p>Thanks for trying <strong>kapi</strong>. Edit this file and run a command.</p>
    <a href="/docs">Read the documentation</a>
  </body>
</html>
`;

const readmeMd = `# Project Title

Thanks for trying **kapi**. This README is a sample Markdown document.

## Getting started

- Install the CLI
- Run \`kapi word-count README.md\`
- See the extracted segments

Read more in the [documentation](https://neokapi.github.io).
`;

const appProperties = `# Application strings
app.title = Welcome aboard
app.greeting = Hello, World!
app.farewell = See you tomorrow
cart.empty = Your cart is empty
`;

const stringsXml = `<?xml version="1.0" encoding="utf-8"?>
<resources>
  <string name="app_name">Welcome aboard</string>
  <string name="greeting">Hello, World!</string>
  <string name="farewell">See you tomorrow</string>
  <string name="cart_empty">Your cart is empty</string>
</resources>
`;

const localizableXcstrings = JSON.stringify(
  {
    sourceLanguage: "en",
    strings: {
      greeting: {
        localizations: {
          en: { stringUnit: { state: "translated", value: "Hello, World!" } },
        },
      },
      farewell: {
        localizations: {
          en: { stringUnit: { state: "translated", value: "See you tomorrow" } },
        },
      },
      "cart.empty": {
        localizations: {
          en: { stringUnit: { state: "translated", value: "Your cart is empty" } },
        },
      },
    },
    version: "1.0",
  },
  null,
  2,
);

const FIXTURES: Record<string, Fixture> = {
  "messages.json": { name: "messages.json", content: messagesJson },
  "app.xliff": { name: "app.xliff", content: appXliff },
  "page.html": { name: "page.html", content: pageHtml },
  "README.md": { name: "README.md", content: readmeMd },
  "app.properties": { name: "app.properties", content: appProperties },
  "strings.xml": { name: "strings.xml", content: stringsXml },
  "Localizable.xcstrings": { name: "Localizable.xcstrings", content: localizableXcstrings },
};

/** All fixture names, for documentation and tooling. */
export const fixtureNames = Object.keys(FIXTURES);

/** Look up a fixture by name. Returns undefined for unknown names. */
export function getFixture(name: string): Fixture | undefined {
  return FIXTURES[name];
}
