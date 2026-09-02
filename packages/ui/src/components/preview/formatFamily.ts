// formatFamily — what shape of content a format carries, asked of the engine.
//
// A reading surface has to choose a rendering: a flowing document, a slide
// deck, a key table. The classification belongs to the format registry, which
// states it on every format (registry.FormatFamily, declared in
// core/formats/families.go against the eight classes in
// core/formats/constructs.yaml). This module reads the generated map so no
// frontend keeps its own list of format ids, which is how a preview pane came
// to render a `.arb` file as prose because nobody added it to a `Set`.
//
// The dataset is packages/reference-data/data/format-families.json, written by
// `make generate-reference-docs` and gated by `make check-reference-docs`. It
// carries the map alone; formats.json carries the same field alongside every
// format's parameter schema, which is a quarter of a megabyte a preview does
// not need.

import families from "@neokapi/reference-data/data/format-families.json";

/** The eight content shapes a format can carry. */
export const FORMAT_FAMILIES = [
  "rich-markup",
  "office-doc",
  "bilingual-interchange",
  "catalog-keyvalue",
  "subtitle-timedtext",
  "plain-text",
  "data-config",
  "binary-readonly",
] as const;

export type FormatFamily = (typeof FORMAT_FAMILIES)[number];

interface FamilyDataset {
  formats: Record<string, string>;
  extensions: Record<string, string>;
}

const dataset = families as FamilyDataset;

function known(value: string | undefined): FormatFamily | undefined {
  return value && (FORMAT_FAMILIES as readonly string[]).includes(value)
    ? (value as FormatFamily)
    : undefined;
}

/**
 * The family of a format named either by its registry id ("json", "markdown")
 * or by a bare file extension ("yml", "properties"), with or without its dot.
 * Undefined for a name the engine does not know, so a caller falls back to the
 * document reading rather than guessing.
 *
 * Both spellings are accepted because `ContentTree.format` carries whichever
 * its producer had: the engine states the format id, and a host that only has a
 * file name passes `extOf(name)`.
 */
export function formatFamily(format: string | undefined): FormatFamily | undefined {
  if (!format) return undefined;
  const key = format.toLowerCase();
  const byId = known(dataset.formats[key]);
  if (byId) return byId;
  const ext = key.startsWith(".") ? key : `.${key}`;
  return known(dataset.extensions[ext]);
}

/**
 * True when a format's units are addressed by a key path, so a reading surface
 * can show the document as a key table: JSON, YAML, properties, PO, ARB and the
 * rest of the string-resource catalogs.
 */
export function isKeyedFormat(format: string | undefined): boolean {
  return formatFamily(format) === "catalog-keyvalue";
}

/**
 * True when a format's content reads as a flowing document (marked-up text,
 * word-processor documents), so the preview lays it out as prose rather than as
 * a list of entries.
 */
export function isDocumentFormat(format: string | undefined): boolean {
  const family = formatFamily(format);
  return family === "rich-markup" || family === "office-doc";
}

/**
 * True when a format carries source/target pairs of its own (XLIFF, TMX, Qt
 * TS). Its units are keyed, so it reads as an entry list, and the key column is
 * the unit id rather than a nesting path.
 */
export function isInterchangeFormat(format: string | undefined): boolean {
  return formatFamily(format) === "bilingual-interchange";
}
