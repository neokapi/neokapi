// The worked example behind the KBF anatomy page (/kbf-lab): one realistic
// extracted source file, serialized field-by-field so every line of the
// resulting `.kbf.json` text is tagged with the anatomical part it belongs to
// (envelope, generator, project, document, block, editions, run, placeholders,
// provenance). The page renders the lines as a highlighted source pane and
// uses the region tags to drive the explanation pane.
//
// The document is authored as typed @neokapi/kapi-format objects so it
// type-checks against the wire schema, and the emitter follows the normative
// field order of the spec (/reference/serialization/content-bundle): envelope
// fields in struct order, block fields id → hash → translatable → type →
// editions → placeholders → properties, edition keys sorted, and each
// edition's fields runs → status → origin → score → derived.

import type { Block, Edition, File, Run } from "@neokapi/kapi-format";
import { SchemaVersion, SourceEdition, sourceEditions } from "@neokapi/kapi-format";
import { t } from "@neokapi/i18n-react/runtime";

// ─── The example document ────────────────────────────────────────────────
//
// A checkout banner: one JSX component contributing two blocks: a heading
// with inline markup and a variable, translated into Norwegian Bokmål, and an
// untranslated plural.

const bannerHeading: Block = {
  id: "banner-heading",
  hash: "8kQzTf",
  translatable: true,
  type: "jsx:element",
  editions: {
    [SourceEdition]: {
      runs: [
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
    },
    nb: {
      runs: [
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
      status: "translated",
      origin: { kind: "ai", engine: "claude", tool: "translate" },
      derived: { from: SourceEdition, rev: "r:5b1d7e0c93a24f68" },
    },
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
  editions: sourceEditions([
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
  ]),
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
  schemaVersion: SchemaVersion,
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
  | "editions"
  | "edition-meta"
  | "run-text"
  | "run-ph"
  | "run-pc"
  | "run-plural"
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
    title: t("File envelope", "KBF anatomy term"),
    spec: "/reference/serialization/content-bundle#file-envelope",
    body: [
      t(
        "The top-level object of a .kbf.json file. kind is the magic string readers sniff to detect the format; schemaVersion carries the MAJOR.MINOR wire contract — a consumer must reject an unrecognized major version and should accept unknown minors, ignoring fields it does not recognize.",
        "KBF anatomy explanation",
      ),
      t(
        "Serialization is deterministic: 2-space indent, fields in the pinned order shown here, all map keys sorted, trailing newline. Determinism is what keeps a file's content hash stable across runs, machines, and the two reference implementations.",
        "KBF anatomy explanation",
      ),
    ],
  },
  {
    id: "generator",
    title: t("GeneratorInfo", "KBF anatomy term"),
    spec: "/reference/serialization/content-bundle#generatorinfo",
    body: [
      t(
        "Extraction provenance at the file level: the extractor that produced this file, its version, and its declared capabilities. Together with each block's properties, this forms the chain from any translated string back to the tool and the source location that produced it.",
        "KBF anatomy explanation",
      ),
    ],
  },
  {
    id: "project",
    title: t("ProjectInfo", "KBF anatomy term"),
    spec: "/reference/serialization/content-bundle#projectinfo",
    body: [
      t(
        "The project this file belongs to, and the BCP-47 locale of every block's source edition. Other languages are not declared here. They appear per block, as keys of the editions map.",
        "KBF anatomy explanation",
      ),
    ],
  },
  {
    id: "vocabulary",
    title: t("Vocabulary", "KBF anatomy term"),
    spec: "/reference/serialization/content-bundle#vocabulary",
    body: [
      t(
        "The vocabulary packs whose span-type entries this file's runs reference — the packs a consumer is expected to have loaded to interpret run types such as jsx:element or jsx:var.",
        "KBF anatomy explanation",
      ),
    ],
  },
  {
    id: "document",
    title: t("Document", "KBF anatomy term"),
    spec: "/reference/serialization/content-bundle#document",
    body: [
      t(
        "One extracted source file: its path, its documentType, and the blocks extracted from it. A .kbf.json file wraps one or more documents. A document may also carry a sourceHash and a skeleton — the opaque payload a merge step consumes to reconstruct the original file with translated content spliced back in (omitted in this example).",
        "KBF anatomy explanation",
      ),
    ],
  },
  {
    id: "block",
    title: t("Block", "KBF anatomy term"),
    spec: "/reference/serialization/content-bundle#block",
    body: [
      t(
        "The unit of translation tracking: typically one JSX element, one HTML paragraph, one Markdown heading, or one attribute value. Content memory, editions, review status, and annotations are all keyed on the block.",
        "KBF anatomy explanation",
      ),
      t(
        "hash is a content hash over the source runs. Re-extractions match blocks by hash, which is how tracking survives edits to the surrounding file: an unchanged block keeps its identity, a changed one is flagged for retranslation.",
        "KBF anatomy explanation",
      ),
    ],
  },
  {
    id: "editions",
    title: t("Editions", "KBF anatomy term"),
    spec: "/reference/serialization/content-bundle#editions",
    body: [
      t(
        "The block's content as peer editions, each under its edition key. The empty key holds the source, in the project's source locale; every translation sits under its BCP-47 locale, and an edition with a tone or a channel under a key such as en;channel=short. Keys are sorted in canonical output.",
        "KBF anatomy explanation",
      ),
      t(
        "Each edition holds its content as a flat sequence of runs. Inline markup lives in runs rather than in the text, so a consumer has no marked-up string to re-parse, and a translator's tool can present each run as an atomic token. A translation is validated against the source: every required placeholder must be preserved.",
        "KBF anatomy explanation",
      ),
      t(
        "A block with no edition in a language is untranslated in that language. The second block in this example has only its source.",
        "KBF anatomy explanation",
      ),
    ],
  },
  {
    id: "edition-meta",
    title: t("Edition status and provenance", "KBF anatomy term"),
    spec: "/reference/serialization/content-bundle#edition",
    body: [
      t(
        "Beside its runs an edition records where it stands (status), how it was produced and under what governance (origin), and, for a derived edition, the edition it was made from and that edition's revision at the time (derived). Comparing that revision with the source's current one tells whether the translation is stale.",
        "KBF anatomy explanation",
      ),
    ],
  },
  {
    id: "run-text",
    title: t("Run — text", "KBF anatomy term"),
    spec: "/reference/serialization/content-bundle#text-a-plain-text-chunk",
    body: [
      t(
        "A plain text chunk: the only run kind that carries translatable characters. Everything else in the sequence is structure.",
        "KBF anatomy explanation",
      ),
    ],
  },
  {
    id: "run-pc",
    title: t("Run — pcOpen / pcClose", "KBF anatomy term"),
    spec: "/reference/serialization/content-bundle#pcopen--pcclose-paired-codes",
    body: [
      t(
        "A paired code: an inline element that opens and closes around translatable content, split into two runs so the enclosed text stays part of the flat sequence. The shared id pairs them; equiv is the stable name a target uses to reference the pair; data preserves the original markup for write-back.",
        "KBF anatomy explanation",
      ),
    ],
  },
  {
    id: "run-ph",
    title: t("Run — ph", "KBF anatomy term"),
    spec: "/reference/serialization/content-bundle#ph-a-self-closing-placeholder",
    body: [
      t(
        "A self-closing placeholder: a variable or void element that contributes no translatable text of its own. equiv names it, disp is a display hint for editors, and data preserves the source expression for write-back. Validation rejects a target that drops a required placeholder.",
        "KBF anatomy explanation",
      ),
    ],
  },
  {
    id: "run-plural",
    title: t("Run — plural", "KBF anatomy term"),
    spec: "/reference/serialization/content-bundle#plural-a-structured-plural-construct",
    body: [
      t(
        "A structured plural construct: each CLDR plural form holds its own run sequence, keyed by the pivot placeholder that selects the form at render time. Plurals are structured data, not flattened strings — a target language supplies exactly the forms its plural rules require.",
        "KBF anatomy explanation",
      ),
    ],
  },
  {
    id: "placeholders",
    title: t("Placeholders", "KBF anatomy term"),
    spec: "/reference/serialization/content-bundle#placeholder",
    body: [
      t(
        "Every variable and element token referenced by the block's runs, enumerated once — including those inside plural and select forms — so validators and CAT tools can examine them without walking the run tree. Metadata that does not fit on a run (jsType, sourceExpr, optionality) lives here.",
        "KBF anatomy explanation",
      ),
    ],
  },
  {
    id: "provenance",
    title: t("Provenance — BlockProperties", "KBF anatomy term"),
    spec: "/reference/serialization/content-bundle#blockproperties",
    body: [
      t(
        "Where the block came from: the source file and line, the component, the JSX path, and the element that produced it, plus an optional note for translators (locNote). With the file-level generator, this is the full provenance of the string: which tool extracted it, from which file, at which position.",
        "KBF anatomy explanation",
      ),
    ],
  },
];

export function termById(id: TermId): AnatomyTerm {
  const term = TERMS.find((x) => x.id === id);
  if (!term) throw new Error(`unknown anatomy term: ${id}`);
  return term;
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
  return "editions";
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

  // One edition: its runs, each tagged with its run kind, then whatever it
  // records about itself, in the order the serializer writes the fields.
  const emitEdition = (key: string, e: Edition, level: number, comma: boolean): void => {
    push(`${pad(level)}${JSON.stringify(key)}: {`, "editions");
    const meta: Array<[string, unknown]> = [];
    if (e.status) meta.push(["status", e.status]);
    if (e.origin) meta.push(["origin", e.origin]);
    if (e.score) meta.push(["score", e.score]);
    if (e.derived) meta.push(["derived", e.derived]);
    push(`${pad(level + 1)}"runs": [`, "editions");
    e.runs.forEach((run, i) => {
      emit(null, run, level + 2, runTerm(run), i < e.runs.length - 1);
    });
    push(pad(level + 1) + (meta.length > 0 ? "]," : "]"), "editions");
    meta.forEach(([field, value], i) => {
      emit(field, value, level + 1, "edition-meta", i < meta.length - 1);
    });
    push(pad(level) + (comma ? "}," : "}"), "editions");
  };

  const emitBlock = (b: Block, last: boolean): void => {
    const L = 4; // indent level of a block object inside documents[0].blocks
    push(pad(L) + "{", "block");
    emit("id", b.id, L + 1, "block", true);
    emit("hash", b.hash, L + 1, "block", true);
    emit("translatable", b.translatable, L + 1, "block", true);
    emit("type", b.type, L + 1, "block", true);
    push(pad(L + 1) + '"editions": {', "editions");
    const keys = Object.keys(b.editions).sort();
    keys.forEach((key, i) => emitEdition(key, b.editions[key], L + 2, i < keys.length - 1));
    push(pad(L + 1) + "},", "editions");
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
