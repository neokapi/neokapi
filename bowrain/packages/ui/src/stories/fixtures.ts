/**
 * Shared test fixtures for Storybook stories.
 *
 * Realistic localization data modelled after HTML-in-JSON content —
 * the most common scenario in the translation editor.
 */

import type {
  SpanInfo,
  BlockInfo,
  ProjectInfo,
  TMMatchInfo,
  BlockTermMatch,
  QAIssue,
  FileQAResult,
  BlockNote,
  BlockHistoryEntry,
  AutomationRule,
  AutomationEvent,
  AutomationHistoryEntry,
  TranslationDashboardStats,
  RoleTemplate,
} from "../types/api";

// ---------------------------------------------------------------------------
// Spans (inline markup tags)
// ---------------------------------------------------------------------------

export const boldOpen: SpanInfo = {
  span_type: "opening",
  type: "fmt:bold",
  id: "1",
  data: "<b>",
};
export const boldClose: SpanInfo = {
  span_type: "closing",
  type: "fmt:bold",
  id: "1",
  data: "</b>",
};
export const italicOpen: SpanInfo = {
  span_type: "opening",
  type: "fmt:italic",
  id: "2",
  data: "<i>",
};
export const italicClose: SpanInfo = {
  span_type: "closing",
  type: "fmt:italic",
  id: "2",
  data: "</i>",
};
export const linkOpen: SpanInfo = {
  span_type: "opening",
  type: "link:hyperlink",
  id: "3",
  data: '<a href="https://example.com">',
};
export const linkClose: SpanInfo = {
  span_type: "closing",
  type: "link:hyperlink",
  id: "3",
  data: "</a>",
};
export const codeOpen: SpanInfo = {
  span_type: "opening",
  type: "fmt:code",
  id: "4",
  data: "<code>",
};
export const codeClose: SpanInfo = {
  span_type: "closing",
  type: "fmt:code",
  id: "4",
  data: "</code>",
};
export const lineBreak: SpanInfo = {
  span_type: "placeholder",
  type: "struct:break",
  id: "5",
  data: "<br/>",
};
export const imgTag: SpanInfo = {
  span_type: "placeholder",
  type: "media:image",
  id: "6",
  data: '<img src="logo.png"/>',
};
export const underlineOpen: SpanInfo = {
  span_type: "opening",
  type: "fmt:underline",
  id: "7",
  data: "<u>",
};
export const underlineClose: SpanInfo = {
  span_type: "closing",
  type: "fmt:underline",
  id: "7",
  data: "</u>",
};
export const strikeOpen: SpanInfo = {
  span_type: "opening",
  type: "fmt:strikethrough",
  id: "8",
  data: "<s>",
};
export const strikeClose: SpanInfo = {
  span_type: "closing",
  type: "fmt:strikethrough",
  id: "8",
  data: "</s>",
};
export const supOpen: SpanInfo = {
  span_type: "opening",
  type: "fmt:superscript",
  id: "9",
  data: "<sup>",
};
export const supClose: SpanInfo = {
  span_type: "closing",
  type: "fmt:superscript",
  id: "9",
  data: "</sup>",
};

// Markdown-style spans (semantic types, delimiter data)
export const mdBoldOpen: SpanInfo = {
  span_type: "opening",
  type: "bold",
  id: "1",
  data: "**",
};
export const mdBoldClose: SpanInfo = {
  span_type: "closing",
  type: "bold",
  id: "1",
  data: "**",
};
export const mdItalicOpen: SpanInfo = {
  span_type: "opening",
  type: "italic",
  id: "2",
  data: "*",
};
export const mdItalicClose: SpanInfo = {
  span_type: "closing",
  type: "italic",
  id: "2",
  data: "*",
};
export const mdCodeOpen: SpanInfo = {
  span_type: "opening",
  type: "code",
  id: "3",
  data: "`",
};
export const mdCodeClose: SpanInfo = {
  span_type: "closing",
  type: "code",
  id: "3",
  data: "`",
};
export const mdLinkOpen: SpanInfo = {
  span_type: "opening",
  type: "link",
  id: "4",
  data: "[",
};
export const mdLinkClose: SpanInfo = {
  span_type: "closing",
  type: "link",
  id: "4",
  data: "](https://docs.example.com)",
};

// Unicode markers used in coded text
const O = "\uE001"; // opening
const C = "\uE002"; // closing
const P = "\uE003"; // placeholder

