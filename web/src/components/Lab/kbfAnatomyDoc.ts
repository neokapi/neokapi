// The worked example behind the KBF anatomy page (/kbf-lab): one realistic
// extracted source file, serialized field-by-field so every line of the
// resulting `.kbf.json` text is tagged with the anatomical part it belongs to
// (envelope, generator, project, document, block, run, targets, placeholders,
// provenance). The page renders the lines as a highlighted source pane and
// uses the region tags to drive the explanation pane.
//
// The document is authored as typed @neokapi/kapi-format objects so it
// type-checks against the wire schema, and the emitter follows the normative
// field order of the spec (/reference/serialization/content-bundle): envelope fields in struct
// order, block fields id → hash → translatable → type → source → targets →
// placeholders → properties, and target locales sorted.

import type { Block, File, Run } from "@neokapi/kapi-format";

// ─── The example document ────────────────────────────────────────────────
//
// A checkout banner: one JSX component contributing two blocks — a heading
// with inline markup and a variable (translated into Norwegian Bokmål), and
// an untranslated plural.

const bannerHeading: Block = {
  id: "banner-heading",
  hash: "8kQzTf",
  translatable: true,
  type: "jsx:element",
  source: [
    { text: "Your order " },
    {
      pcOpen: {
        id: "1",
        type: "jsx:element",
        subType: "strong",
        data: "<strong>",
        equiv: "emph",
        disp: "strong",
      },
    },
    { text: "#" },
    {
      ph: {
        id: "2",
        type: "jsx:var",
        subType: "string",
        data: "{orderId}",
        equiv: "orderId",
        disp: "orderId",
      },
    },
    {
      pcClose: {
        id: "1",
        type: "jsx:element",
        subType: "strong",
        data: "</strong>",
        equiv: "emph",
      },
    },
    { text: " has shipped." },
  ],
  targets: {
    nb: [
      { text: "Bestillingen din " },
      {
        pcOpen: {
          id: "1",
          type: "jsx:element",
          subType: "strong",
          data: "<strong>",
          equiv: "emph",
          disp: "strong",
        },
      },
      { text: "#" },
      {
        ph: {
          id: "2",
          type: "jsx:var",
          subType: "string",
          data: "{orderId}",
          equiv: "orderId",
          disp: "orderId",
        },
      },
      {
        pcClose: {
          id: "1",
          type: "jsx:element",
          subType: "strong",
          data: "</strong>",
          equiv: "emph",
        },
      },
      { text: " er sendt." },
    ],
  },
  placeholders: [
    {
      name: "emph",
      kind: "element",
      jsType: "ReactNode",
      sourceExpr: "<strong>...</strong>",
    },
    {
      name: "orderId",
      kind: "variable",
      jsType: "string",
      sourceExpr: "order.id",
    },
  ],
  properties: {
    file: "src/CheckoutBanner.tsx",
    line: 12,
    component: "CheckoutBanner",
    jsxPath: "CheckoutBanner > h2",
    element: "h2",
  },
};

const bannerItems: Block = {
  id: "banner-items",
  hash: "3mVd9c",
  translatable: true,
  type: "jsx:element",
  source: [
    {
      plural: {
        pivot: "count",
        forms: {
          one: [{ text: "It contains 1 item." }],
          other: [
            { text: "It contains " },
            {
              ph: {
                id: "1",
                type: "jsx:var",
                subType: "number",
                data: "{count}",
                equiv: "count",
                disp: "count",
              },
            },
            { text: " items." },
          ],
        },
      },
    },
  ],
  placeholders: [
    {
      name: "count",
      kind: "icu-pivot",
      jsType: "number",
      sourceExpr: "order.items.length",
    },
  ],
  properties: {
    file: "src/CheckoutBanner.tsx",
    line: 18,
    component: "CheckoutBanner",
    jsxPath: "CheckoutBanner > p > Plural",
    element: "Plural",
  },
};

export const ANATOMY_FILE: File = {
  schemaVersion: "1.0",
  kind: "kapi-bundle",
  created: "2026-05-02T09:30:00Z",
  generator: {
    id: "@neokapi/kapi-format-examples",
    version: "0.0.1",
    capabilities: ["extract", "preview"],
  },
  project: { id: "storefront", sourceLocale: "en" },
  vocabulary: { extends: ["common-formatting", "rich-jsx"] },
  documents: [
    {
      id: "checkout-banner",
      documentType: "jsx",
      path: "src/CheckoutBanner.tsx",
      blocks: [bannerHeading, bannerItems],
    },
  ],
};

// ─── Anatomy terms ───────────────────────────────────────────────────────

export type TermId =
  | "envelope"
  | "generator"
  | "project"
  | "vocabulary"
  | "document"
  | "block"
  | "source"
  | "run-text"
  | "run-ph"
  | "run-pc"
  | "run-plural"
  | "targets"
  | "placeholders"
  | "provenance";

