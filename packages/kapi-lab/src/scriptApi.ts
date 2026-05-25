// The TypeScript ambient declarations that drive Monaco's IntelliSense for the
// script tool's sandbox. These mirror the goja globals the tool exposes
// (core/tools/script.go): `part`, `emit`, `skip`, `log`, and the shape of a
// block's source/target segments. Kept in sync with that file by hand.
export const SCRIPT_API_DTS = `
/** A unit of content flowing through the pipeline. */
declare interface Part {
  /** The kind of part. Only "block" parts carry translatable text. */
  readonly type: "block" | "data" | "media" | "layer-start" | "layer-end" | "group-start" | "group-end";
  /** The translatable block — present only when type === "block". */
  block?: Block;
}

/** A translatable content unit. */
declare interface Block {
  /** Stable identifier (e.g. the JSON key or XLIFF trans-unit id). */
  readonly id: string;
  /** Whether this block is meant to be translated. */
  translatable: boolean;
  /** Source segments. Edit a segment's text in place, or reassign the array. */
  source: Segment[];
  /** Target segments per locale, e.g. part.block.targets["fr"]. */
  targets: { [locale: string]: Segment[] };
}

/** One segment of a block; its content holds the text. */
declare interface Segment {
  content: { text: string };
}

/** The current part being processed. Read and modify it, then emit() it. */
declare const part: Part;

/**
 * Emit a part downstream. Call it once for each part you want to keep
 * (optionally after modifying it). If you call neither emit() nor skip(),
 * the part passes through unchanged.
 */
declare function emit(part: Part): void;

/** Drop the current part — it will not be emitted. */
declare function skip(): void;

/** Write a message to the run log (shown below the editor). */
declare function log(message: string): void;
`;

export interface ScriptExample {
  id: string;
  label: string;
  blurb: string;
  code: string;
}

// A library of small, self-contained scripts. Written in ES5 (no arrow
// functions / template literals) to match the goja sandbox and the tool's
// documented contract.
export const SCRIPT_EXAMPLES: ScriptExample[] = [
  {
    id: "uppercase",
    label: "Uppercase source",
    blurb: "Edit a block's source text in place.",
    code: `// Modify the block's source text, then keep the part.
if (part.type === "block") {
  part.block.source[0].content.text =
    part.block.source[0].content.text.toUpperCase();
}
emit(part);
`,
  },
  {
    id: "filter-short",
    label: "Drop short blocks",
    blurb: "Use skip() to filter parts out of the stream.",
    code: `// Keep only blocks whose source is longer than 10 characters.
if (part.type === "block") {
  if (part.block.source[0].content.text.length > 10) {
    emit(part);
  } else {
    skip();
  }
} else {
  emit(part);
}
`,
  },
  {
    id: "redact-emails",
    label: "Redact email addresses",
    blurb: "Rewrite text with a regular expression.",
    code: `// Replace any email address in the source with [redacted].
if (part.type === "block") {
  var text = part.block.source[0].content.text;
  part.block.source[0].content.text =
    text.replace(/[\\w.+-]+@[\\w.-]+\\.[a-z]{2,}/gi, "[redacted]");
}
emit(part);
`,
  },
  {
    id: "add-target",
    label: "Add a French target",
    blurb: "Write a translation by reassigning targets[locale].",
    code: `// Attach a target translation for a locale.
if (part.type === "block") {
  part.block.targets["fr"] = [{ content: { text: "Bonjour" } }];
}
emit(part);
`,
  },
  {
    id: "wrap",
    label: "Wrap with markers",
    blurb: "Prefix and suffix the source text.",
    code: `// Surround the source text with guillemets.
if (part.type === "block") {
  part.block.source[0].content.text =
    "\\u00ab " + part.block.source[0].content.text + " \\u00bb";
}
emit(part);
`,
  },
  {
    id: "log-by-type",
    label: "Log each part",
    blurb: "Inspect the stream with log() — output appears below.",
    code: `// Log every part's type (and block id), then pass it through.
if (part.type === "block") {
  log("block " + part.block.id + ": " + part.block.source[0].content.text);
} else {
  log(part.type);
}
emit(part);
`,
  },
  {
    id: "mask-vars",
    label: "Mask placeholders",
    blurb: "Replace {variables} so they stand out.",
    code: `// Turn {name} style placeholders into <<name>>.
if (part.type === "block") {
  var text = part.block.source[0].content.text;
  part.block.source[0].content.text =
    text.replace(/\\{(\\w+)\\}/g, "<<$1>>");
}
emit(part);
`,
  },
  {
    id: "drop-media",
    label: "Strip media parts",
    blurb: "Keep text, drop binary/media parts.",
    code: `// Remove media parts from the stream; keep everything else.
if (part.type === "media") {
  skip();
} else {
  emit(part);
}
`,
  },
];

export const DEFAULT_SCRIPT = SCRIPT_EXAMPLES[0].code;