// ---------------------------------------------------------------------------
// Coded text samples
// ---------------------------------------------------------------------------

/** "Click <b>here</b> to continue" */
export const simpleBoldCodedText = `Click ${O}here${C} to continue`;
export const simpleBoldSpans: SpanInfo[] = [boldOpen, boldClose];

/** "Visit <a>our website</a> for <i>more info</i>" */
export const linkAndItalicCodedText = `Visit ${O}our website${C} for ${O}more info${C}`;
export const linkAndItalicSpans: SpanInfo[] = [
  linkOpen,
  linkClose,
  italicOpen,
  italicClose,
];

/** "Use <code>kapi init</code> to set up" */
export const codeInlineCodedText = `Use ${O}kapi init${C} to set up`;
export const codeInlineSpans: SpanInfo[] = [codeOpen, codeClose];

/** "First line<br/>Second line" */
export const lineBreakCodedText = `First line${P}Second line`;
export const lineBreakSpans: SpanInfo[] = [lineBreak];

/** All tag types in one segment */
export const richCodedText = `${O}Bold${C} and ${O}italic${C} with ${O}a link${C} plus ${O}code${C} and ${P}`;
export const richSpans: SpanInfo[] = [
  boldOpen,
  boldClose,
  italicOpen,
  italicClose,
  linkOpen,
  linkClose,
  codeOpen,
  codeClose,
  lineBreak,
];

/** Markdown: "Click **here** to *learn more*" */
export const mdFormattingCodedText = `Click ${O}here${C} to ${O}learn more${C}`;
export const mdFormattingSpans: SpanInfo[] = [
  mdBoldOpen,
  mdBoldClose,
  mdItalicOpen,
  mdItalicClose,
];

/** Markdown: "Run `kapi init` and visit [docs](url)" */
export const mdRichCodedText = `Run ${O}kapi init${C} and visit ${O}docs${C}`;
export const mdRichSpans: SpanInfo[] = [
  mdCodeOpen,
  mdCodeClose,
  mdLinkOpen,
  mdLinkClose,
];

// ---------------------------------------------------------------------------
// Block samples
// ---------------------------------------------------------------------------

export const sampleBlocks: BlockInfo[] = [
  {
    id: "blk-1",
    source: "Welcome to Neokapi",
    source_coded: "Welcome to Neokapi",
    source_spans: [],
    targets: {
      "fr-FR": "Bienvenue sur Neokapi",
      "de-DE": "Willkommen bei Neokapi",
    },
    targets_coded: {
      "fr-FR": "Bienvenue sur Neokapi",
      "de-DE": "Willkommen bei Neokapi",
    },
    translatable: true,
    has_spans: false,
    properties: { state: "translated" },
  },
  {
    id: "blk-2",
    source: "Click here to continue",
    source_coded: simpleBoldCodedText,
    source_spans: simpleBoldSpans,
    targets: { "fr-FR": `Cliquez ${O}ici${C} pour continuer`, "de-DE": "" },
    targets_coded: {
      "fr-FR": `Cliquez ${O}ici${C} pour continuer`,
      "de-DE": "",
    },
    translatable: true,
    has_spans: true,
    properties: { state: "draft" },
  },
  {
    id: "blk-3",
    source: "Visit our website for more info",
    source_coded: linkAndItalicCodedText,
    source_spans: linkAndItalicSpans,
    targets: { "fr-FR": "", "de-DE": "" },
    targets_coded: { "fr-FR": "", "de-DE": "" },
    translatable: true,
    has_spans: true,
    properties: {},
  },
  {
    id: "blk-4",
    source: "Use kapi init to set up",
    source_coded: codeInlineCodedText,
    source_spans: codeInlineSpans,
    targets: {
      "fr-FR": `Utilisez ${O}kapi init${C} pour configurer`,
      "de-DE": `Verwenden Sie ${O}kapi init${C} zum Einrichten`,
    },
    targets_coded: {
      "fr-FR": `Utilisez ${O}kapi init${C} pour configurer`,
      "de-DE": `Verwenden Sie ${O}kapi init${C} zum Einrichten`,
    },
    translatable: true,
    has_spans: true,
    properties: { state: "reviewed" },
  },
  {
    id: "blk-5",
    source: "Terms of Service",
    source_coded: "Terms of Service",
    source_spans: [],
    targets: {
      "fr-FR": "Conditions d'utilisation",
      "de-DE": "Nutzungsbedingungen",
    },
    targets_coded: {
      "fr-FR": "Conditions d'utilisation",
      "de-DE": "Nutzungsbedingungen",
    },
    translatable: true,
    has_spans: false,
    properties: { state: "translated" },
  },
  {
    id: "blk-6",
    source: "First line\nSecond line",
    source_coded: lineBreakCodedText,
    source_spans: lineBreakSpans,
    targets: { "fr-FR": `Première ligne${P}Deuxième ligne`, "de-DE": "" },
    targets_coded: { "fr-FR": `Première ligne${P}Deuxième ligne`, "de-DE": "" },
    translatable: true,
    has_spans: true,
    properties: { state: "draft" },
  },
];

