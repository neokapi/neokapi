import { describe, it, expect } from "vite-plus/test";
import { resolveConstraints, isDeletable, isCloneable } from "../components/editor/tagConstraints";
import type { SpanInfo } from "../types/api";

function span(type: string, overrides?: Partial<SpanInfo>): SpanInfo {
  return { span_type: "placeholder", type, id: "1", data: "<x/>", ...overrides };
}

describe("resolveConstraints", () => {
  it("returns all-true for fmt:bold (formatting)", () => {
    const c = resolveConstraints(span("fmt:bold", { span_type: "opening", data: "<b>" }));
    expect(c.deletable).toBe(true);
    expect(c.cloneable).toBe(true);
    expect(c.reorderable).toBe(true);
  });

  it("returns all-false for struct:break (structure)", () => {
    const c = resolveConstraints(span("struct:break", { data: "<br/>" }));
    expect(c.deletable).toBe(false);
    expect(c.cloneable).toBe(false);
    expect(c.reorderable).toBe(false);
  });

  it("returns deletable=false, cloneable=false, reorderable=true for code:variable", () => {
    const c = resolveConstraints(span("code:variable"));
    expect(c.deletable).toBe(false);
    expect(c.cloneable).toBe(false);
    expect(c.reorderable).toBe(true);
  });

  it("falls back to all-true for unknown types", () => {
    const c = resolveConstraints(span("custom:unknown"));
    expect(c.deletable).toBe(true);
    expect(c.cloneable).toBe(true);
    expect(c.reorderable).toBe(true);
  });

  it("explicit SpanInfo overrides vocabulary defaults", () => {
    // struct:break is normally non-deletable, but SpanInfo says deletable
    const c = resolveConstraints(span("struct:break", { deletable: true }));
    expect(c.deletable).toBe(true);
    // cloneable not overridden — still false from vocabulary
    expect(c.cloneable).toBe(false);
  });

  it("explicit false overrides vocabulary true", () => {
    // fmt:bold is normally cloneable, but SpanInfo says not
    const c = resolveConstraints(
      span("fmt:bold", { span_type: "opening", data: "<b>", cloneable: false }),
    );
    expect(c.cloneable).toBe(false);
    expect(c.deletable).toBe(true); // not overridden
  });
});

describe("isDeletable", () => {
  it("returns true for fmt:bold", () => {
    expect(isDeletable(span("fmt:bold", { span_type: "opening", data: "<b>" }))).toBe(true);
  });

  it("returns false for struct:break", () => {
    expect(isDeletable(span("struct:break"))).toBe(false);
  });

  it("returns false for code:variable", () => {
    expect(isDeletable(span("code:variable"))).toBe(false);
  });
});

describe("isCloneable", () => {
  it("returns true for fmt:bold", () => {
    expect(isCloneable(span("fmt:bold", { span_type: "opening", data: "<b>" }))).toBe(true);
  });

  it("returns false for struct:break", () => {
    expect(isCloneable(span("struct:break"))).toBe(false);
  });

  it("returns false for code:variable", () => {
    expect(isCloneable(span("code:variable"))).toBe(false);
  });
});
