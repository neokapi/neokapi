// Series 3, "The content engine": formats, tools, flows and stores, in the
// Mart sample.
//
// KapiMart is a storefront for small retailers: an interface catalog in
// nested JSON, an About page in Markdown, three locales at different stages,
// a glossary and a content memory. The labs run the engine's own verbs on it,
// ad hoc first and then inside a project it sets up itself.

import type { Lab } from "../types.ts";

const DOCS = {
  contentModel: { label: "The content model", href: "/framework/content-model" },
  inline: { label: "Inline formatting", href: "/framework/inline-formatting" },
  formats: { label: "Formats", href: "/framework/formats" },
  tools: { label: "Tools", href: "/framework/tools" },
  flows: { label: "Flows", href: "/framework/flows" },
  prompts: { label: "Prompts", href: "/framework/prompts" },
  checks: { label: "Checks", href: "/framework/checks" },
  memory: { label: "Content memory", href: "/framework/content-memory" },
  terms: { label: "Terminology", href: "/framework/terminology" },
  firstProject: { label: "Your first project", href: "/kapi/get-started/first-project" },
  pseudo: { label: "Preview before you ship", href: "/kapi/recipes/pseudo-translate" },
};

export const martFormats: Lab = {
  id: "mart-formats",
  series: "engine",
  position: 1,
  title: "Any format, one model",
  tagline: "A JSON catalog and a Markdown page, read into the same blocks.",
  summary:
    "The engine reads every format into one content model: blocks of runs, where inline codes, placeholders and plurals travel with the text. This lab inspects KapiMart's catalog and its About page, pseudo-translates the catalog without losing a placeholder, and re-expresses the page as HTML through the model.",
  sample: "mart",
  concepts: ["content model", "block", "run", "inline code", "reader and writer", "round trip"],
  docs: [DOCS.contentModel, DOCS.inline, DOCS.formats, DOCS.pseudo],
  minutes: 5,
  chapters: [
    {
      id: "tree",
      title: "A storefront's content",
      narration:
        "KapiMart's interface strings are a nested JSON catalog; its About page is Markdown. Three locales sit beside the source at different stages: French complete, German two thirds, Japanese one third.",
      command: "ls src",
      expect: ["en.json", "about.md"],
      look: { file: "src/en.json", view: "preview" },
    },
    {
      id: "stats",
      title: "Measure the catalog",
      narration: "Thirty-six strings, two hundred words. The numbers a quote starts from.",
      command: "kapi stats src/en.json",
      expect: ["Words:"],
    },
    {
      id: "inspect-json",
      title: "The catalog as blocks",
      narration:
        "Every string is a block named by its key, with a revision of its text. The nesting of the JSON is kept in the key path.",
      command: "kapi inspect src/en.json | head -30",
      expect: ['"nav.home"'],
      look: { file: "src/en.json", view: "blocks" },
    },
    {
      id: "inspect-md",
      title: "The page as blocks",
      narration:
        "Prose is addressed by structure: the heading, then the paragraph under it. A role says what the block is, so a heading is a heading in every format.",
      command: "kapi inspect src/about.md | head -34",
      expect: ['"about-kapimart/p"', '"role": "heading"'],
      look: { file: "src/about.md", view: "blocks" },
    },
    {
      id: "plural",
      title: "Inline structure travels with the text",
      narration:
        "An ICU plural or select is one block whose runs the model keeps intact, so a translation cannot drop a branch unnoticed. kgrep finds it as text, with the structure visible.",
      command: "kgrep plural src/en.json",
      expect: ["{count, plural"],
    },
    {
      id: "pseudo",
      title: "Pseudo-translate",
      narration:
        "Pseudo-translation accents every letter and leaves every placeholder and plural branch as it was: a cheap test of a layout before a real translation exists.",
      command: "kapi pseudo-translate src/en.json -o src/qps.json",
      expect: ["completed"],
    },
    {
      id: "placeholders",
      title: "Placeholders survive",
      narration:
        "The price placeholder and the plural's branches are untouched inside the accented text.",
      command: "grep -n 'price_each|summary' src/qps.json",
      expect: ["{price}", "plural"],
      look: { file: "src/qps.json", view: "raw" },
    },
    {
      id: "convert",
      title: "Re-express through the model",
      narration:
        "kconv reads one format and writes another. The Markdown heading becomes an h1 because both are the same block with the same role.",
      command: "kconv src/about.md -o src/about.html",
    },
    {
      id: "html",
      title: "The result",
      narration: "Headings, paragraphs and emphasis, carried across.",
      command: "head -12 src/about.html",
      expect: ["<h1>About KapiMart</h1>"],
      look: { file: "src/about.html", view: "preview" },
    },
    {
      id: "formats",
      title: "What the engine reads and writes",
      narration:
        "Each format has a reader and, where it can write back, a writer, with its edit fidelity named. A faithful writer changes only the bytes that carry the text that changed.",
      command: "kapi formats",
      expect: ["openxml", "faithful"],
    },
  ],
  tryNext: [
    "kconv src/about.md -o src/about.dclg.xml",
    "kapi inspect src/fr.json | head -20",
    "kapi stats src/about.md --json",
  ],
};

