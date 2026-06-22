// Shared fixtures for the PreviewKit stories. These mirror the real `kapi
// inspect` ContentTree shapes (verified against the engine) for every structural
// family FormatPreview targets, each with a fr-FR target and stand-off overlays
// (terms / entities / qa / brand-voice) so the annotation highlighting,
// source↔target toggle and transitions can be exercised without booting WASM.
import type { ContentNode, ContentTree, OverlayView, Run } from "@neokapi/ui-primitives/preview";

function txt(s: string): Run[] {
  return [{ text: s }];
}

function block(
  id: string,
  type: string,
  source: string,
  extra: Partial<ContentNode> = {},
): ContentNode {
  return { kind: "block", id, type, source: txt(source), ...extra };
}

function layer(name: string, children: ContentNode[]): ContentNode {
  return { kind: "layer", id: name, name, children };
}

export function tree(format: string, root: ContentNode[]): ContentTree {
  return { format, root, stats: { layers: 0, groups: 0, blocks: 0, data: 0, media: 0, runs: 0 } };
}

function term(text: string, side = "source", props: Record<string, string> = {}): OverlayView {
  return {
    type: "term",
    side,
    spans: [{ id: "t-" + text, range: zero(), text, props: { match: "exact", ...props } }],
  };
}
function entity(text: string, side = "source"): OverlayView {
  return {
    type: "entity",
    side,
    spans: [{ id: "e-" + text, range: zero(), text, props: { kind: "ORG" } }],
  };
}
function qa(text: string, side: string, rule: string): OverlayView {
  return {
    type: "qa",
    side,
    spans: [{ id: "q-" + text, range: zero(), text, props: { rule, severity: "warning" } }],
  };
}
function brand(text: string, side = "source"): OverlayView {
  // Brand-voice violations ride on the `qa` overlay type, distinguished by
  // props.category="brand-vocabulary" (see preview/overlayHighlight.ts).
  return {
    type: "qa",
    side,
    spans: [
      {
        id: "b-" + text,
        range: zero(),
        text,
        props: { category: "brand-vocabulary", rule: "preferred-term", severity: "warning" },
      },
    ],
  };
}
function zero() {
  return { startRun: 0, startOffset: 0, endRun: 0, endOffset: 0 };
}

// ── PPTX deck ────────────────────────────────────────────────────────────────
export const pptxTree = tree("openxml", [
  layer("/tmp/deck.pptx", [
    layer("ppt/slides/slide1.xml", [
      block("s1", "paragraph", "Welcome to Acme", {
        targets: { "fr-FR": txt("Bienvenue chez Acme") },
        overlays: [term("Acme"), term("Acme", "fr-FR")],
      }),
      block("s2", "paragraph", "Acme makes every quarter count.", {
        targets: { "fr-FR": txt("Acme fait compter chaque trimestre.") },
        overlays: [entity("Acme")],
      }),
      block("s3", "paragraph", "Sign up for Acme today", {
        targets: { "fr-FR": txt("Inscrivez-vous chez Acme dès aujourd'hui") },
        overlays: [brand("Sign up")],
      }),
    ]),
    layer("ppt/slideMasters/slideMaster1.xml", [
      block("m1", "paragraph", "Click to edit Master title style", {}),
    ]),
  ]),
]);

// ── XLSX grid ────────────────────────────────────────────────────────────────
export const xlsxTree = tree("openxml", [
  layer("/tmp/report.xlsx", [
    layer("xl/worksheets/sheet1.xml", [
      block("c1", "cell", "Acme quarterly revenue", {
        properties: { cell: "A1" },
        targets: { "fr-FR": txt("Chiffre d'affaires trimestriel Acme") },
        overlays: [term("Acme")],
      }),
      block("c2", "cell", "Total revenue", {
        properties: { cell: "B1" },
        targets: { "fr-FR": txt("Chiffre d'affaires total") },
      }),
      block("c3", "cell", "Acme net profit", {
        properties: { cell: "A2" },
        targets: { "fr-FR": txt("Bénéfice net Acme") },
      }),
      block("c4", "cell", "Net profit", {
        properties: { cell: "B2" },
        targets: { "fr-FR": txt("Bénéfice net") },
      }),
    ]),
  ]),
]);

// ── DOCX page ────────────────────────────────────────────────────────────────
export const docxTree = tree("openxml", [
  layer("/tmp/welcome.docx", [
    layer("word/document.xml", [
      block("d1", "paragraph", "Welcome to Acme", {
        targets: { "fr-FR": txt("Bienvenue chez Acme") },
        overlays: [term("Acme")],
      }),
      block("d2", "paragraph", "Your Acme account is ready. Sign up to get started.", {
        targets: { "fr-FR": txt("Votre compte Acme est prêt. Inscrivez-vous pour commencer.") },
        overlays: [entity("Acme"), brand("Sign up")],
      }),
    ]),
  ]),
]);

// ── Markdown doc ─────────────────────────────────────────────────────────────
export const mdTree = tree("markdown", [
  layer("/tmp/guide.md", [
    block("h1", "heading", "Welcome to Acme", {
      targets: { "fr-FR": txt("Bienvenue chez Acme") },
      overlays: [term("Acme")],
    }),
    block("p1", "", "Acme helps teams ship localized content faster.", {
      targets: { "fr-FR": txt("Acme aide les équipes à livrer du contenu localisé plus vite.") },
    }),
    block("li1", "list-item", "Sign up for Acme today", {
      targets: { "fr-FR": txt("Inscrivez-vous chez Acme dès aujourd'hui") },
      overlays: [brand("Sign up")],
    }),
  ]),
]);

