/**
 * Shared test fixtures for Storybook stories.
 *
 * Realistic localization data modelled after HTML-in-JSON content —
 * the most common scenario in the translation editor.
 */

import type { ReviewPoint } from "@neokapi/contract-types";
import type { Run } from "@neokapi/kapi-format";
import type { SpanInfo } from "@neokapi/ui-primitives";
import { codeSpans } from "../components/editor/editorText";
import type {
  ReviewContext,
  BlockInfo,
  ProjectInfo,
  MemoryMatchInfo,
  BlockTermMatch,
  CheckIssue,
  FileCheckResult,
  BlockNote,
  BlockHistoryEntry,
  AutomationRule,
  AutomationEvent,
  AutomationHistoryEntry,
  TranslationDashboardStats,
  LocaleTranslationStats,
  ComplianceBasis,
  RoleTemplate,
  ShipState,
} from "../types/api";

// ---------------------------------------------------------------------------
// Inline codes, as the runs a reader produces
// ---------------------------------------------------------------------------

/** A paired code: its opening and closing halves share an id. */
export function pair(
  id: string,
  type: string,
  open: string,
  close: string,
  equiv: string,
): [Run, Run] {
  return [
    { pcOpen: { id, type, data: open, equiv } },
    { pcClose: { id, type, data: close, equiv } },
  ];
}

/** Runs from text and codes in order: a string is a text run. */
export function runsOf(...parts: Array<string | Run>): Run[] {
  return parts.map((part) => (typeof part === "string" ? { text: part } : part));
}

/** The chip a single code run renders as, for stories of the chip primitives. */
export function chip(run: Run): SpanInfo {
  return codeSpans([run])[0];
}

/** Runs as a block payload carries them: the generated contract shape. */
type ServedRuns = NonNullable<BlockInfo["source_runs"]>;
const served = (runs: Run[]): ServedRuns => runs as ServedRuns;

export const [boldOpen, boldClose] = pair("1", "fmt:bold", "<b>", "</b>", "b");
export const [italicOpen, italicClose] = pair("2", "fmt:italic", "<i>", "</i>", "i");
export const [linkOpen, linkClose] = pair(
  "3",
  "link:hyperlink",
  '<a href="https://example.com">',
  "</a>",
  "a",
);
export const [codeOpen, codeClose] = pair("4", "fmt:code", "<code>", "</code>", "code");
export const lineBreak: Run = {
  ph: { id: "5", type: "struct:break", data: "<br/>", equiv: "br" },
};
export const imgTag: Run = {
  ph: { id: "6", type: "media:image", data: '<img src="logo.png"/>', equiv: "img" },
};

// A type no vocabulary pack defines. The registry derives its chip label from
// the type name, so this renders "widget>" / "/widget" rather than "?>" / "/?".
// The same pair exists in the kapi kit's fixtures: both kits import one
// registry, so the chip must read the same in both Storybooks.
export const [unknownOpen, unknownClose] = pair(
  "9",
  "custom:widget",
  "<widget>",
  "</widget>",
  "widget",
);
export const [underlineOpen, underlineClose] = pair("7", "fmt:underline", "<u>", "</u>", "u");
export const [strikeOpen, strikeClose] = pair("8", "fmt:strikethrough", "<s>", "</s>", "s");
export const [supOpen, supClose] = pair("9", "fmt:superscript", "<sup>", "</sup>", "sup");

// Markdown-style codes (semantic types, delimiter data)
export const [mdBoldOpen, mdBoldClose] = pair("1", "bold", "**", "**", "b");
export const [mdItalicOpen, mdItalicClose] = pair("2", "italic", "*", "*", "i");
export const [mdCodeOpen, mdCodeClose] = pair("3", "code", "`", "`", "code");
export const [mdLinkOpen, mdLinkClose] = pair("4", "link", "[", "](https://docs.example.com)", "a");

// ---------------------------------------------------------------------------
// Run samples
// ---------------------------------------------------------------------------

/** "Click <b>here</b> to continue" */
export const simpleBoldRuns = runsOf("Click ", boldOpen, "here", boldClose, " to continue");

/** "Visit <a>our website</a> for <i>more info</i>" */
export const linkAndItalicRuns = runsOf(
  "Visit ",
  linkOpen,
  "our website",
  linkClose,
  " for ",
  italicOpen,
  "more info",
  italicClose,
);

/** "Use <code>kapi init</code> to set up" */
export const codeInlineRuns = runsOf("Use ", codeOpen, "kapi init", codeClose, " to set up");

/** "First line<br/>Second line" */
export const lineBreakRuns = runsOf("First line", lineBreak, "Second line");

/** All tag types in one segment */
export const richRuns = runsOf(
  boldOpen,
  "Bold",
  boldClose,
  " and ",
  italicOpen,
  "italic",
  italicClose,
  " with ",
  linkOpen,
  "a link",
  linkClose,
  " plus ",
  codeOpen,
  "code",
  codeClose,
  " and ",
  lineBreak,
);

/** Markdown: "Click **here** to *learn more*" */
export const mdFormattingRuns = runsOf(
  "Click ",
  mdBoldOpen,
  "here",
  mdBoldClose,
  " to ",
  mdItalicOpen,
  "learn more",
  mdItalicClose,
);

/** Markdown: "Run `kapi init` and visit [docs](url)" */
export const mdRichRuns = runsOf(
  "Run ",
  mdCodeOpen,
  "kapi init",
  mdCodeClose,
  " and visit ",
  mdLinkOpen,
  "docs",
  mdLinkClose,
);

// Codes the navigation and inline-code blocks below carry.
const [code10Open, code10Close] = pair("10", "fmt:code", "<code>", "</code>", "code");
const [code11Open, code11Close] = pair("11", "fmt:code", "<code>", "</code>", "code");
const [code12Open, code12Close] = pair("12", "fmt:code", "<code>", "</code>", "code");
const [code13Open, code13Close] = pair("13", "fmt:code", "<code>", "</code>", "code");
const [a1Open, a1Close] = pair("1", "a", '<a href="/features">', "</a>", "a");
const [b2Open, b2Close] = pair("2", "b", "<b>", "</b>", "b");
const [b3Open, b3Close] = pair("3", "b", "<b>", "</b>", "b");
const [code4Open, code4Close] = pair("4", "code", "<code>", "</code>", "code");
const br5: Run = { ph: { id: "5", type: "br", data: "<br/>", equiv: "br" } };
const [a6Open, a6Close] = pair("6", "a", '<a href="/docs">', "</a>", "a");
const [i7Open, i7Close] = pair("7", "i", "<i>", "</i>", "i");
const [b8Open, b8Close] = pair("8", "b", "<b>", "</b>", "b");

