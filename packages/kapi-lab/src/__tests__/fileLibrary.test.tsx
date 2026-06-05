// @vitest-environment jsdom
import { describe, expect, it } from "vitest";
import { act, renderHook } from "@testing-library/react";
import { resolveSelection, selectionSummary, useFileLibrary } from "../fileLibrary";

const enc = new TextEncoder();

describe("useFileLibrary", () => {
  it("seeds the bundled samples", () => {
    const { result } = renderHook(() => useFileLibrary({}));
    expect(result.current.files.length).toBeGreaterThan(0);
    expect(result.current.paths).toContain("messages.json");
    expect(result.current.files.every((f) => f.origin === "sample")).toBe(true);
  });

  it("restricts to the requested samples", () => {
    const { result } = renderHook(() => useFileLibrary({ sampleIds: ["messages-json"] }));
    expect(result.current.paths).toEqual(["messages.json"]);
  });

  it("records outputs and derives their folder", () => {
    const { result } = renderHook(() => useFileLibrary({ sampleIds: ["messages-json"] }));
    act(() => result.current.setOutput("out/messages.json", enc.encode("{}")));
    expect(result.current.paths).toContain("out/messages.json");
    expect(result.current.folders).toContain("out");
    expect(result.current.get("out/messages.json")?.origin).toBe("output");
  });

  it("overwriting an output bumps changedAt", () => {
    const { result } = renderHook(() => useFileLibrary({ sampleIds: ["messages-json"] }));
    act(() => result.current.setOutput("out/x.json", enc.encode("a")));
    const before = result.current.get("out/x.json")!.changedAt;
    act(() => result.current.setOutput("out/x.json", enc.encode("b")));
    expect(result.current.get("out/x.json")!.changedAt).toBeGreaterThan(before);
  });

  it("removes files and whole folders", () => {
    const { result } = renderHook(() => useFileLibrary({ sampleIds: ["messages-json"] }));
    act(() => {
      result.current.setOutput("out/a.json", enc.encode("a"));
      result.current.setOutput("out/b.json", enc.encode("b"));
    });
    act(() => result.current.removeFolder("out"));
    expect(result.current.paths).toEqual(["messages.json"]);
  });

  it("normalises wasm /project paths", () => {
    const { result } = renderHook(() => useFileLibrary({ sampleIds: ["messages-json"] }));
    act(() => result.current.setOutput("/project/out/c.json", enc.encode("c")));
    expect(result.current.paths).toContain("out/c.json");
  });
});

describe("selection", () => {
  it("resolves single, multi and glob selections", () => {
    const { result } = renderHook(() => useFileLibrary({}));
    const lib = result.current;
    expect(resolveSelection({ mode: "single", paths: ["messages.json"] }, lib)).toHaveLength(1);
    const multi = resolveSelection({ mode: "multi", paths: ["messages.json", "app.xliff"] }, lib);
    expect(multi).toHaveLength(2);
    const glob = resolveSelection({ mode: "glob", paths: [], pattern: "*.json" }, lib);
    expect(glob.every((f) => f.name.endsWith(".json"))).toBe(true);
    expect(glob.length).toBeGreaterThan(0);
  });

  it("summarises selections for the compact field", () => {
    const { result } = renderHook(() => useFileLibrary({}));
    const lib = result.current;
    expect(selectionSummary({ mode: "single", paths: ["messages.json"] }, lib)).toBe(
      "messages.json",
    );
    expect(selectionSummary({ mode: "multi", paths: ["messages.json", "app.xliff"] }, lib)).toBe(
      "2 files",
    );
    expect(selectionSummary({ mode: "glob", paths: [], pattern: "*.json" }, lib)).toMatch(
      /^\*\.json · \d+ match/,
    );
  });
});
