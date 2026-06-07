import { describe, expect, it } from "vitest";
import { extOf, fileType } from "@neokapi/ui-primitives/preview";

describe("extOf", () => {
  it("extracts the lower-cased extension", () => {
    expect(extOf("a/b.JSON")).toBe("json");
    expect(extOf("messages.xliff")).toBe("xliff");
    expect(extOf("README")).toBe("");
    expect(extOf(".gitignore")).toBe("");
  });
});

describe("fileType", () => {
  it("classifies known localisation formats", () => {
    expect(fileType("messages.json").group).toBe("data");
    expect(fileType("app.xliff").group).toBe("bilingual");
    expect(fileType("app.properties").group).toBe("catalog");
    expect(fileType("page.html").lang).toBe("xml");
  });

  it("flags binary office formats", () => {
    expect(fileType("report.docx").binary).toBe(true);
    expect(fileType("messages.json").binary).toBe(false);
  });

  it("falls back for unknown extensions", () => {
    const t = fileType("weird.zzz");
    expect(t.label).toBe("ZZZ");
    expect(t.lang).toBe("text");
    expect(t.group).toBe("text");
  });
});
