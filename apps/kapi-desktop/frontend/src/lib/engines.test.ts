import { describe, it, expect } from "vite-plus/test";
import { engineOf, enginesForFormat, engineFormatFor } from "./engines";
import type { FormatInfo } from "../types/api";

const f = (name: string, extensions: string[], extra: Partial<FormatInfo> = {}): FormatInfo => ({
  name,
  extensions,
  has_reader: true,
  has_writer: true,
  has_schema: false,
  ...extra,
});

const formats: FormatInfo[] = [
  f("json", [".json"], { source: "built-in", engine: "native", priority: 50 }),
  f("docling", [".json", ".docling"], { source: "built-in", engine: "native", priority: 10 }),
  f("markdown", [".md"], { source: "built-in", engine: "native", priority: 50 }),
  f("okf_json", [".json"], { source: "okapi-bridge", engine: "okapi-bridge", priority: 25 }),
  f("okf_md_lite", [".md"], { source: "okapi-bridge", priority: 25 }),
  f("okf_markdown", [".md", ".markdown"], { source: "okapi-bridge", priority: 25 }),
  f("okf_dtd", [".dtd"], { source: "okapi-bridge" }),
  f("okf_writeonly", [".json"], { source: "okapi-bridge", has_reader: false }),
];

describe("engineOf", () => {
  it("reads the listed engine and derives it from the source otherwise", () => {
    expect(engineOf({ engine: "okapi-bridge" })).toBe("okapi-bridge");
    expect(engineOf({ source: "built-in" })).toBe("native");
    expect(engineOf({})).toBe("native");
    expect(engineOf({ source: "kapi-av" })).toBe("kapi-av");
  });
});

describe("enginesForFormat", () => {
  it("lists the engines with a reader for the format's extensions, native first", () => {
    expect(enginesForFormat(formats, "json")).toEqual(["native", "okapi-bridge"]);
    expect(enginesForFormat(formats, "okf_json")).toEqual(["native", "okapi-bridge"]);
    expect(enginesForFormat(formats, "okf_dtd")).toEqual(["okapi-bridge"]);
    expect(enginesForFormat(formats, "no-such")).toEqual([]);
  });
});

describe("engineFormatFor", () => {
  it("names the format itself when it belongs to the engine", () => {
    expect(engineFormatFor(formats, "json", "native")).toBe("json");
    expect(engineFormatFor(formats, "okf_json", "okapi-bridge")).toBe("okf_json");
  });

  it("picks the other engine's reader for the same extensions", () => {
    expect(engineFormatFor(formats, "json", "okapi-bridge")).toBe("okf_json");
    expect(engineFormatFor(formats, "okf_json", "native")).toBe("json");
  });

  it("prefers the widest overlap, then priority, then name", () => {
    expect(engineFormatFor(formats, "markdown", "okapi-bridge")).toBe("okf_markdown");
    const wide = [...formats, f("okf_md_wide", [".md", ".markdown"], { source: "okapi-bridge" })];
    const withMarkdown = wide.map((x) =>
      x.name === "markdown" ? { ...x, extensions: [".md", ".markdown"] } : x,
    );
    expect(engineFormatFor(withMarkdown, "markdown", "okapi-bridge")).toBe("okf_markdown");
    const ranked = withMarkdown.map((x) =>
      x.name === "okf_md_wide" ? { ...x, priority: 500 } : x,
    );
    expect(engineFormatFor(ranked, "markdown", "okapi-bridge")).toBe("okf_md_wide");
  });

  it("is undefined when the engine has no reader for the extensions", () => {
    expect(engineFormatFor(formats, "okf_dtd", "native")).toBeUndefined();
    expect(engineFormatFor(formats, "no-such", "native")).toBeUndefined();
  });
});
