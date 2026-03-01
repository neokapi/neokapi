/**
 * Shared test fixtures for Storybook stories.
 *
 * Realistic localization data modelled after HTML-in-JSON content —
 * the most common scenario in the translation editor.
 */

import type {
  SpanInfo, BlockInfo, ProjectInfo,
  TMMatchInfo, BlockTermMatch, QAIssue, FileQAResult,
  BlockNote, BlockHistoryEntry,
} from "../types/api";

// ---------------------------------------------------------------------------
// Spans (inline markup tags)
// ---------------------------------------------------------------------------

export const boldOpen: SpanInfo = { span_type: "opening", type: "fmt:bold", id: "1", data: "<b>" };
export const boldClose: SpanInfo = { span_type: "closing", type: "fmt:bold", id: "1", data: "</b>" };
export const italicOpen: SpanInfo = { span_type: "opening", type: "fmt:italic", id: "2", data: "<i>" };
export const italicClose: SpanInfo = { span_type: "closing", type: "fmt:italic", id: "2", data: "</i>" };
export const linkOpen: SpanInfo = { span_type: "opening", type: "link:hyperlink", id: "3", data: '<a href="https://example.com">' };
export const linkClose: SpanInfo = { span_type: "closing", type: "link:hyperlink", id: "3", data: "</a>" };
export const codeOpen: SpanInfo = { span_type: "opening", type: "fmt:code", id: "4", data: "<code>" };
export const codeClose: SpanInfo = { span_type: "closing", type: "fmt:code", id: "4", data: "</code>" };
export const lineBreak: SpanInfo = { span_type: "placeholder", type: "struct:break", id: "5", data: "<br/>" };
export const imgTag: SpanInfo = { span_type: "placeholder", type: "media:image", id: "6", data: '<img src="logo.png"/>' };
export const underlineOpen: SpanInfo = { span_type: "opening", type: "fmt:underline", id: "7", data: "<u>" };
export const underlineClose: SpanInfo = { span_type: "closing", type: "fmt:underline", id: "7", data: "</u>" };
export const strikeOpen: SpanInfo = { span_type: "opening", type: "fmt:strikethrough", id: "8", data: "<s>" };
export const strikeClose: SpanInfo = { span_type: "closing", type: "fmt:strikethrough", id: "8", data: "</s>" };
export const supOpen: SpanInfo = { span_type: "opening", type: "fmt:superscript", id: "9", data: "<sup>" };
export const supClose: SpanInfo = { span_type: "closing", type: "fmt:superscript", id: "9", data: "</sup>" };

// Markdown-style spans (semantic types, delimiter data)
export const mdBoldOpen: SpanInfo = { span_type: "opening", type: "bold", id: "1", data: "**" };
export const mdBoldClose: SpanInfo = { span_type: "closing", type: "bold", id: "1", data: "**" };
export const mdItalicOpen: SpanInfo = { span_type: "opening", type: "italic", id: "2", data: "*" };
export const mdItalicClose: SpanInfo = { span_type: "closing", type: "italic", id: "2", data: "*" };
export const mdCodeOpen: SpanInfo = { span_type: "opening", type: "code", id: "3", data: "`" };
export const mdCodeClose: SpanInfo = { span_type: "closing", type: "code", id: "3", data: "`" };
export const mdLinkOpen: SpanInfo = { span_type: "opening", type: "link", id: "4", data: "[" };
export const mdLinkClose: SpanInfo = { span_type: "closing", type: "link", id: "4", data: "](https://docs.example.com)" };

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
export const linkAndItalicSpans: SpanInfo[] = [linkOpen, linkClose, italicOpen, italicClose];

/** "Use <code>kapi init</code> to set up" */
export const codeInlineCodedText = `Use ${O}kapi init${C} to set up`;
export const codeInlineSpans: SpanInfo[] = [codeOpen, codeClose];

/** "First line<br/>Second line" */
export const lineBreakCodedText = `First line${P}Second line`;
export const lineBreakSpans: SpanInfo[] = [lineBreak];

