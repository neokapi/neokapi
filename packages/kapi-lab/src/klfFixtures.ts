// Shared KLF fixtures for the KLF Lab and the KLF conformance runner.
//
// These are authored as typed @neokapi/kapi-format objects (so they
// type-check against the wire schema) and surfaced as pretty-printed `.klf`
// text for the editable Lab panels. They mirror the golden fixtures shared
// between Go (core/klf) and TypeScript (@neokapi/kapi-format), plus two extra
// shapes — a `select` construct and a `sub` (subblock) reference — that round
// out the run model for the docs examples.

import type { Block, File } from "@neokapi/kapi-format";

// ─── Individual golden blocks ────────────────────────────────────────────

/** files-heading — a `<span>` paired code wrapping a `{count}` variable. */
export const filesHeading: Block = {
  id: "files-heading",
  hash: "2xykvb",
  translatable: true,
  type: "jsx:element",
  source: [
    { text: "Files " },
    {
      pcOpen: {
        id: "1",
        type: "jsx:element",
        subType: "span",
        data: '<span className="muted">',
        equiv: "muted",
        disp: "span",
      },
    },
    { text: "(" },
    {
      ph: {
        id: "2",
        type: "jsx:var",
        subType: "number",
        data: "{count}",
        equiv: "count",
        disp: "count",
      },
    },
    { text: " matched)" },
    { pcClose: { id: "1", type: "jsx:element", subType: "span", data: "</span>", equiv: "muted" } },
  ],
  placeholders: [
    {
      name: "muted",
      kind: "element",
      jsType: "ReactNode",
      sourceExpr: '<span className="muted">...</span>',
    },
    { name: "count", kind: "variable", jsType: "number", sourceExpr: "count" },
  ],
  properties: {
    file: "src/FilesHeading.tsx",
    line: 4,
    component: "FilesHeading",
    jsxPath: "FilesHeading > h2",
    element: "h2",
  },
  preview: { sampleValues: { count: 3 } },
};

/** tag-chip — two optional `jsx:node` placeholders around a `{label}` variable. */
export const tagChip: Block = {
  id: "tag-chip",
  hash: "2GcSuQ",
  translatable: true,
  type: "jsx:element",
  source: [
    {
      ph: {
        id: "1",
        type: "jsx:node",
        subType: "logical-and",
        data: 'index !== undefined && <span className="badge">{index}</span>',
        equiv: "badge",
        disp: "⟨badge⟩",
      },
    },
    { text: " " },
    {
      ph: {
        id: "2",
        type: "jsx:var",
        subType: "string",
        data: "{label}",
        equiv: "label",
        disp: "label",
      },
    },
    { text: " " },
    {
      ph: {
        id: "3",
        type: "jsx:node",
        subType: "logical-and",
        data: '!deletable && <span className="required">*</span>',
        equiv: "required",
        disp: "⟨required⟩",
      },
    },
  ],
  placeholders: [
    {
      name: "badge",
      kind: "node",
      jsType: "ReactNode",
      sourceExpr: 'index !== undefined && <span className="badge">{index}</span>',
      optional: true,
    },
    { name: "label", kind: "variable", jsType: "string", sourceExpr: "label" },
    {
      name: "required",
      kind: "node",
      jsType: "ReactNode",
      sourceExpr: '!deletable && <span className="required">*</span>',
      optional: true,
    },
  ],
  properties: {
    file: "src/TagChip.tsx",
    line: 3,
    component: "TagChip",
    jsxPath: "TagChip > span[data-tag-chip]",
    element: "span",
    locNote: "Tag chip shown in the sidebar list of filters.",
  },
  preview: {
    storyId: "components-tagchip--default",
    sampleValues: { label: "react", index: 3, deletable: true },
  },
};

/** shopping-cart-plural — a structured `plural` run with three forms. */
export const shoppingCart: Block = {
  id: "shopping-cart-plural",
  hash: "9QpZ11",
  translatable: true,
  type: "jsx:element",
  source: [
    {
      plural: {
        pivot: "count",
        forms: {
          one: [{ text: "1 item in your cart" }],
          other: [
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
            { text: " items in your cart" },
          ],
          zero: [{ text: "Your cart is empty" }],
        },
      },
    },
  ],
  placeholders: [{ name: "count", kind: "icu-pivot", jsType: "number", sourceExpr: "items" }],
  properties: {
    file: "src/ShoppingCart.tsx",
    line: 4,
    component: "ShoppingCart",
    jsxPath: "ShoppingCart > p > Plural",
    element: "Plural",
  },
  preview: { sampleValues: { count: 3 } },
};

/** like-notification — a `select` construct keyed by an arbitrary pivot. */
export const likeNotification: Block = {
  id: "like-notification",
  hash: "7sVx0p",
  translatable: true,
  type: "jsx:element",
  source: [
    {
      select: {
        pivot: "gender",
        cases: {
          female: [{ text: "She liked your post" }],
          male: [{ text: "He liked your post" }],
          other: [{ text: "They liked your post" }],
        },
      },
    },
  ],
  placeholders: [
    { name: "gender", kind: "icu-pivot", jsType: "string", sourceExpr: "actor.gender" },
  ],
  properties: {
    file: "src/LikeNotification.tsx",
    line: 7,
    component: "LikeNotification",
    jsxPath: "LikeNotification > Select",
    element: "Select",
  },
};

