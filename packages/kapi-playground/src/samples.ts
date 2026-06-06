// Curated sample library — the SINGLE source of truth for the playground demo
// material, shared by the docs CLI playground picker and the kapi-lab
// Workspace/Project explorers.
//
// Two shapes:
//   1. Loose ad-hoc files — a handful of standalone inputs you run a one-off
//      command against (pseudo-translate, word-count, extract …). These reuse
//      the kit fixtures (fixtures.ts) so the bytes are not duplicated.
//   2. Sample projects — a ready-made `.kapi` recipe + its content + a seeded
//      project TMX, so a reader can run the project funnel offline
//      (init/add/extract/run/merge) and get a real localized file via TM
//      leverage (no LLM, no network).
//
// The kapi-lab WorkspaceExplorer/ProjectExplorer historically owned this data
// in their own `workspaceSamples.ts`; that module now re-exports from here so
// there is exactly one copy of every sample byte.

import { getFixture } from "./fixtures";
import type { KapiFile } from "./store";

// ── Loose ad-hoc files ──────────────────────────────────────────────────────

/** A standalone input you run an ad-hoc command against. */
export interface LooseSample {
  id: string;
  label: string;
  /** File name written into the session cwd. */
  filename: string;
  /** Human note about the format. */
  kind: string;
  /** A representative command to suggest at the prompt. */
  suggested: string;
  /** Inline file (path + content) seeded into the cwd. */
  file: KapiFile;
}

function fixtureFile(name: string): KapiFile {
  const fx = getFixture(name);
  if (!fx) throw new Error(`unknown fixture: ${name}`);
  return { path: fx.name, content: fx.content };
}

/**
 * The curated loose files. Small, instantly legible, and drawn from the existing
 * fixtures so no sample bytes are duplicated.
 */
export const LOOSE_SAMPLES: LooseSample[] = [
  {
    id: "json",
    label: "JSON catalog",
    filename: "messages.json",
    kind: "text · JSON",
    suggested: "kapi pseudo-translate messages.json",
    file: fixtureFile("messages.json"),
  },
  {
    id: "html",
    label: "HTML page",
    filename: "page.html",
    kind: "text · HTML",
    suggested: "kapi word-count page.html",
    file: fixtureFile("page.html"),
  },
  {
    id: "xliff",
    label: "XLIFF bilingual",
    filename: "app.xliff",
    kind: "text · XLIFF 1.2",
    suggested: "kapi pseudo-translate app.xliff",
    file: fixtureFile("app.xliff"),
  },
];

// ── Sample projects (.kapi) ──────────────────────────────────────────────────

const enc = new TextEncoder();

// A minimal en→fr TMX. Each <tu> pairs a source segment with its French
// translation; the project funnel imports this into the project TM so the
// `translate` (tm-leverage) flow fills real fr targets offline.
function tmx(pairs: [string, string][]): string {
  const tus = pairs
    .map(
      ([en, fr]) =>
        `    <tu>\n      <tuv xml:lang="en"><seg>${en}</seg></tuv>\n      <tuv xml:lang="fr"><seg>${fr}</seg></tuv>\n    </tu>`,
    )
    .join("\n");
  return `<?xml version="1.0" encoding="UTF-8"?>
<tmx version="1.4">
  <header creationtool="neokapi" creationtoolversion="1.0"
          segtype="sentence" o-tmf="unknown" adminlang="en"
          srclang="en" datatype="plaintext"/>
  <body>
${tus}
  </body>
</tmx>
`;
}

/**
 * A ready-made `.kapi` sample project: a recipe + its content file(s) + a seeded
 * project TMX. Seeding all three into the cwd lets a reader run the offline
 * project funnel (`kapi add` / `kapi extract` / `kapi run translate` /
 * `kapi merge`) and produce a genuine localized file via TM leverage.
 */
export interface ProjectSample {
  id: string;
  label: string;
  /** One-line description for the picker. */
  description: string;
  /** The recipe file name (e.g. "demo.kapi"). */
  recipeName: string;
  /** The primary content file's name (for suggested commands). */
  contentName: string;
  /** Per-sample binary helpers seeded into the cwd. */
  binary: boolean;
  /** Bytes for a binary content file (Office), if any. */
  contentBytes?: () => Uint8Array;
  /** The committed project files (recipe + text content + TMX). */
  files: KapiFile[];
}

function bytesFromBase64(b64: string): Uint8Array {
  const bin = atob(b64);
  const out = new Uint8Array(bin.length);
  for (let i = 0; i < bin.length; i++) out[i] = bin.charCodeAt(i);
  return out;
}