/** All tag types in one segment */
export const richCodedText = `${O}Bold${C} and ${O}italic${C} with ${O}a link${C} plus ${O}code${C} and ${P}`;
export const richSpans: SpanInfo[] = [
  boldOpen, boldClose, italicOpen, italicClose, linkOpen, linkClose, codeOpen, codeClose, lineBreak,
];

/** Markdown: "Click **here** to *learn more*" */
export const mdFormattingCodedText = `Click ${O}here${C} to ${O}learn more${C}`;
export const mdFormattingSpans: SpanInfo[] = [mdBoldOpen, mdBoldClose, mdItalicOpen, mdItalicClose];

/** Markdown: "Run `kapi init` and visit [docs](url)" */
export const mdRichCodedText = `Run ${O}kapi init${C} and visit ${O}docs${C}`;
export const mdRichSpans: SpanInfo[] = [mdCodeOpen, mdCodeClose, mdLinkOpen, mdLinkClose];

// ---------------------------------------------------------------------------
// Block samples
// ---------------------------------------------------------------------------

export const sampleBlocks: BlockInfo[] = [
  {
    id: "blk-1",
    source: "Welcome to Gokapi",
    source_coded: "Welcome to Gokapi",
    source_spans: [],
    targets: { "fr-FR": "Bienvenue sur Gokapi", "de-DE": "Willkommen bei Gokapi" },
    targets_coded: { "fr-FR": "Bienvenue sur Gokapi", "de-DE": "Willkommen bei Gokapi" },
    translatable: true,
    has_spans: false,
    properties: { "state": "translated" },
  },
  {
    id: "blk-2",
    source: "Click here to continue",
    source_coded: simpleBoldCodedText,
    source_spans: simpleBoldSpans,
    targets: { "fr-FR": `Cliquez ${O}ici${C} pour continuer`, "de-DE": "" },
    targets_coded: { "fr-FR": `Cliquez ${O}ici${C} pour continuer`, "de-DE": "" },
    translatable: true,
    has_spans: true,
    properties: { "state": "draft" },
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
    targets: { "fr-FR": `Utilisez ${O}kapi init${C} pour configurer`, "de-DE": `Verwenden Sie ${O}kapi init${C} zum Einrichten` },
    targets_coded: { "fr-FR": `Utilisez ${O}kapi init${C} pour configurer`, "de-DE": `Verwenden Sie ${O}kapi init${C} zum Einrichten` },
    translatable: true,
    has_spans: true,
    properties: { "state": "reviewed" },
  },
  {
    id: "blk-5",
    source: "Terms of Service",
    source_coded: "Terms of Service",
    source_spans: [],
    targets: { "fr-FR": "Conditions d'utilisation", "de-DE": "Nutzungsbedingungen" },
    targets_coded: { "fr-FR": "Conditions d'utilisation", "de-DE": "Nutzungsbedingungen" },
    translatable: true,
    has_spans: false,
    properties: { "state": "translated" },
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
    properties: { "state": "draft" },
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
    source: "Welcome to Gokapi",
    target: "Bienvenue sur Gokapi",
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
  { type: "missing-tag", severity: "error", message: 'Missing closing <b> tag in target' },
  { type: "terminology", severity: "warning", message: '"localization" should be translated as "localisation"' },
  { type: "whitespace", severity: "warning", message: "Leading whitespace in target differs from source" },
];

export const sampleFileQAResults: FileQAResult[] = [
  {
    blockId: "blk-2",
    issues: [
      { type: "missing-tag", severity: "error", message: 'Missing closing <b> tag in target' },
      { type: "terminology", severity: "warning", message: '"localization" should be translated as "localisation"' },
    ],
  },
  {
    blockId: "blk-3",
    issues: [
      { type: "whitespace", severity: "warning", message: "Leading whitespace in target differs from source" },
    ],
  },
  {
    blockId: "blk-6",
    issues: [
      { type: "placeholder", severity: "error", message: "Missing placeholder {count} in target" },
      { type: "punctuation", severity: "error", message: 'Target ends with "." but source does not' },
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
    change_type: "update",
    text: "Bienvenue sur Gokapi",
    coded_text: "Bienvenue sur Gokapi",
    author: "translator@example.com",
    origin: "human",
    timestamp: "2026-02-22T14:20:00Z",
  },
  {
    seq: 2,
    change_type: "update",
    text: "Bienvenue chez Gokapi",
    coded_text: "Bienvenue chez Gokapi",
    author: "ai-translate",
    origin: "machine",
    timestamp: "2026-02-21T09:00:00Z",
  },
  {
    seq: 1,
    change_type: "create",
    text: "Bienvenue \u00e0 Gokapi",
    coded_text: "Bienvenue \u00e0 Gokapi",
    author: "pseudo-translate",
    origin: "pseudo",
    timestamp: "2026-02-20T08:00:00Z",
  },
];

// ---------------------------------------------------------------------------
// Project fixture
// ---------------------------------------------------------------------------

export const sampleProject: ProjectInfo = {
  id: "proj-demo-1",
  name: "Demo App",
  source_locale: "en-US",
  target_locales: ["fr-FR", "de-DE", "ja-JP"],
  workspace_id: "ws-1",
  items: [
    { name: "messages.json", format: "json", type: "file", size: 4200, block_count: 6, word_count: 42 },
    { name: "ui-strings.xliff", format: "xliff", type: "file", size: 8100, block_count: 24, word_count: 180 },
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
    source: "Getting Started with Gokapi",
    source_coded: "Getting Started with Gokapi",
    source_spans: [],
    targets: { "fr-FR": "Premiers pas avec Gokapi", "de-DE": "Erste Schritte mit Gokapi" },
    targets_coded: { "fr-FR": "Premiers pas avec Gokapi", "de-DE": "Erste Schritte mit Gokapi" },
    translatable: true,
    has_spans: false,
    properties: { state: "translated" },
  },
  {
    id: "nav-2",
    source: "Gokapi is an AI-native localization framework that makes it easy to translate your applications into multiple languages. It provides format-aware document parsing and channel-based concurrent processing.",
    source_coded: "Gokapi is an AI-native localization framework that makes it easy to translate your applications into multiple languages. It provides format-aware document parsing and channel-based concurrent processing.",
    source_spans: [],
    targets: { "fr-FR": "Gokapi est un framework de localisation natif IA qui facilite la traduction de vos applications dans plusieurs langues. Il fournit une analyse de documents sensible au format et un traitement concurrent bas\u00e9 sur des canaux.", "de-DE": "" },
    targets_coded: { "fr-FR": "Gokapi est un framework de localisation natif IA qui facilite la traduction de vos applications dans plusieurs langues. Il fournit une analyse de documents sensible au format et un traitement concurrent bas\u00e9 sur des canaux.", "de-DE": "" },
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
    source: "Install the CLI tool using Go. Make sure you have Go 1.22 or later installed on your system before proceeding with the setup.",
    source_coded: "Install the CLI tool using Go. Make sure you have Go 1.22 or later installed on your system before proceeding with the setup.",
    source_spans: [],
    targets: { "fr-FR": "Installez l'outil CLI en utilisant Go. Assurez-vous d'avoir Go 1.22 ou une version ult\u00e9rieure install\u00e9e sur votre syst\u00e8me.", "de-DE": "" },
    targets_coded: { "fr-FR": "Installez l'outil CLI en utilisant Go. Assurez-vous d'avoir Go 1.22 ou une version ult\u00e9rieure install\u00e9e sur votre syst\u00e8me.", "de-DE": "" },
    translatable: true,
    has_spans: false,
    properties: { state: "translated" },
  },
  {
    id: "nav-5",
    source: "Run the following command to verify your installation is working correctly and check the installed version.",
    source_coded: "Run the following command to verify your installation is working correctly and check the installed version.",
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
    targets_coded: { "fr-FR": "D\u00e9marrage rapide", "de-DE": "Schnellstart" },
    translatable: true,
    has_spans: false,
    properties: { state: "translated" },
  },
  {
    id: "nav-7",
    source: "Initialize a new project by running kapi init in your project directory. This creates a .kapi folder with default configuration files.",
    source_coded: `Initialize a new project by running ${O}kapi init${C} in your project directory. This creates a ${O}.kapi${C} folder with default configuration files.`,
    source_spans: [
      { span_type: "opening", type: "fmt:code", id: "10", data: "<code>" },
      { span_type: "closing", type: "fmt:code", id: "10", data: "</code>" },
      { span_type: "opening", type: "fmt:code", id: "11", data: "<code>" },
      { span_type: "closing", type: "fmt:code", id: "11", data: "</code>" },
    ],
    targets: { "fr-FR": `Initialisez un nouveau projet en ex\u00e9cutant ${O}kapi init${C} dans votre r\u00e9pertoire de projet. Cela cr\u00e9e un dossier ${O}.kapi${C} avec la configuration par d\u00e9faut.`, "de-DE": "" },
    targets_coded: { "fr-FR": `Initialisez un nouveau projet en ex\u00e9cutant ${O}kapi init${C} dans votre r\u00e9pertoire de projet. Cela cr\u00e9e un dossier ${O}.kapi${C} avec la configuration par d\u00e9faut.`, "de-DE": "" },
    translatable: true,
    has_spans: true,
    properties: { state: "draft" },
  },
  {
    id: "nav-8",
    source: "Add your source files to the project by placing them in the directory structure defined in your configuration. Supported formats include JSON, XLIFF, PO, and many more.",
    source_coded: "Add your source files to the project by placing them in the directory structure defined in your configuration. Supported formats include JSON, XLIFF, PO, and many more.",
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
    source: "The configuration file supports multiple target locales, custom flows, and provider settings. Edit config.yaml in the .kapi directory to customize your translation workflow.",
    source_coded: `The configuration file supports multiple target locales, custom flows, and provider settings. Edit ${O}config.yaml${C} in the ${O}.kapi${C} directory to customize your translation workflow.`,
    source_spans: [
      { span_type: "opening", type: "fmt:code", id: "12", data: "<code>" },
      { span_type: "closing", type: "fmt:code", id: "12", data: "</code>" },
      { span_type: "opening", type: "fmt:code", id: "13", data: "<code>" },
      { span_type: "closing", type: "fmt:code", id: "13", data: "</code>" },
    ],
    targets: { "fr-FR": `Le fichier de configuration prend en charge plusieurs locales cibles. Modifiez ${O}config.yaml${C} dans le r\u00e9pertoire ${O}.kapi${C} pour personnaliser votre workflow.`, "de-DE": "" },
    targets_coded: { "fr-FR": `Le fichier de configuration prend en charge plusieurs locales cibles. Modifiez ${O}config.yaml${C} dans le r\u00e9pertoire ${O}.kapi${C} pour personnaliser votre workflow.`, "de-DE": "" },
    translatable: true,
    has_spans: true,
    properties: { state: "draft" },
  },
  {
    id: "nav-11",
    source: "Translation Workflow",
    source_coded: "Translation Workflow",
    source_spans: [],
    targets: { "fr-FR": "Flux de traduction", "de-DE": "\u00dcbersetzungs-Workflow" },
    targets_coded: { "fr-FR": "Flux de traduction", "de-DE": "\u00dcbersetzungs-Workflow" },
    translatable: true,
    has_spans: false,
    properties: { state: "reviewed" },
  },
  {
    id: "nav-12",
    source: "Use the translate command to process your files through the configured flow. Review and approve translations in the visual editor before exporting the final output.",
    source_coded: "Use the translate command to process your files through the configured flow. Review and approve translations in the visual editor before exporting the final output.",
    source_spans: [],
    targets: { "fr-FR": "Utilisez la commande translate pour traiter vos fichiers via le flux configur\u00e9. V\u00e9rifiez et approuvez les traductions dans l'\u00e9diteur visuel avant d'exporter la sortie finale.", "de-DE": "" },
    targets_coded: { "fr-FR": "Utilisez la commande translate pour traiter vos fichiers via le flux configur\u00e9. V\u00e9rifiez et approuvez les traductions dans l'\u00e9diteur visuel avant d'exporter la sortie finale.", "de-DE": "" },
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
      { span_type: "opening", type: "a", id: "1", data: '<a href="/features">' },
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