// ---------------------------------------------------------------------------
// Block samples
// ---------------------------------------------------------------------------

export const sampleBlocks: BlockInfo[] = [
  {
    id: "blk-1",
    source: "Welcome to Neokapi",
    targets: {
      "fr-FR": "Bienvenue sur Neokapi",
      "de-DE": "Willkommen bei Neokapi",
    },
    translatable: true,
    properties: { state: "translated" },
  },
  {
    id: "blk-2",
    source: "Click here to continue",
    source_runs: served(simpleBoldRuns),
    targets: { "fr-FR": "Cliquez ici pour continuer", "de-DE": "" },
    targets_runs: {
      "fr-FR": served(runsOf("Cliquez ", boldOpen, "ici", boldClose, " pour continuer")),
    },
    translatable: true,
    has_inline_codes: true,
    properties: { state: "draft" },
  },
  {
    id: "blk-3",
    source: "Visit our website for more info",
    source_runs: served(linkAndItalicRuns),
    targets: { "fr-FR": "", "de-DE": "" },
    translatable: true,
    has_inline_codes: true,
    properties: {},
  },
  {
    id: "blk-4",
    source: "Use kapi init to set up",
    source_runs: served(codeInlineRuns),
    targets: {
      "fr-FR": "Utilisez kapi init pour configurer",
      "de-DE": "Verwenden Sie kapi init zum Einrichten",
    },
    targets_runs: {
      "fr-FR": served(runsOf("Utilisez ", codeOpen, "kapi init", codeClose, " pour configurer")),
      "de-DE": served(
        runsOf("Verwenden Sie ", codeOpen, "kapi init", codeClose, " zum Einrichten"),
      ),
    },
    translatable: true,
    has_inline_codes: true,
    properties: { state: "reviewed" },
  },
  {
    id: "blk-5",
    source: "Terms of Service",
    targets: {
      "fr-FR": "Conditions d'utilisation",
      "de-DE": "Nutzungsbedingungen",
    },
    translatable: true,
    properties: { state: "translated" },
  },
  {
    id: "blk-6",
    source: "First line\nSecond line",
    source_runs: served(lineBreakRuns),
    targets: { "fr-FR": "Première ligne\nDeuxième ligne", "de-DE": "" },
    targets_runs: {
      "fr-FR": served(runsOf("Première ligne", lineBreak, "Deuxième ligne")),
    },
    translatable: true,
    has_inline_codes: true,
    properties: { state: "draft" },
  },
];

// ---------------------------------------------------------------------------
// Project fixture
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// content memory Match samples
// ---------------------------------------------------------------------------

export const sampleMemoryMatches: MemoryMatchInfo[] = [
  {
    source: "Welcome to Neokapi",
    target: "Bienvenue sur Neokapi",
    score: 1.0,
    match_type: "exact",
  },
  {
    source: "Welcome to the application",
    target: "Bienvenue dans l'application",
    score: 0.87,
    match_type: "fuzzy",
  },
  {
    source: "Welcome back",
    target: "Bon retour",
    score: 0.65,
    match_type: "fuzzy",
  },
];

// ---------------------------------------------------------------------------
// Term match samples
// ---------------------------------------------------------------------------

export const sampleTermMatches: BlockTermMatch[] = [
  {
    source_term: "localization",
    target_terms: ["localisation", "adaptation"],
    domain: "i18n",
    status: "preferred",
    start: 0,
    end: 12,
  },
  {
    source_term: "content memory",
    target_terms: ["m\u00e9moire de traduction"],
    domain: "i18n",
    status: "approved",
    start: 20,
    end: 38,
  },
  {
    source_term: "term",
    target_terms: ["terme"],
    domain: "linguistics",
    status: "admitted",
    start: 44,
    end: 48,
  },
];

export const deprecatedTermMatch: BlockTermMatch = {
  source_term: "internationalization",
  target_terms: [],
  domain: "i18n",
  status: "deprecated",
  start: 0,
  end: 20,
};

// ---------------------------------------------------------------------------
// Finding samples
// ---------------------------------------------------------------------------

export const sampleCheckIssues: CheckIssue[] = [
  {
    type: "missing-tag",
    severity: "error",
    message: "Missing closing <b> tag in target",
  },
  {
    type: "terminology",
    severity: "warning",
    message: '"localization" should be translated as "localisation"',
  },
  {
    type: "whitespace",
    severity: "warning",
    message: "Leading whitespace in target differs from source",
  },
];

export const sampleFileCheckResults: FileCheckResult[] = [
  {
    blockId: "blk-1",
    issues: [
      {
        type: "do-not-translate",
        severity: "error",
        message: 'Do-not-translate term "Neokapi" is missing from the fr-FR target',
        position: { kind: "range", start: { run: 0, offset: 11 }, end: { run: 1 } },
        original_text: "Neokapi",
        suggestion: 'Keep "Neokapi" verbatim in the target',
      },
    ],
  },
  {
    blockId: "blk-2",
    issues: [
      {
        type: "missing-tag",
        severity: "error",
        message: "Missing closing <b> tag in target",
      },
      {
        type: "terminology",
        severity: "warning",
        message: '"localization" should be translated as "localisation"',
      },
    ],
  },
  {
    blockId: "blk-3",
    issues: [
      {
        type: "whitespace",
        severity: "warning",
        message: "Leading whitespace in target differs from source",
      },
    ],
  },
  {
    blockId: "blk-6",
    issues: [
      {
        type: "placeholder",
        severity: "error",
        message: "Missing placeholder {count} in target",
      },
      {
        type: "punctuation",
        severity: "error",
        message: 'Target ends with "." but source does not',
      },
    ],
  },
];

// ---------------------------------------------------------------------------
// Block note samples
// ---------------------------------------------------------------------------