export interface AnatomyTerm {
  id: TermId;
  /** Term name, as the spec uses it. */
  title: string;
  /** Anchor into the normative spec. */
  spec: string;
  /** Explanation paragraphs (academic register; no chrome). */
  body: string[];
}

/** Terms in reading order — the right-hand pane's table of contents. */
export const TERMS: AnatomyTerm[] = [
  {
    id: "envelope",
    title: "File envelope",
    spec: "/reference/serialization/content-bundle#file-envelope",
    body: [
      "The top-level object of a .kbf.json file. kind is the magic string readers sniff to detect the format; schemaVersion carries the MAJOR.MINOR wire contract — a consumer must reject an unrecognized major version and should accept unknown minors, ignoring fields it does not recognize.",
      "Serialization is deterministic: 2-space indent, fields in the pinned order shown here, all map keys sorted, trailing newline. Determinism is what keeps a file's content hash stable across runs, machines, and the two reference implementations.",
    ],
  },
  {
    id: "generator",
    title: "GeneratorInfo",
    spec: "/reference/serialization/content-bundle#generatorinfo",
    body: [
      "Extraction provenance at the file level: the extractor that produced this file, its version, and its declared capabilities. Together with each block's properties, this forms the chain from any translated string back to the tool and the source location that produced it.",
    ],
  },
  {
    id: "project",
    title: "ProjectInfo",
    spec: "/reference/serialization/content-bundle#projectinfo",
    body: [
      "The project this file belongs to, and the BCP-47 locale of every source run sequence in it. Target locales are not declared here — they appear per block, as keys of the targets map.",
    ],
  },
  {
    id: "vocabulary",
    title: "Vocabulary",
    spec: "/reference/serialization/content-bundle#vocabulary",
    body: [
      "The vocabulary packs whose span-type entries this file's runs reference — the packs a consumer is expected to have loaded to interpret run types such as jsx:element or jsx:var.",
    ],
  },
  {
    id: "document",
    title: "Document",
    spec: "/reference/serialization/content-bundle#document",
    body: [
      "One extracted source file: its path, its documentType, and the blocks extracted from it. A .kbf.json file wraps one or more documents. A document may also carry a sourceHash and a skeleton — the opaque payload a merge step consumes to reconstruct the original file with translated content spliced back in (omitted in this example).",
    ],
  },
  {
    id: "block",
    title: "Block",
    spec: "/reference/serialization/content-bundle#block",
    body: [
      "The unit of translation tracking — typically one JSX element, one HTML paragraph, one Markdown heading, or one attribute value. Content memory, targets, review status, and annotations are all keyed on the block.",
      "hash is a content hash over the source runs. Re-extractions match blocks by hash, which is how tracking survives edits to the surrounding file: an unchanged block keeps its identity, a changed one is flagged for retranslation.",
    ],
  },
  {
    id: "source",
    title: "Source runs",
    spec: "/reference/serialization/content-bundle#the-run-model",
    body: [
      "The source content as a flat sequence of runs. Inline markup lives in runs, not in the text — there is no marked-up string for a consumer to re-parse, and a translator's tool can present each run as an atomic token.",
    ],
  },
  {
    id: "run-text",
    title: "Run — text",
    spec: "/reference/serialization/content-bundle#text-a-plain-text-chunk",
    body: [
      "A plain text chunk: the only run kind that carries translatable characters. Everything else in the sequence is structure.",
    ],
  },
  {
    id: "run-pc",
    title: "Run — pcOpen / pcClose",
    spec: "/reference/serialization/content-bundle#pcopen--pcclose-paired-codes",
    body: [
      "A paired code: an inline element that opens and closes around translatable content, split into two runs so the enclosed text stays part of the flat sequence. The shared id pairs them; equiv is the stable name a target uses to reference the pair; data preserves the original markup for write-back.",
    ],
  },
  {
    id: "run-ph",
    title: "Run — ph",
    spec: "/reference/serialization/content-bundle#ph-a-self-closing-placeholder",
    body: [
      "A self-closing placeholder: a variable or void element that contributes no translatable text of its own. equiv names it, disp is a display hint for editors, and data preserves the source expression for write-back. Validation rejects a target that drops a required placeholder.",
    ],
  },
  {
    id: "run-plural",
    title: "Run — plural",
    spec: "/reference/serialization/content-bundle#plural-a-structured-plural-construct",
    body: [
      "A structured plural construct: each CLDR plural form holds its own run sequence, keyed by the pivot placeholder that selects the form at render time. Plurals are structured data, not flattened strings — a target language supplies exactly the forms its plural rules require.",
    ],
  },
  {
    id: "targets",
    title: "Targets",
    spec: "/reference/serialization/content-bundle#targets",
    body: [
      "Per-locale translations. Each key is a BCP-47 locale (sorted in canonical output) and each value is that locale's own run sequence, validated against the source: every required placeholder must be preserved.",
      "A block with no entry for a locale is simply untranslated there — pending work, not an error. The second block in this example carries no targets yet.",
    ],
  },
  {
    id: "placeholders",
    title: "Placeholders",
    spec: "/reference/serialization/content-bundle#placeholder",
    body: [
      "Every variable and element token referenced by the block's runs, enumerated once — including those inside plural and select forms — so validators and CAT tools can examine them without walking the run tree. Metadata that does not fit on a run (jsType, sourceExpr, optionality) lives here.",
    ],
  },
  {
    id: "provenance",
    title: "Provenance — BlockProperties",
    spec: "/reference/serialization/content-bundle#blockproperties",
    body: [
      "Where the block came from: the source file and line, the component, the JSX path, and the element that produced it, plus an optional note for translators (locNote). With the file-level generator, this is the full provenance of the string: which tool extracted it, from which file, at which position.",
    ],
  },
];