/** email-body / email-cta — a `sub` run referencing a separate subblock. */
export const emailBody: Block = {
  id: "email-body",
  hash: "Qm3a8t",
  translatable: true,
  type: "jsx:element",
  source: [
    { text: "Thanks for signing up. " },
    { sub: { id: "1", ref: "email-cta", equiv: "cta" } },
  ],
  placeholders: [{ name: "cta", kind: "node", jsType: "ReactNode", sourceExpr: "<CallToAction/>" }],
  properties: {
    file: "src/WelcomeEmail.tsx",
    line: 12,
    component: "WelcomeEmail",
    jsxPath: "WelcomeEmail > p",
    element: "p",
  },
};

export const emailCta: Block = {
  id: "email-cta",
  hash: "Lp9w2k",
  translatable: true,
  type: "jsx:element",
  source: [{ text: "Confirm your email address" }],
  placeholders: [],
  properties: {
    file: "src/WelcomeEmail.tsx",
    line: 14,
    component: "WelcomeEmail",
    jsxPath: "WelcomeEmail > a.cta",
    element: "a",
  },
};

// ─── Envelope helper ─────────────────────────────────────────────────────

function fileWith(blocks: Block[]): File {
  return {
    schemaVersion: "1.0",
    kind: "kapi-localization-format",
    created: "2026-04-15T10:00:00Z",
    generator: {
      id: "@neokapi/kapi-format-examples",
      version: "0.0.1",
      capabilities: ["extract", "preview"],
    },
    project: { id: "neokapi-kapi-format-examples", sourceLocale: "en" },
    vocabulary: { extends: ["common-formatting", "rich-html", "rich-jsx"] },
    documents: [{ id: "examples", documentType: "jsx", path: "examples/all.tsx", blocks }],
  };
}

export interface KlfSample {
  id: string;
  label: string;
  blurb: string;
  file: File;
}

/** The KLF samples offered in the Lab's sample picker. */
export const KLF_SAMPLES: KlfSample[] = [
  {
    id: "full",
    label: "Complete document",
    blurb: "All three golden blocks: a paired code + variable, optional nodes, and a plural.",
    file: fileWith([filesHeading, tagChip, shoppingCart]),
  },
  {
    id: "files-heading",
    label: "Paired code",
    blurb: "A <span> paired code (pcOpen/pcClose) wrapping a {count} variable placeholder.",
    file: fileWith([filesHeading]),
  },
  {
    id: "tag-chip",
    label: "Optional nodes",
    blurb: "Two optional jsx:node placeholders that a target may legitimately drop.",
    file: fileWith([tagChip]),
  },
  {
    id: "plural",
    label: "Plural",
    blurb: "A structured plural run — each CLDR form holds its own Run[].",
    file: fileWith([shoppingCart]),
  },
  {
    id: "select",
    label: "Select",
    blurb: "A select construct keyed by an arbitrary pivot (male / female / other).",
    file: fileWith([likeNotification]),
  },
  {
    id: "sub",
    label: "Subblock",
    blurb: "A sub run referencing a second block extracted from an embedded subfilter.",
    file: fileWith([emailBody, emailCta]),
  },
];

export function klfSampleById(id: string): KlfSample {
  return KLF_SAMPLES.find((s) => s.id === id) ?? KLF_SAMPLES[0];
}

/** Pretty-print a File as editable `.klf` text (2-space indent, trailing LF). */
export function klfText(file: File): string {
  return `${JSON.stringify(file, null, 2)}\n`;
}

// ─── Annotation overlay (.klfl) ──────────────────────────────────────────

// A companion overlay targeting the complete-document blocks. It exercises
// every anchor kind — block, run, range, form — plus a deliberately orphaned
// run anchor (runId "99") so the Lab can show a resolution *failure* reason.
export const ANNOTATIONS_KLFL = [
  JSON.stringify({
    type: "header",
    annotationType: "@neokapi/example",
    annotationVersion: "1.0.0",
    producer: { id: "@neokapi/kapi-format-examples", version: "0.0.1" },
    created: "2026-04-15T12:00:00Z",
    targetArchive: "sha256:deadbeef",
  }),
  JSON.stringify({
    type: "annotation",
    id: "review-1",
    anchor: { kind: "block", block: "files-heading" },
    data: { kind: "review-status", status: "approved", reviewer: "jane" },
  }),
  JSON.stringify({
    type: "annotation",
    id: "term-1",
    anchor: { kind: "run", block: "tag-chip", path: [2], runId: "2" },
    data: { kind: "protected-term", term: "label", action: "preserve" },
  }),
  JSON.stringify({
    type: "annotation",
    id: "entity-1",
    anchor: { kind: "range", block: "files-heading", path: [4], offset: 1, length: 7 },
    data: { kind: "named-entity", entity: "matched" },
  }),
  JSON.stringify({
    type: "annotation",
    id: "qa-1",
    anchor: { kind: "form", block: "shopping-cart-plural", path: [0], key: "one" },
    data: { kind: "qa", note: "singular form reviewed" },
  }),
  JSON.stringify({
    type: "annotation",
    id: "orphan-1",
    anchor: { kind: "run", block: "tag-chip", path: [2], runId: "99" },
    data: { kind: "protected-term", term: "label", note: "stale runId — should not resolve" },
  }),
].join("\n");