const JSON_PROJECT_CONTENT = `{
  "greeting": "Welcome to Acme",
  "cta": "Sign up today",
  "farewell": "Talk soon"
}
`;

const JSON_PROJECT_TMX = tmx([
  ["Welcome to Acme", "Bienvenue chez Acme"],
  ["Sign up today", "Inscrivez-vous aujourd'hui"],
  ["Talk soon", "À bientôt"],
]);

// Matches the translatable text baked into DOCX_B64's word/document.xml.
const DOCX_PROJECT_TMX = tmx([
  ["Welcome to Acme", "Bienvenue chez Acme"],
  ["Your account is ready.", "Votre compte est prêt."],
  ["Sign in to continue", "Connectez-vous pour continuer"],
]);

// Matches the shared strings baked into XLSX_B64's xl/sharedStrings.xml.
const XLSX_PROJECT_TMX = tmx([
  ["Total revenue", "Chiffre d'affaires total"],
  ["Net profit", "Bénéfice net"],
]);

// Tiny, hand-built OOXML packages base64-embedded so the sample projects are
// self-contained (no network). Shared with the kapi-lab explorers.
export const DOCX_B64 =
  "UEsDBBQAAAAIAOm6xFzmdcR+0gAAAIsBAAATAAAAW0NvbnRlbnRfVHlwZXNdLnhtbH2QvVLDMAzHX8XnlasVGBh6SToAKzD0BXSOkvjw11luad++Sls6cIVR+n/8ZLebQ/BqT4Vdip1+NI3e9O32mImVKJE7Pdea1wBsZwrIJmWKooypBKwylgky2i+cCJ6a5hlsipViXdWlQ/ftK42481W9HWR9oRTyrNXLxbiwOo05e2exig77OPyirK4EI8mzh2eX+UEMGu4SFuVvwDX3Ic8ubiD1iaW+YxAXfKcywJDsLkjS/F9z5840js7SLb+05ZIsMbs4BW9uSkAXf+6H83f3J1BLAwQUAAAACADpusRcXzOVUpUAAAAHAQAACwAAAF9yZWxzLy5yZWxzjc87DsIwDAbgq0Q+QJ0yMKCmXVi6Ii4QJW5T0TzkhNftycBAEQOjf//6LHfDw6/iRpyXGBS0jYSh70606lKD7JaURW2ErMCVkg6I2TjyOjcxUaibKbLXpY48Y9LmomfCnZR75E8DtqYYrQIebQvi/Ez0jx2naTF0jObqKZQfJ74aVdY8U1Fwj2zRvuOmsoB9h5sX+xdQSwMEFAAAAAgA6brEXO2a4OirAAAAIQEAABEAAAB3b3JkL2RvY3VtZW50LnhtbIWPSwrCMBCGrxJygKa6cFH6wDO4EJcxHdtAMxMmibW3NxFEEMHNN8zz/6cdHm4Rd+BgCTu5q2o59O3ajGSSA4witzE0ayfnGH2jVDAzOB0q8oC5dyN2OuaUJ7USj57JQAgWJ7eofV0flNMWZTl5pXEr0RdwQezPsBhyICKJo3HQqlIs5Bf99/yFEgttDKVszQbBoMet+rt2shMKi0XGEEaL6ZeUentUn//7J1BLAQIUAxQAAAAIAOm6xFzmdcR+0gAAAIsBAAATAAAAAAAAAAAAAACAAQAAAABbQ29udGVudF9UeXBlc10ueG1sUEsBAhQDFAAAAAgA6brEXF8zlVKVAAAABwEAAAsAAAAAAAAAAAAAAIABAwEAAF9yZWxzLy5yZWxzUEsBAhQDFAAAAAgA6brEXO2a4OirAAAAIQEAABEAAAAAAAAAAAAAAIABwQEAAHdvcmQvZG9jdW1lbnQueG1sUEsFBgAAAAADAAMAuQAAAJsCAAAAAA==";
export const XLSX_B64 =
  "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";

/** The committed YAML recipe for a project sample. */
function recipeYaml(opts: { content: string; format: string; target: string }): string {
  return `version: v1
name: demo
defaults:
  source_language: en
  target_languages: [fr]
content:
  - path: ${opts.content}
    format: ${opts.format}
    target: "${opts.target}"
flows:
  translate:
    steps:
      - tool: tm-leverage
  translate-exact:
    steps:
      - tool: tm-leverage
        config:
          fillTargetThreshold: 100
`;
}

