// @vitest-environment jsdom
//
// The two rendering contracts the preview owes a document: a code block must
// render as code (block, monospace, whitespace preserved, markdown markers
// literal), and target text must carry its locale's writing direction.
import { describe, it, expect } from "vitest";
import { createElement } from "react";
import { createRoot } from "react-dom/client";
import { act } from "react";

import FormatPreview from "../components/preview/FormatPreview";
import { treeToRenderDoc } from "../components/preview/renderDoc";
import type { ContentNode, ContentTree } from "../components/preview/types";

function render(tree: ContentTree, side = "source"): HTMLDivElement {
  const container = document.createElement("div");
  document.body.appendChild(container);
  act(() => {
    createRoot(container).render(
      createElement(FormatPreview, { tree, side, transition: "none" as const }),
    );
  });
  return container;
}

function block(id: string, text: string, extra: Partial<ContentNode> = {}): ContentNode {
  return { kind: "block", id, source: [{ text }], ...extra };
}

/** A markdown document: prose, a fenced code block, more prose. */
function markdownTree(): ContentTree {
  return {
    format: "markdown",
    stats: {},
    root: [
      block("b1", "Install the CLI", { type: "heading", sourceLocale: "en" }),
      block("b2", 'func main() {\n\tfmt.Println("*not emphasis*")\n}', {
        type: "code-block",
        translatable: false,
        preserveWhitespace: true,
        sourceLocale: "en",
        structure: { role: "code" },
        properties: { "code.language": "go" },
      }),
      block("b3", "Then run it.", { type: "paragraph", sourceLocale: "en" }),
    ],
  } as ContentTree;
}

describe("preview — code blocks", () => {
  it("gives a code block a code role in the render model", () => {
    const doc = treeToRenderDoc(markdownTree());
    expect(doc.kind).toBe("doc");
    const roles = doc.paragraphs?.map((p) => p.role);
    expect(roles).toEqual(["heading", "code", "body"]);
    const code = doc.paragraphs?.[1];
    expect(code?.preserveWhitespace).toBe(true);
    expect(code?.codeLanguage).toBe("go");
  });

  it("renders it as <pre><code>, not as a paragraph", () => {
    const c = render(markdownTree());
    const pre = c.querySelector("pre");
    expect(pre).not.toBeNull();
    expect(pre?.querySelector("code")).not.toBeNull();
    expect(pre?.getAttribute("data-code-language")).toBe("go");
    // The prose blocks stay paragraphs; only the code block is a <pre>.
    expect(c.querySelectorAll("pre")).toHaveLength(1);
  });

  it("keeps newlines, indentation, and markdown markers literal inside code", () => {
    const c = render(markdownTree());
    const text = c.querySelector("pre")?.textContent ?? "";
    expect(text).toContain("\n");
    expect(text).toContain("\t");
    // `*not emphasis*` must survive verbatim — no markdown interpretation, no
    // smart-quote substitution.
    expect(text).toContain('fmt.Println("*not emphasis*")');
    expect(c.querySelector("pre em")).toBeNull();
    expect(c.querySelector("pre strong")).toBeNull();
  });

  it("lays code out left-to-right even in a right-to-left document", () => {
    const tree = markdownTree();
    for (const n of tree.root) n.sourceLocale = "ar";
    const c = render(tree);
    expect(c.querySelector("pre")?.getAttribute("dir")).toBe("ltr");
    // …while the prose around it is RTL.
    expect(c.querySelector("p")?.getAttribute("dir")).toBe("rtl");
  });

  it("preserves whitespace for any block the engine flagged, not only code", () => {
    const tree: ContentTree = {
      format: "markdown",
      stats: {},
      root: [
        block("b0", "Title", { type: "heading" }),
        block("b1", "line one\nline two", { type: "paragraph", preserveWhitespace: true }),
      ],
    } as ContentTree;
    const c = render(tree);
    const p = c.querySelector("p");
    expect(p?.textContent).toContain("line one");
    expect(p?.className).toMatch(/preserve/);
  });
});

describe("preview — writing direction", () => {
  const bilingual: ContentTree = {
    format: "json",
    stats: {},
    root: [
      {
        kind: "block",
        id: "b1",
        name: "app.title",
        sourceLocale: "en",
        source: [{ text: "Welcome to Kapi" }],
        targets: {
          ar: [{ text: "مرحبا بك في Kapi" }],
          he: [{ text: "ברוכים הבאים ל-Kapi" }],
          nb: [{ text: "Velkommen til Kapi" }],
        },
      },
    ],
  } as ContentTree;

  it("renders an LTR source as ltr", () => {
    const c = render(bilingual, "source");
    const text = c.querySelector("[lang]");
    expect(text?.getAttribute("dir")).toBe("ltr");
    expect(text?.getAttribute("lang")).toBe("en");
    expect(text?.textContent).toBe("Welcome to Kapi");
  });

  it("renders an Arabic target as rtl, tagged with its locale", () => {
    const c = render(bilingual, "ar");
    const rtl = c.querySelector('[dir="rtl"]');
    expect(rtl).not.toBeNull();
    expect(rtl?.getAttribute("lang")).toBe("ar");
    expect(rtl?.textContent).toContain("مرحبا");
  });

  it("renders a Hebrew target as rtl", () => {
    const c = render(bilingual, "he");
    expect(c.querySelector('[dir="rtl"]')?.getAttribute("lang")).toBe("he");
  });

  it("keeps an LTR target ltr", () => {
    const c = render(bilingual, "nb");
    expect(c.querySelector('[dir="rtl"]')).toBeNull();
  });

  it("isolates the catalog key from an RTL value", () => {
    const c = render(bilingual, "ar");
    const key = c.querySelector("bdi");
    // <bdi> is unicode-bidi: isolate by default; the explicit dir pins it LTR so
    // an identifier never renders mirrored beside its Arabic value.
    expect(key?.getAttribute("dir")).toBe("ltr");
    expect(key?.textContent).toBe("app.title");
  });

  it("carries the source locale onto the render model", () => {
    expect(treeToRenderDoc(bilingual).sourceLocale).toBe("en");
  });
});
