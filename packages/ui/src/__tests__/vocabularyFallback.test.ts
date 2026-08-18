import { describe, it, expect } from "vite-plus/test";
import { getDefaultRegistry } from "../vocabularies";
import { semanticLabel } from "../components/editor/tagSemantics";
import { unknownOpen, unknownClose } from "../stories/editor/fixtures";
import type { SpanInfo } from "../types/span";

function span(
  spanType: "opening" | "closing" | "placeholder",
  type: string,
  data: string,
): SpanInfo {
  return { span_type: spanType, type, id: "1", data };
}

// This registry is the only one in the tree: the bowrain kit re-exports it
// rather than loading a second copy of the packs. These expectations are
// mirrored by bowrain/packages/ui/src/__tests__/vocabularyRegistry.test.ts.
describe("chip labels for types no pack defines", () => {
  it("derives the label from the type name", () => {
    const registry = getDefaultRegistry();
    expect(registry.chipLabel(span("opening", "custom:unknown", "<span>"))).toBe("unknown>");
    expect(registry.chipLabel(span("closing", "custom:unknown", "</span>"))).toBe("/unknown");
    expect(registry.chipLabel(span("placeholder", "custom:unknown", "<span/>"))).toBe("unknown");
  });

  it("uses the whole type name when it carries no prefix", () => {
    expect(getDefaultRegistry().chipLabel(span("opening", "mystery", "<x>"))).toBe("mystery>");
  });

  it("keeps the pack's fallback for the other renderings", () => {
    const info = getDefaultRegistry().lookupOrFallback("custom:unknown");
    expect(info.category).toBe("generic");
    expect(info.html.open).toBe('<span data-type="custom:unknown">');
    expect(info.display.open).toBe("[custom:unknown]");
  });

  it("renders the shared story fixture the way the bowrain kit does", () => {
    expect(semanticLabel(unknownOpen)).toBe("widget>");
    expect(semanticLabel(unknownClose)).toBe("/widget");
  });
});

describe("chip labels for types the packs define", () => {
  it("reads them from the pack", () => {
    const registry = getDefaultRegistry();
    expect(registry.chipLabel(span("opening", "fmt:bold", "<b>"))).toBe("B>");
    expect(registry.chipLabel(span("closing", "fmt:bold", "</b>"))).toBe("/B");
    expect(registry.chipLabel(span("placeholder", "struct:break", "<br/>"))).toBe("br");
  });
});

// A KBF catalog extracted from a React tree carries jsx: codes. Without the JSX
// pack loaded they fell through to the fallback, which derives its label from
// the type name — so every variable in a block read "var", whichever variable it
// was, and every element read "element>".
describe("the JSX pack", () => {
  const jsxVar = (equiv: string): SpanInfo => ({
    span_type: "placeholder",
    type: "jsx:var",
    id: "1",
    data: `{${equiv}}`,
    equiv_text: equiv,
    sub_type: "string",
  });

  it("names the variable a chip stands for, not its type", () => {
    const registry = getDefaultRegistry();
    expect(registry.chipLabel(jsxVar("resetDate"))).toBe("resetDate");
    expect(registry.chipLabel(jsxVar("workspaceName"))).toBe("workspaceName");
    expect(semanticLabel(jsxVar("resetDate"))).toBe("resetDate");
  });

  it("falls back to the type's short name when the span carries no equiv", () => {
    expect(getDefaultRegistry().chipLabel(jsxVar(""))).toBe("var");
  });

  it("expands an element's markup template from the span", () => {
    const el: SpanInfo = {
      span_type: "opening",
      type: "jsx:element",
      id: "3",
      data: '<span className="x">',
      sub_type: "span",
    };
    expect(getDefaultRegistry().chipLabel(el)).toBe("span>");
    expect(getDefaultRegistry().htmlTag(el)).toBe('<span data-neokapi-span="3">');
  });

  it("reads a variable's text equivalent as its name", () => {
    expect(getDefaultRegistry().textEquiv(jsxVar("resetDate"))).toBe("resetDate");
  });
});