export const sampleBlockNotes: BlockNote[] = [
  {
    id: "note-1",
    blockId: "blk-1",
    author: "translator@example.com",
    text: "This greeting is used on the landing page hero section.",
    createdAt: "2026-02-20T10:30:00Z",
  },
  {
    id: "note-2",
    blockId: "blk-1",
    author: "reviewer@example.com",
    text: "Consider using a more formal tone for the German translation.",
    createdAt: "2026-02-21T15:45:00Z",
  },
];

// ---------------------------------------------------------------------------
// Block history samples
// ---------------------------------------------------------------------------

export const sampleBlockHistory: BlockHistoryEntry[] = [
  {
    seq: 3,
    changeType: "target_modified",
    text: "Bienvenue sur Neokapi",
    codedText: "Bienvenue sur Neokapi",
    author: "translator@example.com",
    actorRole: "member",
    origin: "human",
    timestamp: "2026-02-22T14:20:00Z",
  },
  {
    seq: 2,
    changeType: "target_modified",
    text: "Bienvenue chez Neokapi",
    codedText: "Bienvenue chez Neokapi",
    author: "translate",
    origin: "ai",
    timestamp: "2026-02-21T09:00:00Z",
  },
  {
    seq: 1,
    changeType: "target_added",
    text: "Bienvenue \u00e0 Neokapi",
    codedText: "Bienvenue \u00e0 Neokapi",
    author: "pseudo-translate",
    origin: "mt",
    timestamp: "2026-02-20T08:00:00Z",
  },
];

// ---------------------------------------------------------------------------
// Project fixture
// ---------------------------------------------------------------------------

export const sampleProject: ProjectInfo = {
  id: "proj-demo-1",
  name: "Demo App",
  default_source_language: "en-US",
  target_languages: ["fr-FR", "de-DE", "ja-JP"],
  dashboard_visibility: "private",
  properties: { workflow_enabled: "true", workflow_mode: "review" },
  workspace_id: "ws-1",
  items: [
    {
      id: "itm-msg1",
      name: "messages.json",
      format: "json",
      type: "file",
      collection_id: "coll-default",
      size: 4200,
      block_count: 6,
      word_count: 42,
    },
    {
      id: "itm-xlf1",
      name: "ui-strings.xliff",
      format: "xliff",
      type: "file",
      collection_id: "coll-default",
      size: 8100,
      block_count: 24,
      word_count: 180,
    },
  ],
  collections: [
    {
      id: "coll-default",
      project_id: "proj-demo-1",
      name: "default",
      kind: "uploaded",
      item_label: "file",
      is_default: true,
      item_count: 2,
      created_at: "2025-11-01T10:00:00Z",
      updated_at: "2025-11-01T10:00:00Z",
    },
    {
      id: "coll-docs",
      project_id: "proj-demo-1",
      name: "Documentation",
      kind: "uploaded",
      item_label: "document",
      is_default: false,
      item_count: 0,
      created_at: "2026-01-15T10:00:00Z",
      updated_at: "2026-01-15T10:00:00Z",
    },
    {
      id: "coll-cms",
      project_id: "proj-demo-1",
      name: "WordPress",
      kind: "connected",
      item_label: "page",
      is_default: false,
      item_count: 0,
      connector_config: { type: "wordpress" },
      created_at: "2026-02-01T10:00:00Z",
      updated_at: "2026-02-01T10:00:00Z",
    },
  ],
  streams: [
    {
      name: "main",
      parent: "",
      base_cursor: 0,
      archived: false,
      visibility: "public",
      description: "",
      created_at: "2025-11-01T10:00:00Z",
      created_by: "alice",
    },
    {
      name: "feature/translations",
      parent: "main",
      base_cursor: 42,
      archived: false,
      visibility: "private",
      description: "New translations for v2.0",
      created_at: "2026-02-10T10:00:00Z",
      created_by: "bob",
    },
  ],
  created_at: "2025-11-01T10:00:00Z",
  modified_at: "2026-02-20T14:30:00Z",
};

// ---------------------------------------------------------------------------
// Navigation blocks — 12 blocks simulating a "Getting Started" guide
// ---------------------------------------------------------------------------

