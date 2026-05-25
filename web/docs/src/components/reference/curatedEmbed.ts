// Decide which curated framework-view (if any) belongs on a per-entry static
// reference page (R4, #673). The curated components live in
// src/components/curated; here we only compute *whether* one applies and *what
// props* it gets — purely from the reference data, so the page stays generated.
//
//   • format → <BlockPreview sample="…" /> — only when a bundled kit fixture
//     matches one of the format's declared extensions (the same fixtures the
//     playground ships: messages.json, app.xliff, page.html, README.md,
//     app.properties, strings.xml, Localizable.xcstrings). Otherwise omit:
//     binary / unsupported formats (idml, mif, openxml, pdf, …) would only
//     error in the in-browser reader, so those pages are static-only.
//   • tool → <BeforeAfter sample="…" command="…" /> — only when the tool is a
//     built-in, offline-runnable *transform* with a dedicated CLI command that
//     writes an output file (so there is a meaningful before/after to show).
//     Analysis tools that print a report to stdout, AI/credentialed tools, and
//     Okapi-bridge tools (need the Java subprocess) are static-only.

import type { ReferenceEntry } from "@neokapi/reference-data";

// ── Format → fixture affinity ───────────────────────────────────────────────

/** Extension → bundled fixture name, most-specific first. */
const EXT_TO_FIXTURE: Record<string, string> = {
  ".xliff": "app.xliff",
  ".xlf": "app.xliff",
  ".html": "page.html",
  ".htm": "page.html",
  ".xhtml": "page.html",
  ".md": "README.md",
  ".markdown": "README.md",
  ".properties": "app.properties",
  ".xml": "strings.xml",
  ".xcstrings": "Localizable.xcstrings",
  ".json": "messages.json",
};

/**
 * The fixture name a BlockPreview should read for a format, or null when no
 * bundled sample matches any of the format's extensions (→ static-only page).
 */
export function formatPreviewSample(entry: ReferenceEntry): string | null {
  for (const ext of entry.extensions ?? []) {
    const fix = EXT_TO_FIXTURE[ext.toLowerCase()];
    if (fix) return fix;
  }
  return null;
}

// ── Tool → before/after transform ───────────────────────────────────────────

/**
 * Built-in transform tools that have a dedicated top-level CLI command which
 * writes an output file (`-o out.<ext>`), so a source→result BeforeAfter is
 * meaningful. Each maps to the fixture to feed it. Kept to JSON-friendly
 * transforms so the in-browser run is fast and deterministic.
 *
 * Pure-analysis tools (word-count, char-count, scoping-report, …) print to
 * stdout rather than producing an output file, so a before/after pane would be
 * empty — those pages are static-only.
 */
interface ToolTransform {
  /** The kapi command line; {in} / {out} are filled with the fixture + output. */
  command: string;
  /** The bundled fixture to use as source. */
  sample: string;
  /** The output file the command writes. */
  output: string;
  /** A short caption describing the transform. */
  caption: string;
}

// Only `pseudo-translate` is enabled: it is built-in, offline, deterministic,
// needs no bridge or credentials, writes a distinct output file, and produces a
// clearly different result on the bundled JSON fixture. Other built-in
// transforms either no-op on the (target-less, PII-free) fixtures or print a
// report to stdout rather than producing an output file, so they would yield an
// empty or unchanged "after" pane — those pages stay static-only.
const TOOL_TRANSFORMS: Record<string, ToolTransform> = {
  "pseudo-translate": {
    command: "kapi pseudo-translate messages.json -o out.json --target-lang fr",
    sample: "messages.json",
    output: "out.json",
    caption: "Pseudo-translating a JSON message catalog with the built-in tool.",
  },
};

/**
 * BeforeAfter props for a tool, or null when no meaningful in-browser transform
 * applies (→ static-only page). Restricted to the curated TOOL_TRANSFORMS so the
 * command, sample, and output are all known-good and deterministic.
 */
export function toolBeforeAfter(entry: ReferenceEntry): ToolTransform | null {
  if (entry.source !== "built-in") return null;
  return TOOL_TRANSFORMS[entry.id] ?? null;
}