/**
 * The curated sample projects. The JSON project is fully text (recipe + content
 * + TMX all inline). The Office project ships a tiny binary .docx whose bytes
 * are seeded separately.
 */
export const PROJECT_SAMPLES: ProjectSample[] = [
  {
    id: "json",
    label: "JSON catalog project",
    description: "A .kapi recipe over a JSON catalog with a seeded en→fr TM.",
    recipeName: "demo.kapi",
    contentName: "messages.json",
    binary: false,
    files: [
      { path: "messages.json", content: JSON_PROJECT_CONTENT },
      { path: "project.tmx", content: JSON_PROJECT_TMX },
      {
        path: "demo.kapi",
        content: recipeYaml({
          content: "messages.json",
          format: "json",
          target: "out/{lang}/messages.json",
        }),
      },
    ],
  },
  {
    id: "docx",
    label: "Word document project",
    description: "A .kapi recipe over a Word (.docx) file with a seeded en→fr TM.",
    recipeName: "demo.kapi",
    contentName: "welcome.docx",
    binary: true,
    contentBytes: () => bytesFromBase64(DOCX_B64),
    files: [
      { path: "project.tmx", content: DOCX_PROJECT_TMX },
      {
        path: "demo.kapi",
        content: recipeYaml({
          content: "welcome.docx",
          format: "openxml",
          target: "out/{lang}/welcome.docx",
        }),
      },
    ],
  },
];

export function projectSampleById(id: string): ProjectSample {
  return PROJECT_SAMPLES.find((s) => s.id === id) ?? PROJECT_SAMPLES[0];
}

// ── Back-compat: the WorkspaceSample shape used by kapi-lab explorers ────────
//
// kapi-lab's WorkspaceExplorer/ProjectExplorer (and their tests + stories)
// consume a flat `WorkspaceSample` with `bytes()` + `tmx`. We synthesize that
// from the same source data here so there is exactly one copy of every byte;
// kapi-lab's `workspaceSamples.ts` re-exports these.

export interface WorkspaceSample {
  id: string;
  label: string;
  filename: string;
  /** Human note about the format. */
  kind: string;
  /** Bytes seeded into the in-memory filesystem. */
  bytes: () => Uint8Array;
  /** True for binary formats (don't render the raw bytes as text). */
  binary: boolean;
  /**
   * A project TMX (en→fr) whose source segments match this sample's
   * translatable text, so `kapi tm import` + the `translate` (tm-leverage) flow
   * fill real `fr` targets offline — no LLM.
   */
  tmx: string;
}

function tmxOf(files: KapiFile[]): string {
  return files.find((f) => f.path === "project.tmx")?.content ?? "";
}

export const WORKSPACE_SAMPLES: WorkspaceSample[] = [
  {
    id: "json",
    label: "JSON catalog",
    filename: "messages.json",
    kind: "text · JSON",
    bytes: () => enc.encode(JSON_PROJECT_CONTENT),
    binary: false,
    tmx: JSON_PROJECT_TMX,
  },
  {
    id: "docx",
    label: "Word document",
    filename: "welcome.docx",
    kind: "binary · OOXML (.docx)",
    bytes: () => bytesFromBase64(DOCX_B64),
    binary: true,
    tmx: DOCX_PROJECT_TMX,
  },
  {
    id: "xlsx",
    label: "Excel sheet",
    filename: "report.xlsx",
    kind: "binary · OOXML (.xlsx)",
    bytes: () => bytesFromBase64(XLSX_B64),
    binary: true,
    tmx: XLSX_PROJECT_TMX,
  },
];

export function workspaceSampleById(id: string): WorkspaceSample {
  return WORKSPACE_SAMPLES.find((s) => s.id === id) ?? WORKSPACE_SAMPLES[0];
}

// ── Hero samples (drop-a-file widgets) ───────────────────────────────────────
//
// The minimal corpus the docs landing hero (and the reusable ToolDropWidget)
// offers as "try a sample" chips. Each carries its own bytes so a widget needs
// no file-library seeding — a learner drops their own file or picks one of these.
//
// To add a richer .docx or a real .pptx later: base64-encode a tiny valid OOXML
// package and add an entry below (mirror the docx entry). Do NOT fabricate a
// pptx skeleton — encode a genuine PowerPoint export so the openxml reader has
// real slide text to extract. The hero's tool switcher and ToolDropWidget pick
// these up automatically by id.