export const navigationBlocks: BlockInfo[] = [
  {
    id: "nav-1",
    source: "Getting Started with Neokapi",
    targets: {
      "fr-FR": "Premiers pas avec Neokapi",
      "de-DE": "Erste Schritte mit Neokapi",
    },
    translatable: true,
    properties: { state: "translated" },
  },
  {
    id: "nav-2",
    source:
      "Neokapi is an AI-native localization framework that makes it easy to translate your applications into multiple languages. It provides format-aware document parsing and channel-based concurrent processing.",
    targets: {
      "fr-FR":
        "Neokapi est un framework de localisation natif IA qui facilite la traduction de vos applications dans plusieurs langues. Il fournit une analyse de documents sensible au format et un traitement concurrent bas\u00e9 sur des canaux.",
      "de-DE": "",
    },
    translatable: true,
    properties: { state: "translated" },
  },
  {
    id: "nav-3",
    source: "Installation",
    targets: { "fr-FR": "Installation", "de-DE": "Installation" },
    translatable: true,
    properties: { state: "translated" },
  },
  {
    id: "nav-4",
    source:
      "Install the CLI tool using Go. Make sure you have Go 1.22 or later installed on your system before proceeding with the setup.",
    targets: {
      "fr-FR":
        "Installez l'outil CLI en utilisant Go. Assurez-vous d'avoir Go 1.22 ou une version ult\u00e9rieure install\u00e9e sur votre syst\u00e8me.",
      "de-DE": "",
    },
    translatable: true,
    properties: { state: "translated" },
  },
  {
    id: "nav-5",
    source:
      "Run the following command to verify your installation is working correctly and check the installed version.",
    targets: { "fr-FR": "", "de-DE": "" },
    translatable: true,
    properties: {},
  },
  {
    id: "nav-6",
    source: "Quick Start",
    targets: { "fr-FR": "D\u00e9marrage rapide", "de-DE": "Schnellstart" },
    translatable: true,
    properties: { state: "translated" },
  },
  {
    id: "nav-7",
    source:
      "Initialize a new project by running kapi init in your project directory. This creates a .kapi folder with default configuration files.",
    source_runs: served(
      runsOf(
        "Initialize a new project by running ",
        code10Open,
        "kapi init",
        code10Close,
        " in your project directory. This creates a ",
        code11Open,
        ".kapi",
        code11Close,
        " folder with default configuration files.",
      ),
    ),
    targets: {
      "fr-FR":
        "Initialisez un nouveau projet en ex\u00e9cutant kapi init dans votre r\u00e9pertoire de projet. Cela cr\u00e9e un dossier .kapi avec la configuration par d\u00e9faut.",
      "de-DE": "",
    },
    targets_runs: {
      "fr-FR": served(
        runsOf(
          "Initialisez un nouveau projet en ex\u00e9cutant ",
          code10Open,
          "kapi init",
          code10Close,
          " dans votre r\u00e9pertoire de projet. Cela cr\u00e9e un dossier ",
          code11Open,
          ".kapi",
          code11Close,
          " avec la configuration par d\u00e9faut.",
        ),
      ),
    },
    translatable: true,
    has_inline_codes: true,
    properties: { state: "draft" },
  },
  {
    id: "nav-8",
    source:
      "Add your source files to the project by placing them in the directory structure defined in your configuration. Supported formats include JSON, XLIFF, PO, and many more.",
    targets: { "fr-FR": "", "de-DE": "" },
    translatable: true,
    properties: {},
  },
  {
    id: "nav-9",
    source: "Configuration",
    targets: { "fr-FR": "Configuration", "de-DE": "Konfiguration" },
    translatable: true,
    properties: { state: "translated" },
  },
  {
    id: "nav-10",
    source:
      "The configuration file supports multiple target locales, custom flows, and provider settings. Edit config.yaml in the .kapi directory to customize your translation workflow.",
    source_runs: served(
      runsOf(
        "The configuration file supports multiple target locales, custom flows, and provider settings. Edit ",
        code12Open,
        "config.yaml",
        code12Close,
        " in the ",
        code13Open,
        ".kapi",
        code13Close,
        " directory to customize your translation workflow.",
      ),
    ),
    targets: {
      "fr-FR":
        "Le fichier de configuration prend en charge plusieurs locales cibles. Modifiez config.yaml dans le r\u00e9pertoire .kapi pour personnaliser votre workflow.",
      "de-DE": "",
    },
    targets_runs: {
      "fr-FR": served(
        runsOf(
          "Le fichier de configuration prend en charge plusieurs locales cibles. Modifiez ",
          code12Open,
          "config.yaml",
          code12Close,
          " dans le r\u00e9pertoire ",
          code13Open,
          ".kapi",
          code13Close,
          " pour personnaliser votre workflow.",
        ),
      ),
    },
    translatable: true,
    has_inline_codes: true,
    properties: { state: "draft" },
  },
  {
    id: "nav-11",
    source: "Translation Workflow",
    targets: {
      "fr-FR": "Flux de traduction",
      "de-DE": "\u00dcbersetzungs-Workflow",
    },
    translatable: true,
    properties: { state: "reviewed" },
  },
  {
    id: "nav-12",
    source:
      "Use the translate command to process your files through the configured flow. Review and approve translations in the visual editor before exporting the final output.",
    targets: {
      "fr-FR":
        "Utilisez la commande translate pour traiter vos fichiers via le flux configur\u00e9. V\u00e9rifiez et approuvez les traductions dans l'\u00e9diteur visuel avant d'exporter la sortie finale.",
      "de-DE": "",
    },
    translatable: true,
    properties: { state: "translated" },
  },
];

// ---------------------------------------------------------------------------
// Inline code pipeline blocks — blocks with dense mixed-content markup
// produced by code finder rules matching HTML tags in JSON values.
// ---------------------------------------------------------------------------

/**
 * Blocks simulating output from a JSON filter with codeFinderRules enabled.
 * Shows realistic mixed-content segments with multiple inline code categories:
 * paired formatting (b, i, a, code) and self-closing placeholders (br, img).
 */
export const inlineCodeBlocks: BlockInfo[] = [
  {
    id: "ic-1",
    source: "Click here to learn more about our features.",
    source_runs: served(
      runsOf(
        "Click ",
        a1Open,
        "here",
        a1Close,
        " to learn more about our ",
        b2Open,
        "features",
        b2Close,
        ".",
      ),
    ),
    targets: {
      "fr-FR": "Cliquez ici pour en savoir plus sur nos fonctionnalit\u00e9s.",
    },
    targets_runs: {
      "fr-FR": served(
        runsOf(
          "Cliquez ",
          a1Open,
          "ici",
          a1Close,
          " pour en savoir plus sur nos ",
          b2Open,
          "fonctionnalit\u00e9s",
          b2Close,
          ".",
        ),
      ),
    },
    translatable: true,
    has_inline_codes: true,
    properties: { state: "translated" },
  },
  {
    id: "ic-2",
    source: "Important: Run kapi init to get started.\nSee the docs for more.",
    source_runs: served(
      runsOf(
        b3Open,
        "Important:",
        b3Close,
        " Run ",
        code4Open,
        "kapi init",
        code4Close,
        " to get started.",
        br5,
        "See the ",
        a6Open,
        "docs",
        a6Close,
        " for more.",
      ),
    ),
    targets: {
      "fr-FR":
        "Important\u00a0: Ex\u00e9cutez kapi init pour commencer.\nConsultez la documentation pour plus de d\u00e9tails.",
    },
    targets_runs: {
      "fr-FR": served(
        runsOf(
          b3Open,
          "Important\u00a0:",
          b3Close,
          " Ex\u00e9cutez ",
          code4Open,
          "kapi init",
          code4Close,
          " pour commencer.",
          br5,
          "Consultez la ",
          a6Open,
          "documentation",
          a6Close,
          " pour plus de d\u00e9tails.",
        ),
      ),
    },
    translatable: true,
    has_inline_codes: true,
    properties: { state: "draft" },
  },
  {
    id: "ic-3",
    source: "Upload your file (max 10 MB) and click Submit.",
    source_runs: served(
      runsOf(
        "Upload your file (max ",
        i7Open,
        "10 MB",
        i7Close,
        ") and click ",
        b8Open,
        "Submit",
        b8Close,
        ".",
      ),
    ),
    targets: { "fr-FR": "" },
    translatable: true,
    has_inline_codes: true,
    properties: {},
  },
];