export function termById(id: TermId): AnatomyTerm {
  const t = TERMS.find((x) => x.id === id);
  if (!t) throw new Error(`unknown anatomy term: ${id}`);
  return t;
}

// ─── Line emitter ────────────────────────────────────────────────────────

export interface AnatomyLine {
  text: string;
  /** The anatomy term this line belongs to (innermost region). */
  term: TermId;
}

function runTerm(run: Run): TermId {
  if ("text" in run) return "run-text";
  if ("ph" in run) return "run-ph";
  if ("pcOpen" in run || "pcClose" in run) return "run-pc";
  if ("plural" in run) return "run-plural";
  return "source";
}

/** Serialize the example File to lines, each tagged with its anatomy term. */
function buildLines(f: File): AnatomyLine[] {
  const lines: AnatomyLine[] = [];
  const push = (text: string, term: TermId): void => {
    lines.push({ text, term });
  };
  const pad = (n: number): string => "  ".repeat(n);

  // Emit `"key": <json>` (or a bare value when key is null) at an indent
  // level, tagging every emitted line with the given term.
  const emit = (
    key: string | null,
    value: unknown,
    level: number,
    term: TermId,
    comma: boolean,
  ): void => {
    const raw = JSON.stringify(value, null, 2).split("\n");
    raw.forEach((l, i) => {
      let text = pad(level) + l;
      if (i === 0 && key !== null) text = `${pad(level)}"${key}": ${l}`;
      if (i === raw.length - 1 && comma) text += ",";
      push(text, term);
    });
  };

  const emitBlock = (b: Block, last: boolean): void => {
    const L = 4; // indent level of a block object inside documents[0].blocks
    push(pad(L) + "{", "block");
    emit("id", b.id, L + 1, "block", true);
    emit("hash", b.hash, L + 1, "block", true);
    emit("translatable", b.translatable, L + 1, "block", true);
    emit("type", b.type, L + 1, "block", true);
    push(pad(L + 1) + '"source": [', "source");
    b.source.forEach((run, i) => {
      emit(null, run, L + 2, runTerm(run), i < b.source.length - 1);
    });
    push(pad(L + 1) + "],", "source");
    if (b.targets) {
      push(pad(L + 1) + '"targets": {', "targets");
      const locales = Object.keys(b.targets).sort();
      locales.forEach((loc, i) => {
        emit(loc, b.targets?.[loc], L + 2, "targets", i < locales.length - 1);
      });
      push(pad(L + 1) + "},", "targets");
    }
    emit("placeholders", b.placeholders, L + 1, "placeholders", true);
    emit("properties", b.properties, L + 1, "provenance", false);
    push(pad(L) + (last ? "}" : "},"), "block");
  };

  const d = f.documents[0];
  push("{", "envelope");
  emit("schemaVersion", f.schemaVersion, 1, "envelope", true);
  emit("kind", f.kind, 1, "envelope", true);
  emit("created", f.created, 1, "envelope", true);
  emit("generator", f.generator, 1, "generator", true);
  emit("project", f.project, 1, "project", true);
  emit("vocabulary", f.vocabulary, 1, "vocabulary", true);
  push('  "documents": [', "document");
  push("    {", "document");
  emit("id", d.id, 3, "document", true);
  emit("documentType", d.documentType, 3, "document", true);
  emit("path", d.path, 3, "document", true);
  push(pad(3) + '"blocks": [', "document");
  d.blocks.forEach((b, i) => emitBlock(b, i === d.blocks.length - 1));
  push(pad(3) + "]", "document");
  push("    }", "document");
  push("  ]", "document");
  push("}", "envelope");
  return lines;
}

export const ANATOMY_LINES: AnatomyLine[] = buildLines(ANATOMY_FILE);

/** The example as plain `.kbf.json` text (used for tokenization and copy). */
export const ANATOMY_TEXT: string = ANATOMY_LINES.map((l) => l.text).join("\n") + "\n";
