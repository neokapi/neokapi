import { describe, it, expect } from "vite-plus/test";
import { getDefaultRegistry, semanticLabel } from "../index";
import { getDefaultRegistry as primitivesRegistry, type SpanInfo } from "@neokapi/ui-primitives";
import { unknownOpen, unknownClose } from "../stories/fixtures";

function span(
  spanType: "opening" | "closing" | "placeholder",
  type: string,
  data: string,
): SpanInfo {
  return { span_type: spanType, type, id: "1", data };
}

// The registry this package exports and the one backing the tagSemantics
// helpers it re-exports must be the same object. Two registries loaded from two
// copies of the same packs rendered unknown span types differently on one
// public surface: semanticLabel said "unknown>" while getDefaultRegistry()
// said "?>".
describe("vocabulary registry identity", () => {
  it("exports the @neokapi/ui-primitives registry, not a second one", () => {
    expect(getDefaultRegistry()).toBe(primitivesRegistry());
  });
});

describe("unknown span type chip labels", () => {
  it("derives a readable label from the type name", () => {
    const registry = getDefaultRegistry();
    expect(registry.chipLabel(span("opening", "custom:unknown", "<span>"))).toBe("unknown>");
    expect(registry.chipLabel(span("closing", "custom:unknown", "</span>"))).toBe("/unknown");
    expect(registry.chipLabel(span("placeholder", "custom:unknown", "<span/>"))).toBe("unknown");
  });

  it("agrees with the semanticLabel helper on the same span", () => {
    const s = span("opening", "custom:unknown", "<span>");
    expect(getDefaultRegistry().chipLabel(s)).toBe(semanticLabel(s));
  });

  it("uses the whole type name when it carries no prefix", () => {
    const registry = getDefaultRegistry();
    expect(registry.chipLabel(span("opening", "mystery", "<x>"))).toBe("mystery>");
  });

  it("still resolves known types from the packs", () => {
    const registry = getDefaultRegistry();
    expect(registry.chipLabel(span("opening", "fmt:bold", "<b>"))).toBe("B>");
    expect(registry.chipLabel(span("closing", "fmt:bold", "</b>"))).toBe("/B");
    expect(registry.chipLabel(span("placeholder", "struct:break", "<br/>"))).toBe("br");
  });

  it("renders the shared story fixture the way the kapi kit does", () => {
    expect(semanticLabel(unknownOpen)).toBe("widget>");
    expect(semanticLabel(unknownClose)).toBe("/widget");
  });
});