// ---------------------------------------------------------------------------
// Automation fixtures
// ---------------------------------------------------------------------------

export const sampleAutomationEvents: AutomationEvent[] = [
  { type: "file.uploaded", description: "A file is uploaded to the project" },
  { type: "file.updated", description: "A file is updated in the project" },
  {
    type: "translation.completed",
    description: "All blocks in a file are translated",
  },
  { type: "connector.sync", description: "A connector sync completes" },
  { type: "project.created", description: "A new project is created" },
];

export const sampleAutomationRules: AutomationRule[] = [
  {
    id: "rule-1",
    project_id: "proj-demo-1",
    name: "Auto-translate on upload",
    trigger: "file.uploaded",
    conditions: [{ Field: "file.format", Operator: "equals", Value: "json" }],
    actions: [
      {
        Type: "auto_translate",
        Config: { provider: "claude", target_locale: "fr" },
      },
    ],
    enabled: true,
    builtin: false,
    created_at: "2026-02-15T10:00:00Z",
    updated_at: "2026-02-20T14:30:00Z",
  },
  {
    id: "rule-2",
    project_id: "proj-demo-1",
    name: "Checks after translation",
    trigger: "translation.completed",
    conditions: [],
    actions: [{ Type: "run_flow", Config: { flow: "qa" } }],
    enabled: true,
    builtin: true,
    created_at: "2026-01-10T08:00:00Z",
    updated_at: "2026-01-10T08:00:00Z",
  },
  {
    id: "rule-3",
    project_id: "proj-demo-1",
    name: "Notify on sync",
    trigger: "connector.sync",
    conditions: [{ Field: "connector.type", Operator: "equals", Value: "git" }],
    actions: [
      { Type: "webhook", Config: { url: "https://hooks.example.com/notify" } },
      { Type: "notify", Config: { channel: "#localization" } },
    ],
    enabled: false,
    builtin: false,
    created_at: "2026-02-01T12:00:00Z",
    updated_at: "2026-03-01T09:15:00Z",
  },
  {
    id: "rule-4",
    project_id: "proj-demo-1",
    name: "Create review tasks after automations",
    trigger: "push.automations.completed",
    conditions: [],
    actions: [{ Type: "create_review_tasks", Config: { mode: "review" } }],
    enabled: true,
    builtin: true,
    created_at: "2026-03-25T10:00:00Z",
    updated_at: "2026-03-25T10:00:00Z",
  },
  {
    id: "rule-5",
    project_id: "proj-demo-1",
    name: "Source review gate",
    trigger: "push.automations.completed",
    conditions: [],
    actions: [{ Type: "create_source_review", Config: { reviewer: "user-1" } }],
    enabled: false,
    builtin: false,
    created_at: "2026-03-25T10:00:00Z",
    updated_at: "2026-03-25T10:00:00Z",
  },
];

export const sampleAutomationHistory: AutomationHistoryEntry[] = [
  {
    id: "exec-1",
    rule_id: "rule-1",
    project_id: "proj-demo-1",
    event_id: "evt-upload-123",
    status: "success",
    error: "",
    started_at: "2026-03-08T14:20:00Z",
    ended_at: "2026-03-08T14:20:05Z",
  },
  {
    id: "exec-2",
    rule_id: "rule-2",
    project_id: "proj-demo-1",
    event_id: "evt-translate-456",
    status: "success",
    error: "",
    started_at: "2026-03-08T14:20:06Z",
    ended_at: "2026-03-08T14:20:08Z",
  },
  {
    id: "exec-3",
    rule_id: "rule-3",
    project_id: "proj-demo-1",
    event_id: "evt-sync-789",
    status: "failed",
    error: "webhook returned 503: Service Unavailable",
    started_at: "2026-03-07T09:00:00Z",
    ended_at: "2026-03-07T09:00:02Z",
  },
  {
    id: "exec-4",
    rule_id: "rule-1",
    project_id: "proj-demo-1",
    event_id: "evt-upload-100",
    status: "skipped",
    error: "",
    started_at: "2026-03-06T16:45:00Z",
    ended_at: "2026-03-06T16:45:00Z",
  },
];

// ---------------------------------------------------------------------------
// Translation Dashboard Stats fixtures
// ---------------------------------------------------------------------------