// ---------------------------------------------------------------------------
// Project fixture
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// TM Match samples
// ---------------------------------------------------------------------------

export const sampleTMMatches: TMMatchInfo[] = [
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
    source_term: "translation memory",
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
// QA Issue samples
// ---------------------------------------------------------------------------

export const sampleQAIssues: QAIssue[] = [
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

export const sampleFileQAResults: FileQAResult[] = [
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
    author: "ai-translate",
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
    source_coded: "Getting Started with Neokapi",
    source_spans: [],
    targets: {
      "fr-FR": "Premiers pas avec Neokapi",
      "de-DE": "Erste Schritte mit Neokapi",
    },
    targets_coded: {
      "fr-FR": "Premiers pas avec Neokapi",
      "de-DE": "Erste Schritte mit Neokapi",
    },
    translatable: true,
    has_spans: false,
    properties: { state: "translated" },
  },
  {
    id: "nav-2",
    source:
      "Neokapi is an AI-native localization framework that makes it easy to translate your applications into multiple languages. It provides format-aware document parsing and channel-based concurrent processing.",
    source_coded:
      "Neokapi is an AI-native localization framework that makes it easy to translate your applications into multiple languages. It provides format-aware document parsing and channel-based concurrent processing.",
    source_spans: [],
    targets: {
      "fr-FR":
        "Neokapi est un framework de localisation natif IA qui facilite la traduction de vos applications dans plusieurs langues. Il fournit une analyse de documents sensible au format et un traitement concurrent bas\u00e9 sur des canaux.",
      "de-DE": "",
    },
    targets_coded: {
      "fr-FR":
        "Neokapi est un framework de localisation natif IA qui facilite la traduction de vos applications dans plusieurs langues. Il fournit une analyse de documents sensible au format et un traitement concurrent bas\u00e9 sur des canaux.",
      "de-DE": "",
    },
    translatable: true,
    has_spans: false,
    properties: { state: "translated" },
  },
  {
    id: "nav-3",
    source: "Installation",
    source_coded: "Installation",
    source_spans: [],
    targets: { "fr-FR": "Installation", "de-DE": "Installation" },
    targets_coded: { "fr-FR": "Installation", "de-DE": "Installation" },
    translatable: true,
    has_spans: false,
    properties: { state: "translated" },
  },
  {
    id: "nav-4",
    source:
      "Install the CLI tool using Go. Make sure you have Go 1.22 or later installed on your system before proceeding with the setup.",
    source_coded:
      "Install the CLI tool using Go. Make sure you have Go 1.22 or later installed on your system before proceeding with the setup.",
    source_spans: [],
    targets: {
      "fr-FR":
        "Installez l'outil CLI en utilisant Go. Assurez-vous d'avoir Go 1.22 ou une version ult\u00e9rieure install\u00e9e sur votre syst\u00e8me.",
      "de-DE": "",
    },
    targets_coded: {
      "fr-FR":
        "Installez l'outil CLI en utilisant Go. Assurez-vous d'avoir Go 1.22 ou une version ult\u00e9rieure install\u00e9e sur votre syst\u00e8me.",
      "de-DE": "",
    },
    translatable: true,
    has_spans: false,
    properties: { state: "translated" },
  },
  {
    id: "nav-5",
    source:
      "Run the following command to verify your installation is working correctly and check the installed version.",
    source_coded:
      "Run the following command to verify your installation is working correctly and check the installed version.",
    source_spans: [],
    targets: { "fr-FR": "", "de-DE": "" },
    targets_coded: { "fr-FR": "", "de-DE": "" },
    translatable: true,
    has_spans: false,
    properties: {},
  },
  {
    id: "nav-6",
    source: "Quick Start",
    source_coded: "Quick Start",
    source_spans: [],
    targets: { "fr-FR": "D\u00e9marrage rapide", "de-DE": "Schnellstart" },
    targets_coded: {
      "fr-FR": "D\u00e9marrage rapide",
      "de-DE": "Schnellstart",
    },
    translatable: true,
    has_spans: false,
    properties: { state: "translated" },
  },
  {
    id: "nav-7",
    source:
      "Initialize a new project by running kapi init in your project directory. This creates a .kapi folder with default configuration files.",
    source_coded: `Initialize a new project by running ${O}kapi init${C} in your project directory. This creates a ${O}.kapi${C} folder with default configuration files.`,
    source_spans: [
      { span_type: "opening", type: "fmt:code", id: "10", data: "<code>" },
      { span_type: "closing", type: "fmt:code", id: "10", data: "</code>" },
      { span_type: "opening", type: "fmt:code", id: "11", data: "<code>" },
      { span_type: "closing", type: "fmt:code", id: "11", data: "</code>" },
    ],
    targets: {
      "fr-FR": `Initialisez un nouveau projet en ex\u00e9cutant ${O}kapi init${C} dans votre r\u00e9pertoire de projet. Cela cr\u00e9e un dossier ${O}.kapi${C} avec la configuration par d\u00e9faut.`,
      "de-DE": "",
    },
    targets_coded: {
      "fr-FR": `Initialisez un nouveau projet en ex\u00e9cutant ${O}kapi init${C} dans votre r\u00e9pertoire de projet. Cela cr\u00e9e un dossier ${O}.kapi${C} avec la configuration par d\u00e9faut.`,
      "de-DE": "",
    },
    translatable: true,
    has_spans: true,
    properties: { state: "draft" },
  },
  {
    id: "nav-8",
    source:
      "Add your source files to the project by placing them in the directory structure defined in your configuration. Supported formats include JSON, XLIFF, PO, and many more.",
    source_coded:
      "Add your source files to the project by placing them in the directory structure defined in your configuration. Supported formats include JSON, XLIFF, PO, and many more.",
    source_spans: [],
    targets: { "fr-FR": "", "de-DE": "" },
    targets_coded: { "fr-FR": "", "de-DE": "" },
    translatable: true,
    has_spans: false,
    properties: {},
  },
  {
    id: "nav-9",
    source: "Configuration",
    source_coded: "Configuration",
    source_spans: [],
    targets: { "fr-FR": "Configuration", "de-DE": "Konfiguration" },
    targets_coded: { "fr-FR": "Configuration", "de-DE": "Konfiguration" },
    translatable: true,
    has_spans: false,
    properties: { state: "translated" },
  },
  {
    id: "nav-10",
    source:
      "The configuration file supports multiple target locales, custom flows, and provider settings. Edit config.yaml in the .kapi directory to customize your translation workflow.",
    source_coded: `The configuration file supports multiple target locales, custom flows, and provider settings. Edit ${O}config.yaml${C} in the ${O}.kapi${C} directory to customize your translation workflow.`,
    source_spans: [
      { span_type: "opening", type: "fmt:code", id: "12", data: "<code>" },
      { span_type: "closing", type: "fmt:code", id: "12", data: "</code>" },
      { span_type: "opening", type: "fmt:code", id: "13", data: "<code>" },
      { span_type: "closing", type: "fmt:code", id: "13", data: "</code>" },
    ],
    targets: {
      "fr-FR": `Le fichier de configuration prend en charge plusieurs locales cibles. Modifiez ${O}config.yaml${C} dans le r\u00e9pertoire ${O}.kapi${C} pour personnaliser votre workflow.`,
      "de-DE": "",
    },
    targets_coded: {
      "fr-FR": `Le fichier de configuration prend en charge plusieurs locales cibles. Modifiez ${O}config.yaml${C} dans le r\u00e9pertoire ${O}.kapi${C} pour personnaliser votre workflow.`,
      "de-DE": "",
    },
    translatable: true,
    has_spans: true,
    properties: { state: "draft" },
  },
  {
    id: "nav-11",
    source: "Translation Workflow",
    source_coded: "Translation Workflow",
    source_spans: [],
    targets: {
      "fr-FR": "Flux de traduction",
      "de-DE": "\u00dcbersetzungs-Workflow",
    },
    targets_coded: {
      "fr-FR": "Flux de traduction",
      "de-DE": "\u00dcbersetzungs-Workflow",
    },
    translatable: true,
    has_spans: false,
    properties: { state: "reviewed" },
  },
  {
    id: "nav-12",
    source:
      "Use the translate command to process your files through the configured flow. Review and approve translations in the visual editor before exporting the final output.",
    source_coded:
      "Use the translate command to process your files through the configured flow. Review and approve translations in the visual editor before exporting the final output.",
    source_spans: [],
    targets: {
      "fr-FR":
        "Utilisez la commande translate pour traiter vos fichiers via le flux configur\u00e9. V\u00e9rifiez et approuvez les traductions dans l'\u00e9diteur visuel avant d'exporter la sortie finale.",
      "de-DE": "",
    },
    targets_coded: {
      "fr-FR":
        "Utilisez la commande translate pour traiter vos fichiers via le flux configur\u00e9. V\u00e9rifiez et approuvez les traductions dans l'\u00e9diteur visuel avant d'exporter la sortie finale.",
      "de-DE": "",
    },
    translatable: true,
    has_spans: false,
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
    source_coded: `Click ${O}here${C} to learn more about our ${O}features${C}.`,
    source_spans: [
      {
        span_type: "opening",
        type: "a",
        id: "1",
        data: '<a href="/features">',
      },
      { span_type: "closing", type: "a", id: "1", data: "</a>" },
      { span_type: "opening", type: "b", id: "2", data: "<b>" },
      { span_type: "closing", type: "b", id: "2", data: "</b>" },
    ],
    targets: {
      "fr-FR": `Cliquez ${O}ici${C} pour en savoir plus sur nos ${O}fonctionnalit\u00e9s${C}.`,
    },
    targets_coded: {
      "fr-FR": `Cliquez ${O}ici${C} pour en savoir plus sur nos ${O}fonctionnalit\u00e9s${C}.`,
    },
    translatable: true,
    has_spans: true,
    properties: { state: "translated" },
  },
  {
    id: "ic-2",
    source: "Important: Run kapi init to get started.\nSee the docs for more.",
    source_coded: `${O}Important:${C} Run ${O}kapi init${C} to get started.${P}See the ${O}docs${C} for more.`,
    source_spans: [
      { span_type: "opening", type: "b", id: "3", data: "<b>" },
      { span_type: "closing", type: "b", id: "3", data: "</b>" },
      { span_type: "opening", type: "code", id: "4", data: "<code>" },
      { span_type: "closing", type: "code", id: "4", data: "</code>" },
      { span_type: "placeholder", type: "br", id: "5", data: "<br/>" },
      { span_type: "opening", type: "a", id: "6", data: '<a href="/docs">' },
      { span_type: "closing", type: "a", id: "6", data: "</a>" },
    ],
    targets: {
      "fr-FR": `${O}Important\u00a0:${C} Ex\u00e9cutez ${O}kapi init${C} pour commencer.${P}Consultez la ${O}documentation${C} pour plus de d\u00e9tails.`,
    },
    targets_coded: {
      "fr-FR": `${O}Important\u00a0:${C} Ex\u00e9cutez ${O}kapi init${C} pour commencer.${P}Consultez la ${O}documentation${C} pour plus de d\u00e9tails.`,
    },
    translatable: true,
    has_spans: true,
    properties: { state: "draft" },
  },
  {
    id: "ic-3",
    source: "Upload your file (max 10 MB) and click Submit.",
    source_coded: `Upload your file (max ${O}10 MB${C}) and click ${O}Submit${C}.`,
    source_spans: [
      { span_type: "opening", type: "i", id: "7", data: "<i>" },
      { span_type: "closing", type: "i", id: "7", data: "</i>" },
      { span_type: "opening", type: "b", id: "8", data: "<b>" },
      { span_type: "closing", type: "b", id: "8", data: "</b>" },
    ],
    targets: { "fr-FR": "" },
    targets_coded: { "fr-FR": "" },
    translatable: true,
    has_spans: true,
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
    name: "QA check after translation",
    trigger: "translation.completed",
    conditions: [],
    actions: [{ Type: "run_flow", Config: { flow: "qa-check" } }],
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
      collection_name: "Default",
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