export const martTools: Lab = {
  id: "mart-tools",
  series: "engine",
  position: 2,
  title: "Tools, flows and the demo provider",
  tagline: "One tool at a time, then a flow, with every prompt in the open.",
  summary:
    "A tool is one processing step over blocks; a flow composes tools; a provider is what a translate step talks to. This lab lists both, runs the built-in translate flow on the About page with the demo provider, prints the prompts a model would receive, and runs a check as a single tool.",
  sample: "mart",
  concepts: ["tool", "flow", "source → flow → sink", "provider", "prompts", "checks as tools"],
  docs: [DOCS.tools, DOCS.flows, DOCS.prompts, DOCS.checks],
  minutes: 6,
  chapters: [
    {
      id: "tools",
      title: "The tool registry",
      narration:
        "A tool is one processing step: it reads blocks and writes blocks. The registry groups them by what they do: translation, quality, analysis, text processing.",
      command: "kapi tools",
      expect: ["recycle", "placeholder-check"],
    },
    {
      id: "flows",
      title: "Flows compose tools",
      narration:
        "A flow is a named composition. The built-in translate flow is recycle, then translate, then the deterministic checks: reuse before AI, and verify what AI produced.",
      command: "kapi flows",
      expect: ["translate", "secure-translate"],
    },
    {
      id: "explain",
      title: "Where content enters and leaves",
      narration:
        "A run binds a source and a sink. --explain prints the binding without running anything: a file in, a file out, since this is an ad-hoc run with no project.",
      command: "kapi run translate -i src/about.md -o src/about.fr.md --target-lang fr --explain",
      expect: ["file(src/about.md) → file(src/about.fr.md)"],
    },
    {
      id: "run",
      title: "Run the flow",
      narration:
        "In the browser every provider is the demo stub: deterministic, keyless, and marked. The flow ran for real; only the model is illustrative.",
      command: "kapi run translate -i src/about.md -o src/about.fr.md --target-lang fr",
      expect: ["completed"],
      notice: ["completed"],
    },
    {
      id: "result",
      title: "The marked output",
      narration: "Every sentence the stub wrote carries its marker, so nobody ships it by mistake.",
      command: "head -8 src/about.fr.md",
      expect: ["⟦fr⟧"],
      look: { file: "src/about.fr.md", view: "preview" },
    },
    {
      id: "prompts",
      title: "What reaches a model",
      narration:
        "exec runs one tool with nothing around it. --explain-prompts prints what the translate step sends: the task, the constraints, the block's place in the document, the text. Nothing is hidden.",
      command:
        "kapi exec translate src/about.md --target-lang de -o src/about.de.md --explain-prompts 2>&1 | head -36",
      expect: ["prompt:", "translate.single"],
      notice: ["constraint"],
    },
    {
      id: "check-tool",
      title: "Checks are tools too",
      narration:
        "The German catalog is missing a select; the placeholder check finds it. An exec reports and never fails a build; the gate that stops one is kapi check.",
      command: "kapi exec placeholder-check src/en.json --target src/de.json --target-lang de",
      expect: ["{provider, select}", "FAILS"],
    },
    {
      id: "json",
      title: "Every command speaks JSON",
      narration:
        "--json prints the structured result and --jq filters it in place, which is what scripts and agents read.",
      command: "kapi stats src/en.json --json --jq .total.words",
      expect: ["215"],
    },
  ],
  tryNext: [
    "kapi run secure-translate -i src/about.md -o src/about.secure.md --target-lang fr --explain",
    "kapi tools schema recycle",
    "kapi exec term-extract --help",
  ],
};

const FR_DRAFT = `{
  "products": {
    "title": "Produits",
    "add_to_cart": "Ajouter au chariot"
  },
  "account": {
    "title": "Compte"
  },
  "checkout": {
    "pay_with": "Payer"
  }
}
`;

const PROMO = `# Spring sale

Leverage our seamless checkout and the KapiMart Copilot to sell more this
spring. Every order ships from one dashboard.
`;