/** Realistic dashboard stats for a project with 3 target locales and 2 collections. */
export const sampleDashboardStats: TranslationDashboardStats = {
  locale_stats: [
    {
      locale: "fr-FR",
      translated_blocks: 42,
      total_blocks: 50,
      translated_words: 3200,
      total_words: 3800,
      percentage: 84.2,
    },
    {
      locale: "de-DE",
      translated_blocks: 28,
      total_blocks: 50,
      translated_words: 2100,
      total_words: 3800,
      percentage: 55.3,
    },
    {
      locale: "ja-JP",
      translated_blocks: 12,
      total_blocks: 50,
      translated_words: 900,
      total_words: 3800,
      percentage: 23.7,
    },
  ],
  item_stats: [
    {
      item_name: "messages.json",
      item_id: "itm-msg1",
      format: "json",
      collection_id: "coll-default",
      block_count: 18,
      word_count: 1200,
      locales: [
        {
          locale: "fr-FR",
          translated_blocks: 18,
          total_blocks: 18,
          translated_words: 1200,
          total_words: 1200,
          percentage: 100,
        },
        {
          locale: "de-DE",
          translated_blocks: 14,
          total_blocks: 18,
          translated_words: 920,
          total_words: 1200,
          percentage: 76.7,
        },
        {
          locale: "ja-JP",
          translated_blocks: 6,
          total_blocks: 18,
          translated_words: 400,
          total_words: 1200,
          percentage: 33.3,
        },
      ],
    },
    {
      item_name: "ui-strings.xliff",
      item_id: "itm-xlf1",
      format: "xliff",
      collection_id: "coll-default",
      block_count: 24,
      word_count: 1800,
      locales: [
        {
          locale: "fr-FR",
          translated_blocks: 20,
          total_blocks: 24,
          translated_words: 1500,
          total_words: 1800,
          percentage: 83.3,
        },
        {
          locale: "de-DE",
          translated_blocks: 12,
          total_blocks: 24,
          translated_words: 900,
          total_words: 1800,
          percentage: 50.0,
        },
        {
          locale: "ja-JP",
          translated_blocks: 4,
          total_blocks: 24,
          translated_words: 300,
          total_words: 1800,
          percentage: 16.7,
        },
      ],
    },
    {
      item_name: "landing-page.html",
      item_id: "itm-lp1",
      format: "html",
      collection_id: "coll-web",
      block_count: 8,
      word_count: 800,
      locales: [
        {
          locale: "fr-FR",
          translated_blocks: 4,
          total_blocks: 8,
          translated_words: 500,
          total_words: 800,
          percentage: 62.5,
        },
        {
          locale: "de-DE",
          translated_blocks: 2,
          total_blocks: 8,
          translated_words: 280,
          total_words: 800,
          percentage: 35.0,
        },
        {
          locale: "ja-JP",
          translated_blocks: 2,
          total_blocks: 8,
          translated_words: 200,
          total_words: 800,
          percentage: 25.0,
        },
      ],
    },
  ],
  collection_stats: [
    {
      collection_id: "coll-default",
      collection_name: "Product app",
      channel: "acme/app",
      coordinates: { product: "acme", channel: "app" },
      item_count: 2,
      block_count: 42,
      word_count: 3000,
      locales: [
        {
          locale: "fr-FR",
          translated_blocks: 38,
          total_blocks: 42,
          translated_words: 2700,
          total_words: 3000,
          percentage: 90.0,
        },
        {
          locale: "de-DE",
          translated_blocks: 26,
          total_blocks: 42,
          translated_words: 1820,
          total_words: 3000,
          percentage: 60.7,
        },
        {
          locale: "ja-JP",
          translated_blocks: 10,
          total_blocks: 42,
          translated_words: 700,
          total_words: 3000,
          percentage: 23.3,
        },
      ],
    },
    {
      collection_id: "coll-web",
      collection_name: "Website",
      channel: "acme/web",
      coordinates: { product: "acme", channel: "web" },
      item_count: 1,
      block_count: 8,
      word_count: 800,
      locales: [
        {
          locale: "fr-FR",
          translated_blocks: 4,
          total_blocks: 8,
          translated_words: 500,
          total_words: 800,
          percentage: 62.5,
        },
        {
          locale: "de-DE",
          translated_blocks: 2,
          total_blocks: 8,
          translated_words: 280,
          total_words: 800,
          percentage: 35.0,
        },
        {
          locale: "ja-JP",
          translated_blocks: 2,
          total_blocks: 8,
          translated_words: 200,
          total_words: 800,
          percentage: 25.0,
        },
      ],
    },
  ],
  total_blocks: 55,
  translatable_blocks: 50,
  total_source_words: 3800,
};

