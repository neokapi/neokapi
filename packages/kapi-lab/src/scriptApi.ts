// The TypeScript ambient declarations that drive Monaco's IntelliSense for the
// script tool's sandbox. These mirror the goja globals the tool exposes
// (core/tools/script.go): `part`, `emit`, `skip`, `log`, and the shape of a
// block's source/target segments. Kept in sync with that file by hand.
//
// Two authoring forms are supported. Define `function process(part) { … }` and
// return the part to keep it (null to drop it); or write top-level code using
// the global `part` and call emit()/skip(). The function form is the default in
// the lab; a JSDoc `@param {Part} part` (a comment, so goja ignores it) gives the
// parameter full type-aware completion.
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

/** The current part (in the global form). Read and modify it, then emit() it. */
declare const part: Part;

/**
 * Emit a part downstream. Call it once for each part you want to keep
 * (optionally after modifying it). If you call neither emit() nor skip() — and
 * a process() function returns nothing — the part passes through unchanged.
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

// A library of small, self-contained scripts in the function form: define
// process(part) and return the part to keep it (or null to drop it). Written in
// ES5 (no arrow functions / template literals) to match the goja sandbox.
export const SCRIPT_EXAMPLES: ScriptExample[] = [
  {
    id: "uppercase",
    label: "Uppercase source",
    blurb: "Edit a block's source text, then return it.",
    code: `/** @param {Part} part */
function process(part) {
  if (part.type === "block") {
    part.block.source[0].content.text =
      part.block.source[0].content.text.toUpperCase();
  }
  return part;
}
`,
  },
  {
    id: "filter-short",
    label: "Drop short blocks",
    blurb: "Return null to drop a part from the stream.",
    code: `/** @param {Part} part */
function process(part) {
  if (part.type !== "block") return part;
  // Keep only blocks whose source is longer than 10 characters.
  return part.block.source[0].content.text.length > 10 ? part : null;
}
`,
  },
  {
    id: "redact-emails",
    label: "Redact email addresses",
    blurb: "Rewrite text with a regular expression.",
    code: `/** @param {Part} part */
function process(part) {
  if (part.type === "block") {
    var text = part.block.source[0].content.text;
    part.block.source[0].content.text =
      text.replace(/[\\w.+-]+@[\\w.-]+\\.[a-z]{2,}/gi, "[redacted]");
  }
  return part;
}
`,
  },
  {
    id: "add-target",
    label: "Add a French target",
    blurb: "Write a translation by assigning targets[locale].",
    code: `/** @param {Part} part */
function process(part) {
  if (part.type === "block") {
    part.block.targets["fr"] = [{ content: { text: "Bonjour" } }];
  }
  return part;
}
`,
  },
  {
    id: "wrap",
    label: "Wrap with markers",
    blurb: "Prefix and suffix the source text.",
    code: `/** @param {Part} part */
function process(part) {
  if (part.type === "block") {
    part.block.source[0].content.text =
      "\\u00ab " + part.block.source[0].content.text + " \\u00bb";
  }
  return part;
}
`,
  },
  {
    id: "log-by-type",
    label: "Log each part",
    blurb: "Inspect the stream with log() — output appears below.",
    code: `/** @param {Part} part */
function process(part) {
  if (part.type === "block") {
    log("block " + part.block.id + ": " + part.block.source[0].content.text);
  } else {
    log(part.type);
  }
  return part;
}
`,
  },
  {
    id: "mask-vars",
    label: "Mask placeholders",
    blurb: "Replace {variables} so they stand out.",
    code: `/** @param {Part} part */
function process(part) {
  if (part.type === "block") {
    var text = part.block.source[0].content.text;
    part.block.source[0].content.text = text.replace(/\\{(\\w+)\\}/g, "<<$1>>");
  }
  return part;
}
`,
  },
  {
    id: "drop-media",
    label: "Strip media parts",
    blurb: "Drop media parts; keep everything else.",
    code: `/** @param {Part} part */
function process(part) {
  return part.type === "media" ? null : part;
}
`,
  },
  {
    id: "globals-form",
    label: "Globals form (no function)",
    blurb: "You can also skip the function and use the global part + emit/skip.",
    code: `// No process() function: this body runs once per Part, with the global
// \`part\` in scope. Call emit(part) to keep it, or skip() to drop it.
if (part.type === "block") {
  part.block.source[0].content.text =
    part.block.source[0].content.text.toUpperCase();
}
emit(part);
`,
  },
];

// The starter shown in the editor: the function form, with a JSDoc-typed param
// for full IntelliSense, returning the part to keep it.
export const DEFAULT_SCRIPT = `// process(part) runs once for every Part in the document.
// Return the part to keep it, or return null to drop it.
// (emit()/skip() also work, and you can omit the function and just use \`part\`.)

/** @param {Part} part */
function process(part) {
  if (part.type === "block") {
    // Edit the block's source text — here, shout it.
    part.block.source[0].content.text =
      part.block.source[0].content.text.toUpperCase();
  }
  return part;
}
`;