export const martProject: Lab = {
  id: "mart-project",
  series: "engine",
  position: 3,
  title: "A project remembers",
  tagline: "Set up a project, give it memory and terms, and hold a draft to them.",
  summary:
    "Ad hoc, nothing is kept between runs. A project keeps a content memory, a terms store and a record. This lab sets one up around KapiMart with kapi init, imports a TMX and the vocabulary, holds a returned draft to the terms, catches brand words in a new page, and runs the loop.",
  sample: "mart",
  concepts: [
    "kapi init",
    "kapi add",
    "content memory",
    "TMX",
    "terms store",
    "term-check",
    "kapi up",
  ],
  docs: [DOCS.firstProject, DOCS.memory, DOCS.terms, DOCS.checks],
  minutes: 7,
  chapters: [
    {
      id: "init",
      title: "Set up a project",
      narration:
        "kapi init writes a recipe for the content it recognises. It found the Markdown; the catalog it leaves to you, because a JSON file could be anything.",
      command:
        "kapi init --name kapimart --target-locale fr --target-locale de --target-locale ja --agents none --no-rules-files",
      expect: ["Initialized kapi project"],
      look: { file: "kapi.yaml", view: "raw" },
    },
    {
      id: "add",
      title: "Claim the catalog",
      narration:
        "kapi add claims the catalog and says where its translations go. The template names the locale, so fr.json, de.json and ja.json are its targets.",
      command: "kapi add src/en.json --target 'src/{lang}.json'",
      expect: ["Added src/en.json"],
      look: { file: "kapi.yaml", view: "raw" },
    },
    {
      id: "status",
      title: "Standing, derived from the files",
      narration:
        "kapi status derives each language's standing from what is on disk: French complete, German two thirds, Japanese one third. No gate is declared yet, so nothing is withheld.",
      command: "kapi status",
      expect: ["fr", "ja", "not gated"],
    },
    {
      id: "memory",
      title: "Import a content memory",
      narration:
        "A TMX from an earlier vendor is the interchange tier. Imported into the project's content memory, its pairs are reused before any provider is asked.",
      command: "kapi memory import memory.tmx -s en -t fr",
      expect: ["Imported 5 entries"],
    },
    {
      id: "terms",
      title: "Import the vocabulary",
      narration:
        "The terms arrive as a bundle: the product and payment names kept as written in every language, the shop words with their renderings, and the brand's forbidden words beside the word to use instead.",
      command: "kapi store import ./context",
      expect: ["14 concepts"],
    },
    {
      id: "context",
      title: "What a writer reads first",
      narration: "The brief for the catalog, from the terms just imported.",
      command: "kapi context src/en.json | head -10",
      expect: ['AI assistant, not "bot" or "Copilot"'],
    },
    {
      id: "draft",
      title: "Hold a draft to the terms",
      narration:
        "A French draft someone sent back. The terminology check holds it to the terms: chariot where the terms say panier, and two payment brands that went missing.",
      files: [{ path: "src/fr-draft.json", content: FR_DRAFT }],
      command: "kapi exec term-check src/en.json --target src/fr-draft.json --target-lang fr",
      expect: ['required translation "panier" missing', "PayPal"],
      look: { file: "src/fr-draft.json", view: "raw" },
    },
    {
      id: "placeholder",
      title: "And to its placeholders",
      narration: "The draft also flattened a select. The placeholder check finds that.",
      command:
        "kapi exec placeholder-check src/en.json --target src/fr-draft.json --target-lang fr",
      expect: ["{provider, select}"],
    },
    {
      id: "promo",
      title: "Prose is held to the same words",
      narration:
        "A new promo page uses three words the brand does not. The project's check fails on each and names the replacement.",
      files: [{ path: "src/promo.md", content: PROMO }],
      command: "kapi check",
      exit: 3,
      expect: ["Copilot", "leverage"],
      notice: ["FAILS"],
      look: { file: "src/promo.md", view: "preview" },
    },
    {
      id: "fix",
      title: "Correct the page",
      narration: "One edit takes the house words.",
      command:
        "ksed -i 's/Leverage our seamless checkout and the KapiMart Copilot/Use the KapiMart checkout and the AI assistant/' src/promo.md",
    },
    {
      id: "pass",
      title: "The check passes",
      narration: "The page passes the project's checks. Exit 0 is what a build sees.",
      command: "kapi check",
      exit: 0,
      expect: ["PASS"],
    },
    {
      id: "up",
      title: "Run the loop",
      narration:
        "kapi up drafts the missing German and Japanese with the demo stub, after recycling what the catalogs and the memory already hold. Japanese parks on a failing check, for a person to look at.",
      command: "kapi up",
      expect: ["Ran flow"],
    },
    {
      id: "after",
      title: "Every language caught up",
      narration: "Three languages at 100%, one of them waiting for a person.",
      command: "kapi status",
      expect: ["100%"],
    },
  ],
  tryNext: [
    "kapi status --review --lang ja | head -10",
    "kapi context search panier",
    "kapi memory search cart",
  ],
};

export const MART_LABS: Lab[] = [martFormats, martTools, martProject];