/** Large project stats with 6 locales for stress-testing charts. */
export const largeDashboardStats: TranslationDashboardStats = {
  locale_stats: [
    {
      locale: "fr-FR",
      translated_blocks: 420,
      total_blocks: 500,
      translated_words: 32000,
      total_words: 38000,
      percentage: 84.2,
    },
    {
      locale: "de-DE",
      translated_blocks: 350,
      total_blocks: 500,
      translated_words: 26500,
      total_words: 38000,
      percentage: 69.7,
    },
    {
      locale: "ja-JP",
      translated_blocks: 200,
      total_blocks: 500,
      translated_words: 15200,
      total_words: 38000,
      percentage: 40.0,
    },
    {
      locale: "es-ES",
      translated_blocks: 480,
      total_blocks: 500,
      translated_words: 36500,
      total_words: 38000,
      percentage: 96.1,
    },
    {
      locale: "pt-BR",
      translated_blocks: 150,
      total_blocks: 500,
      translated_words: 11400,
      total_words: 38000,
      percentage: 30.0,
    },
    {
      locale: "zh-CN",
      translated_blocks: 90,
      total_blocks: 500,
      translated_words: 6800,
      total_words: 38000,
      percentage: 17.9,
    },
  ],
  item_stats: [
    {
      item_name: "messages.json",
      item_id: "i1",
      format: "json",
      collection_id: "c1",
      block_count: 120,
      word_count: 8200,
      locales: [
        {
          locale: "fr-FR",
          translated_blocks: 120,
          total_blocks: 120,
          translated_words: 8200,
          total_words: 8200,
          percentage: 100,
        },
        {
          locale: "de-DE",
          translated_blocks: 100,
          total_blocks: 120,
          translated_words: 6800,
          total_words: 8200,
          percentage: 82.9,
        },
        {
          locale: "ja-JP",
          translated_blocks: 60,
          total_blocks: 120,
          translated_words: 4100,
          total_words: 8200,
          percentage: 50.0,
        },
        {
          locale: "es-ES",
          translated_blocks: 118,
          total_blocks: 120,
          translated_words: 8100,
          total_words: 8200,
          percentage: 98.8,
        },
        {
          locale: "pt-BR",
          translated_blocks: 40,
          total_blocks: 120,
          translated_words: 2700,
          total_words: 8200,
          percentage: 32.9,
        },
        {
          locale: "zh-CN",
          translated_blocks: 20,
          total_blocks: 120,
          translated_words: 1400,
          total_words: 8200,
          percentage: 17.1,
        },
      ],
    },
    {
      item_name: "ui-components.xliff",
      item_id: "i2",
      format: "xliff",
      collection_id: "c1",
      block_count: 200,
      word_count: 15000,
      locales: [
        {
          locale: "fr-FR",
          translated_blocks: 180,
          total_blocks: 200,
          translated_words: 13500,
          total_words: 15000,
          percentage: 90.0,
        },
        {
          locale: "de-DE",
          translated_blocks: 140,
          total_blocks: 200,
          translated_words: 10500,
          total_words: 15000,
          percentage: 70.0,
        },
        {
          locale: "ja-JP",
          translated_blocks: 80,
          total_blocks: 200,
          translated_words: 6000,
          total_words: 15000,
          percentage: 40.0,
        },
        {
          locale: "es-ES",
          translated_blocks: 195,
          total_blocks: 200,
          translated_words: 14600,
          total_words: 15000,
          percentage: 97.3,
        },
        {
          locale: "pt-BR",
          translated_blocks: 60,
          total_blocks: 200,
          translated_words: 4500,
          total_words: 15000,
          percentage: 30.0,
        },
        {
          locale: "zh-CN",
          translated_blocks: 40,
          total_blocks: 200,
          translated_words: 3000,
          total_words: 15000,
          percentage: 20.0,
        },
      ],
    },
    {
      item_name: "help-center.md",
      item_id: "i3",
      format: "md",
      collection_id: "c2",
      block_count: 80,
      word_count: 6400,
      locales: [
        {
          locale: "fr-FR",
          translated_blocks: 60,
          total_blocks: 80,
          translated_words: 4800,
          total_words: 6400,
          percentage: 75.0,
        },
        {
          locale: "de-DE",
          translated_blocks: 50,
          total_blocks: 80,
          translated_words: 4000,
          total_words: 6400,
          percentage: 62.5,
        },
        {
          locale: "ja-JP",
          translated_blocks: 30,
          total_blocks: 80,
          translated_words: 2400,
          total_words: 6400,
          percentage: 37.5,
        },
        {
          locale: "es-ES",
          translated_blocks: 75,
          total_blocks: 80,
          translated_words: 6000,
          total_words: 6400,
          percentage: 93.8,
        },
        {
          locale: "pt-BR",
          translated_blocks: 25,
          total_blocks: 80,
          translated_words: 2000,
          total_words: 6400,
          percentage: 31.3,
        },
        {
          locale: "zh-CN",
          translated_blocks: 15,
          total_blocks: 80,
          translated_words: 1200,
          total_words: 6400,
          percentage: 18.8,
        },
      ],
    },
    {
      item_name: "emails.po",
      item_id: "i4",
      format: "po",
      collection_id: "c2",
      block_count: 100,
      word_count: 8400,
      locales: [
        {
          locale: "fr-FR",
          translated_blocks: 60,
          total_blocks: 100,
          translated_words: 5500,
          total_words: 8400,
          percentage: 65.5,
        },
        {
          locale: "de-DE",
          translated_blocks: 60,
          total_blocks: 100,
          translated_words: 5200,
          total_words: 8400,
          percentage: 61.9,
        },
        {
          locale: "ja-JP",
          translated_blocks: 30,
          total_blocks: 100,
          translated_words: 2700,
          total_words: 8400,
          percentage: 32.1,
        },
        {
          locale: "es-ES",
          translated_blocks: 92,
          total_blocks: 100,
          translated_words: 7800,
          total_words: 8400,
          percentage: 92.9,
        },
        {
          locale: "pt-BR",
          translated_blocks: 25,
          total_blocks: 100,
          translated_words: 2200,
          total_words: 8400,
          percentage: 26.2,
        },
        {
          locale: "zh-CN",
          translated_blocks: 15,
          total_blocks: 100,
          translated_words: 1200,
          total_words: 8400,
          percentage: 14.3,
        },
      ],
    },
  ],
  collection_stats: [
    {
      collection_id: "c1",
      collection_name: "App UI",
      item_count: 2,
      block_count: 320,
      word_count: 23200,
      locales: [
        {
          locale: "fr-FR",
          translated_blocks: 300,
          total_blocks: 320,
          translated_words: 21700,
          total_words: 23200,
          percentage: 93.5,
        },
        {
          locale: "de-DE",
          translated_blocks: 240,
          total_blocks: 320,
          translated_words: 17300,
          total_words: 23200,
          percentage: 74.6,
        },
        {
          locale: "ja-JP",
          translated_blocks: 140,
          total_blocks: 320,
          translated_words: 10100,
          total_words: 23200,
          percentage: 43.5,
        },
        {
          locale: "es-ES",
          translated_blocks: 313,
          total_blocks: 320,
          translated_words: 22700,
          total_words: 23200,
          percentage: 97.8,
        },
        {
          locale: "pt-BR",
          translated_blocks: 100,
          total_blocks: 320,
          translated_words: 7200,
          total_words: 23200,
          percentage: 31.0,
        },
        {
          locale: "zh-CN",
          translated_blocks: 60,
          total_blocks: 320,
          translated_words: 4400,
          total_words: 23200,
          percentage: 19.0,
        },
      ],
    },
    {
      collection_id: "c2",
      collection_name: "Content",
      item_count: 2,
      block_count: 180,
      word_count: 14800,
      locales: [
        {
          locale: "fr-FR",
          translated_blocks: 120,
          total_blocks: 180,
          translated_words: 10300,
          total_words: 14800,
          percentage: 69.6,
        },
        {
          locale: "de-DE",
          translated_blocks: 110,
          total_blocks: 180,
          translated_words: 9200,
          total_words: 14800,
          percentage: 62.2,
        },
        {
          locale: "ja-JP",
          translated_blocks: 60,
          total_blocks: 180,
          translated_words: 5100,
          total_words: 14800,
          percentage: 34.5,
        },
        {
          locale: "es-ES",
          translated_blocks: 167,
          total_blocks: 180,
          translated_words: 13800,
          total_words: 14800,
          percentage: 93.2,
        },
        {
          locale: "pt-BR",
          translated_blocks: 50,
          total_blocks: 180,
          translated_words: 4200,
          total_words: 14800,
          percentage: 28.4,
        },
        {
          locale: "zh-CN",
          translated_blocks: 30,
          total_blocks: 180,
          translated_words: 2400,
          total_words: 14800,
          percentage: 16.2,
        },
      ],
    },
  ],
  total_blocks: 520,
  translatable_blocks: 500,
  total_source_words: 38000,
};

// ---------------------------------------------------------------------------
// Ship-state dashboard fixture
// ---------------------------------------------------------------------------

/**
 * store.DeriveShipState, mirrored: pending when the scope is empty, coverage is
 * partial, or any check fails; translated while establishing is incomplete;
 * then established.
 */
function deriveShipState(
  translatedBlocks: number,
  totalBlocks: number,
  approvedBlocks: number,
  failingChecks: number,
): ShipState {
  if (totalBlocks === 0 || translatedBlocks < totalBlocks || failingChecks > 0) return "pending";
  return approvedBlocks < totalBlocks ? "translated" : "established";
}

