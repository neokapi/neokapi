/**
 * Shared test fixtures for Storybook stories.
 *
 * Realistic localization data modelled after HTML-in-JSON content —
 * the most common scenario in the translation editor.
 */

import type { SpanInfo, BlockInfo, ProjectInfo } from "../types/api";

// ---------------------------------------------------------------------------
// Spans (inline markup tags)
// ---------------------------------------------------------------------------

export const boldOpen: SpanInfo = { span_type: "opening", type: "b", id: "1", data: "<b>" };
export const boldClose: SpanInfo = { span_type: "closing", type: "b", id: "1", data: "</b>" };
export const italicOpen: SpanInfo = { span_type: "opening", type: "i", id: "2", data: "<i>" };
export const italicClose: SpanInfo = { span_type: "closing", type: "i", id: "2", data: "</i>" };
export const linkOpen: SpanInfo = { span_type: "opening", type: "a", id: "3", data: '<a href="https://example.com">' };
export const linkClose: SpanInfo = { span_type: "closing", type: "a", id: "3", data: "</a>" };
export const codeOpen: SpanInfo = { span_type: "opening", type: "code", id: "4", data: "<code>" };
export const codeClose: SpanInfo = { span_type: "closing", type: "code", id: "4", data: "</code>" };
export const lineBreak: SpanInfo = { span_type: "placeholder", type: "br", id: "5", data: "<br/>" };
export const imgTag: SpanInfo = { span_type: "placeholder", type: "img", id: "6", data: '<img src="logo.png"/>' };
export const underlineOpen: SpanInfo = { span_type: "opening", type: "u", id: "7", data: "<u>" };
export const underlineClose: SpanInfo = { span_type: "closing", type: "u", id: "7", data: "</u>" };
export const strikeOpen: SpanInfo = { span_type: "opening", type: "s", id: "8", data: "<s>" };
export const strikeClose: SpanInfo = { span_type: "closing", type: "s", id: "8", data: "</s>" };
export const supOpen: SpanInfo = { span_type: "opening", type: "sup", id: "9", data: "<sup>" };
export const supClose: SpanInfo = { span_type: "closing", type: "sup", id: "9", data: "</sup>" };

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