// ── JSON entry list ──────────────────────────────────────────────────────────
export const jsonTree = tree("json", [
  layer("/tmp/messages.json", [
    block("greeting", "json:value", "Hello, Acme", {
      properties: { path: "$.greeting" },
      targets: { "fr-FR": txt("Bonjour, Acme") },
      overlays: [term("Acme")],
    }),
    block("cart.empty", "json:value", "Your cart is empty", {
      properties: { path: "$.cart.empty" },
      targets: { "fr-FR": txt("Votre panier est vide") },
    }),
    block("checkout", "json:value", "Proceed to checkout", {
      properties: { path: "$.checkout" },
      targets: { "fr-FR": txt("Passer à la caisse") },
      overlays: [qa("caisse", "fr-FR", "terminology")],
    }),
  ]),
]);

// ── PDF pages ────────────────────────────────────────────────────────────────
export const pdfTree = tree("pdf", [
  layer("/tmp/brochure.pdf", [
    block("pg1h", "heading", "Acme Annual Report", {
      properties: { page: "1" },
      targets: { "fr-FR": txt("Rapport annuel Acme") },
      overlays: [term("Acme")],
    }),
    block("pg1b", "paragraph", "A year of growth for every Acme customer.", {
      properties: { page: "1" },
      targets: { "fr-FR": txt("Une année de croissance pour chaque client Acme.") },
    }),
    block("pg2h", "heading", "Financial Highlights", {
      properties: { page: "2" },
      targets: { "fr-FR": txt("Points financiers clés") },
    }),
  ]),
]);

// ── Generic fallback (unknown format, nested containers) ──────────────────────
export const genericTree = tree("acme-config", [
  layer("/tmp/app.acme", [
    block("title", "", "Acme Settings", {}),
    {
      kind: "group",
      id: "notifications",
      name: "notifications",
      children: [
        block("n1", "", "Email me about updates", {}),
        block("n2", "", "Send weekly digest", {}),
      ],
    },
  ]),
]);

// ── DocLang / Docling (the WS1 structural layer: roles + page geometry) ───────
// A single page carrying the structural layer the editor's Structure (outline)
// and Layout (spatial) views render: semantic roles + level, a furniture
// running header, a list group, a table, and per-block bounding boxes on a
// 512-grid (DocLang's default resolution, top-left origin).
function geo(x: number, y: number, w: number, h: number) {
  return { page: 1, x, y, w, h, resolution: 512, origin: "top-left" };
}

export const doclangTree = tree("doclang", [
  layer("report.dclg.xml", [
    block("ph", "page_header", "Confidential — Q1", {
      structure: { role: "page-header", layer: "furniture" },
      geometry: geo(40, 16, 200, 18),
    }),
    block("title", "title", "Annual Report", {
      structure: { role: "title" },
      targets: { "fr-FR": txt("Rapport annuel") },
      geometry: geo(72, 60, 380, 36),
    }),
    block("h1", "heading", "Overview", {
      structure: { role: "heading", level: 2 },
      targets: { "fr-FR": txt("Aperçu") },
      geometry: geo(72, 120, 200, 22),
    }),
    block("p1", "text", "A year of steady growth across every region.", {
      structure: { role: "paragraph" },
      targets: { "fr-FR": txt("Une année de croissance régulière dans toutes les régions.") },
      geometry: geo(72, 152, 380, 40),
    }),
    {
      kind: "group",
      id: "list1",
      name: "list",
      type: "list",
      children: [
        block("li1", "text", "Revenue up 12%", {
          structure: { role: "list-item" },
          geometry: geo(96, 204, 300, 18),
        }),
        block("li2", "text", "Two new markets", {
          structure: { role: "list-item" },
          geometry: geo(96, 226, 300, 18),
        }),
      ],
    },
    {
      kind: "group",
      id: "tbl1",
      name: "table",
      type: "table",
      children: [
        {
          kind: "group",
          id: "tr1",
          name: "table-row",
          type: "table-row",
          children: [
            block("c1", "table-cell", "Region", {
              structure: { role: "table-header" },
              geometry: geo(72, 264, 150, 20),
            }),
            block("c2", "table-cell", "Sales", {
              structure: { role: "table-header" },
              geometry: geo(222, 264, 150, 20),
            }),
          ],
        },
        {
          kind: "group",
          id: "tr2",
          name: "table-row",
          type: "table-row",
          children: [
            block("c3", "table-cell", "EU", {
              structure: { role: "table-cell" },
              geometry: geo(72, 284, 150, 20),
            }),
            block("c4", "table-cell", "100", {
              structure: { role: "table-cell" },
              geometry: geo(222, 284, 150, 20),
            }),
          ],
        },
      ],
    },
  ]),
]);

export const ALL_TREES: { filename: string; tree: ContentTree }[] = [
  { filename: "deck.pptx", tree: pptxTree },
  { filename: "report.xlsx", tree: xlsxTree },
  { filename: "welcome.docx", tree: docxTree },
  { filename: "guide.md", tree: mdTree },
  { filename: "messages.json", tree: jsonTree },
  { filename: "brochure.pdf", tree: pdfTree },
  { filename: "report.dclg.xml", tree: doclangTree },
  { filename: "app.acme", tree: genericTree },
];