/**
 * sampleDashboardStats with server-derived ship states stamped onto every
 * locale slice via deriveShipState. fr-FR is promoted to full coverage and full
 * approval (established), de-DE to full coverage without full approval
 * (translated), and ja-JP to full coverage with failing checks — pending on
 * the failing-checks arm rather than on coverage, so a story can express it.
 */
export const shipStateDashboardStats: TranslationDashboardStats = (() => {
  const promoteToFull = new Set(["fr-FR", "de-DE", "ja-JP"]);
  const approvedFraction: Record<string, number> = { "fr-FR": 1, "de-DE": 0.4, "ja-JP": 0.25 };
  const failingFraction: Record<string, number> = { "ja-JP": 0.1 };

  const stamp = (l: LocaleTranslationStats): LocaleTranslationStats => {
    const full = promoteToFull.has(l.locale);
    const translated = full ? l.total_blocks : l.translated_blocks;
    const approved = Math.round(l.total_blocks * (approvedFraction[l.locale] ?? 0));
    const failing = Math.round(translated * (failingFraction[l.locale] ?? 0));
    return {
      ...l,
      translated_blocks: translated,
      translated_words: full ? l.total_words : l.translated_words,
      percentage: full ? 100 : l.percentage,
      approved_blocks: approved,
      failing_checks: failing,
      ship_state: deriveShipState(translated, l.total_blocks, approved, failing),
    };
  };

  return {
    ...sampleDashboardStats,
    locale_stats: sampleDashboardStats.locale_stats.map(stamp),
    collection_stats: sampleDashboardStats.collection_stats.map((c) => ({
      ...c,
      locales: c.locales.map(stamp),
    })),
  };
})();

/**
 * Ship-state dashboard with the derived compliance fields: fr-FR governed by
 * terms and a voice profile (worker draft scoring has run), de-DE governed by
 * terms, and every other locale governed by neither, so its blocks are not
 * governed and it has a not-governed count instead of a rate. The newest server
 * shape behind the compliance rate chip in the ship-readiness band.
 */
export const complianceDashboardStats: TranslationDashboardStats = (() => {
  const basisFor: Record<string, ComplianceBasis> = {
    "fr-FR": "voice+checks+terms",
    "de-DE": "checks+terms",
  };
  const rateFor: Record<string, number> = { "fr-FR": 0.92, "de-DE": 1 };

  const stamp = (l: LocaleTranslationStats): LocaleTranslationStats => {
    if (l.translated_blocks <= 0) return l;
    const rate = rateFor[l.locale];
    if (rate === undefined) {
      // Neither terms nor a voice profile govern this language, so no translated
      // block has a verdict to rate.
      return { ...l, not_governed_blocks: l.translated_blocks, compliance_basis: "checks" };
    }
    return {
      ...l,
      compliant_blocks: Math.round(l.translated_blocks * rate),
      compliance_rate: rate,
      compliance_basis: basisFor[l.locale],
    };
  };

  return {
    ...shipStateDashboardStats,
    locale_stats: shipStateDashboardStats.locale_stats.map(stamp),
    collection_stats: shipStateDashboardStats.collection_stats.map((c) => ({
      ...c,
      locales: c.locales.map(stamp),
    })),
  };
})();

// ---------------------------------------------------------------------------
// Role Templates
// ---------------------------------------------------------------------------

export const sampleRoleTemplates: RoleTemplate[] = [
  {
    id: "role-1",
    workspace_id: "ws-1",
    name: "translator",
    display_name: "Translator",
    description: "Can view content and submit translations",
    permissions: 0,
    permission_names: ["view_content", "translate"],
    is_builtin: true,
    position: 1,
    created_at: "2026-01-01T10:00:00Z",
    updated_at: "2026-01-01T10:00:00Z",
  },
  {
    id: "role-2",
    workspace_id: "ws-1",
    name: "reviewer",
    display_name: "Reviewer",
    description: "Can review and approve translations",
    permissions: 0,
    permission_names: ["view_content", "translate", "review", "manage_terms"],
    is_builtin: true,
    position: 2,
    created_at: "2026-01-01T10:00:00Z",
    updated_at: "2026-01-01T10:00:00Z",
  },
  {
    id: "role-3",
    workspace_id: "ws-1",
    name: "project-manager",
    display_name: "Project Manager",
    description: "Full project management access",
    permissions: 0,
    permission_names: [
      "view_content",
      "edit_source",
      "translate",
      "review",
      "manage_terms",
      "manage_tm",
      "run_flows",
      "manage_files",
      "manage_streams",
      "manage_members",
      "manage_project",
    ],
    is_builtin: false,
    position: 3,
    created_at: "2026-02-15T10:00:00Z",
    updated_at: "2026-03-01T10:00:00Z",
  },
];

/**
 * The point a review surface shows a governed unit at: the collection it sits
 * in, the voice in force with the guidance the producer was given, and the
 * vocabulary the check enforces.
 */
export const sampleReviewPoint: ReviewPoint = {
  path: "auth.json",
  language: "fr-FR",
  default: false,
  collection: "Product UI",
  coordinates: { product: "kapi", channel: "app" },
  voice: {
    name: "Bowrain Voice",
    source: "store:vp-1",
    // As core/profile.RenderVoiceGuideCompact prints it: the parenthesised tone
    // and style parts, then the tone guidance, then the swaps quoted and sorted.
    guide:
      "Voice profile (personality: precise, plain; formality: neutral; use active voice). " +
      "Tone guidance: say what the product does for the reader, in their words. " +
      'Never use these terms (use the replacement): "leverage" → "use"; "utilize" → "use".',
  },
  term_rules: [
    { term: "leverage", replacement: "use" },
    { term: "utilize", replacement: "use", advisory: true },
    { term: "best-in-class", note: "Rephrase without a superlative." },
    { term: "Bowrain", note: "The product name stays in English.", do_not_translate: true },
  ],
  terms_total: 4,
};

/** The review context for a unit nothing has resolved anything for. */
export function emptyReviewContext(
  blockId: string,
  itemName: string,
  locale: string,
): ReviewContext {
  return {
    block_id: blockId,
    item_name: itemName,
    locale,
    collection_id: "",
    terms: [],
    notes: [],
    point: { default: false, terms_total: 0 },
    neighbourhood: { key: blockId, window: 2 },
    history: {},
    judgement: {},
    provenance: {},
  };
}
