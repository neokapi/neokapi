// Baked render models for the hero Format-carousel. These are STATIC data so the
// hero is instant and pulls ZERO wasm on page load — the engine only boots when
// the reader opens the full modal.
//
// The structure is FAITHFUL to the real extraction (verified against the engine
// via `kapi inspect` on the modal's TRY_SAMPLES): a pptx slide is one
// ppt/slides/slide1.xml layer whose first paragraph is the title and the rest
// are bullets; an xlsx sheet is xl/worksheets/sheet1.xml with cells placed by
// A1-style refs; a markdown doc is a heading + paragraph + list items. The
// block ids (p1, x1, m1, …) mirror the engine's tuN ordering so the EN→FR diff
// highlights the same spans a live run would. The FR text is hand-translated for
// a clean teaser (the modal shows the real engine output).

import type { RenderDoc } from "@neokapi/kapi-lab/renderDoc";

export interface HeroSlide {
  id: "slide" | "sheet" | "doc";
  /** Dot/label shown in the carousel rail. */
  label: string;
  /** File-format chrome label (e.g. "deck.pptx"). */
  filename: string;
  source: RenderDoc;
  target: RenderDoc;
}

// ── Slide (PowerPoint) ───────────────────────────────────────────────────────

const slideSource: RenderDoc = {
  kind: "slides",
  format: "openxml",
  slides: [
    {
      name: "ppt/slides/slide1.xml",
      title: { id: "p1", text: "Welcome to Acme", role: "title" },
      bullets: [
        { id: "p2", text: "Acme makes every quarter count.", role: "bullet" },
        { id: "p3", text: "Sign up for Acme today", role: "bullet" },
        { id: "p4", text: "Talk to the Acme team soon", role: "bullet" },
      ],
    },
  ],
};

const slideTarget: RenderDoc = {
  kind: "slides",
  format: "openxml",
  slides: [
    {
      name: "ppt/slides/slide1.xml",
      title: { id: "p1", text: "Bienvenue chez Acme", role: "title" },
      bullets: [
        { id: "p2", text: "Acme fait compter chaque trimestre.", role: "bullet" },
        { id: "p3", text: "Inscrivez-vous chez Acme dès aujourd'hui", role: "bullet" },
        { id: "p4", text: "Parlez vite à l'équipe Acme", role: "bullet" },
      ],
    },
  ],
};

// ── Sheet (Excel) ────────────────────────────────────────────────────────────

const sheetSource: RenderDoc = {
  kind: "sheet",
  format: "openxml",
  sheet: {
    name: "xl/worksheets/sheet1.xml",
    cols: 2,
    rows: 3,
    cells: [
      { id: "x1", col: 0, row: 0, ref: "A1", text: "Acme quarterly revenue" },
      { id: "x2", col: 1, row: 0, ref: "B1", text: "Total revenue" },
      { id: "x3", col: 0, row: 1, ref: "A2", text: "Acme net profit" },
      { id: "x4", col: 1, row: 1, ref: "B2", text: "Net profit" },
      { id: "x5", col: 0, row: 2, ref: "A3", text: "Acme customer count" },
      { id: "x6", col: 1, row: 2, ref: "B3", text: "Active accounts" },
    ],
  },
};

const sheetTarget: RenderDoc = {
  kind: "sheet",
  format: "openxml",
  sheet: {
    name: "xl/worksheets/sheet1.xml",
    cols: 2,
    rows: 3,
    cells: [
      { id: "x1", col: 0, row: 0, ref: "A1", text: "Chiffre d'affaires trimestriel Acme" },
      { id: "x2", col: 1, row: 0, ref: "B1", text: "Chiffre d'affaires total" },
      { id: "x3", col: 0, row: 1, ref: "A2", text: "Bénéfice net Acme" },
      { id: "x4", col: 1, row: 1, ref: "B2", text: "Bénéfice net" },
      { id: "x5", col: 0, row: 2, ref: "A3", text: "Nombre de clients Acme" },
      { id: "x6", col: 1, row: 2, ref: "B3", text: "Comptes actifs" },
    ],
  },
};

// ── Doc (Markdown) ───────────────────────────────────────────────────────────

const docSource: RenderDoc = {
  kind: "doc",
  format: "markdown",
  paragraphs: [
    { id: "m1", text: "Welcome to Acme", role: "heading" },
    { id: "m2", text: "Acme helps teams ship faster.", role: "body" },
    { id: "m3", text: "Sign up for Acme today", role: "bullet" },
    { id: "m4", text: "Talk to the Acme team soon", role: "bullet" },
  ],
};

const docTarget: RenderDoc = {
  kind: "doc",
  format: "markdown",
  paragraphs: [
    { id: "m1", text: "Bienvenue chez Acme", role: "heading" },
    { id: "m2", text: "Acme aide les équipes à livrer plus vite.", role: "body" },
    { id: "m3", text: "Inscrivez-vous chez Acme dès aujourd'hui", role: "bullet" },
    { id: "m4", text: "Parlez vite à l'équipe Acme", role: "bullet" },
  ],
};

export const HERO_CAROUSEL: HeroSlide[] = [
  { id: "slide", label: "Slide", filename: "deck.pptx", source: slideSource, target: slideTarget },
  {
    id: "sheet",
    label: "Sheet",
    filename: "report.xlsx",
    source: sheetSource,
    target: sheetTarget,
  },
  { id: "doc", label: "Doc", filename: "guide.md", source: docSource, target: docTarget },
];