export interface HeroSample {
  id: string;
  label: string;
  /** File name written into the in-memory filesystem (extension drives detect). */
  filename: string;
  /** Short human note about the format, shown under the chip. */
  kind: string;
  /** True for binary OOXML packages (no text source view). */
  binary: boolean;
  /** The sample's bytes. */
  bytes: () => Uint8Array;
}

// A standalone JSON catalog for the hero widgets. It deliberately contains a US
// spelling ("color") so the default search/replace demo (color → colour) shows a
// visible change, and natural marketing copy that pseudo-translation expands
// legibly. Kept separate from JSON_PROJECT_CONTENT so changing it can't shift the
// project explorers' seeded TM.
const HERO_JSON_CONTENT = `{
  "greeting": "Welcome to Acme",
  "cta": "Sign up today",
  "theme": "Pick your favorite color",
  "farewell": "Talk soon"
}
`;

export const HERO_SAMPLES: HeroSample[] = [
  {
    id: "json",
    label: "messages.json",
    filename: "messages.json",
    kind: "text · JSON catalog",
    binary: false,
    bytes: () => enc.encode(HERO_JSON_CONTENT),
  },
  {
    id: "docx",
    label: "welcome.docx",
    filename: "welcome.docx",
    kind: "binary · Word (.docx)",
    binary: true,
    bytes: () => bytesFromBase64(DOCX_B64),
  },
  // TODO: add a real .pptx here once a tiny valid PowerPoint package is
  // base64-encoded (see the note above). Example shape:
  //   { id: "pptx", label: "deck.pptx", filename: "deck.pptx",
  //     kind: "binary · PowerPoint (.pptx)", binary: true,
  //     bytes: () => bytesFromBase64(PPTX_B64) },
];

// ── Try-Neokapi showcase samples (real downloadable Office/text files) ───────
//
// Real, valid documents that contain the showcase term "Acme" so the docs
// landing "Try Neokapi in your browser" modal can offer a genuine
// "Download source" plus a "Download result" that runs the REAL wasm engine
// (search-replace Acme → Globex) and downloads the transformed file. A visitor
// can open the result in Excel / PowerPoint / any editor and confirm neokapi
// round-trips real files. Generated by scripts/gen-tryneokapi-samples.py and
// verified to round-trip via `kapi run` with an inline search-replace recipe.
//
// These are separate from HERO_SAMPLES (the old drop-a-file hero corpus): the
// modal's faked visual showcase needs no bytes at all — only this real proof
// path does.
export const TRY_XLSX_B64 =
  "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";
export const TRY_PPTX_B64 =
  "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";
export const TRY_MD_B64 =
  "IyBXZWxjb21lIHRvIEFjbWUKCkFjbWUgaGVscHMgdGVhbXMgc2hpcCBmYXN0ZXIuIFBpY2sgeW91ciBmYXZvcml0ZSBjb2xvciBhbmQgZ2V0IHN0YXJ0ZWQuCgotIFNpZ24gdXAgZm9yIEFjbWUgdG9kYXkKLSBUYWxrIHRvIHRoZSBBY21lIHRlYW0gc29vbgo=";

export interface TrySample {
  id: string;
  /** File name written into the in-memory filesystem (extension drives detect). */
  filename: string;
  /** Short human label shown on the download buttons. */
  label: string;
  /** The kapi format id used when running the engine (and for detection notes). */
  format: string;
  /** The sample's source bytes. */
  bytes: () => Uint8Array;
}

// The three real downloadable sources, one per showcase document kind. The
// modal pairs each with a "Download result" that runs search-replace in wasm.
export const TRY_SAMPLES: TrySample[] = [
  {
    id: "pptx",
    filename: "deck.pptx",
    label: "deck.pptx",
    format: "openxml",
    bytes: () => bytesFromBase64(TRY_PPTX_B64),
  },
  {
    id: "xlsx",
    filename: "report.xlsx",
    label: "report.xlsx",
    format: "openxml",
    bytes: () => bytesFromBase64(TRY_XLSX_B64),
  },
  {
    id: "md",
    filename: "guide.md",
    label: "guide.md",
    format: "markdown",
    bytes: () => bytesFromBase64(TRY_MD_B64),
  },
];

export function trySampleById(id: string): TrySample {
  return TRY_SAMPLES.find((s) => s.id === id) ?? TRY_SAMPLES[0];
}

export function heroSampleById(id: string): HeroSample {
  return HERO_SAMPLES.find((s) => s.id === id) ?? HERO_SAMPLES[0];
}

export { JSON_PROJECT_CONTENT as JSON_SAMPLE, tmxOf };
