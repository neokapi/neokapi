import { describe, it, expect } from "vitest";
import {
  isEmptyFilter,
  matchGlob,
  filterFiles,
  filterLanguages,
  filterCollectionNames,
} from "../lib/filter";
import type { ProjectFilter } from "../types/api";

const f = (p: Partial<ProjectFilter>): ProjectFilter => ({ id: "1", name: "x", ...p });

describe("isEmptyFilter", () => {
  it("treats null / all-empty dimensions as no-op", () => {
    expect(isEmptyFilter(null)).toBe(true);
    expect(isEmptyFilter(f({ collections: [], glob: "", languages: [] }))).toBe(true);
  });
  it("is non-empty when any dimension narrows", () => {
    expect(isEmptyFilter(f({ collections: ["A"] }))).toBe(false);
    expect(isEmptyFilter(f({ glob: "*.md" }))).toBe(false);
    expect(isEmptyFilter(f({ languages: ["de-DE"] }))).toBe(false);
  });
});

describe("matchGlob", () => {
  it("matches a bare glob anywhere in the tree", () => {
    expect(matchGlob("*.json", "src/locales/en.json")).toBe(true);
    expect(matchGlob("*.json", "src/readme.md")).toBe(false);
  });
  it("respects path separators with * vs **", () => {
    expect(matchGlob("src/*.json", "src/en.json")).toBe(true);
    expect(matchGlob("src/*.json", "src/locales/en.json")).toBe(false);
    expect(matchGlob("src/**/*.json", "src/locales/en.json")).toBe(true);
  });
  it("empty glob matches everything", () => {
    expect(matchGlob("", "anything")).toBe(true);
  });
});

describe("filterFiles", () => {
  const files = [
    { path: "/p/docs/a.md", relative: "docs/a.md", collection: "Website" },
    { path: "/p/store/b.json", relative: "store/b.json", collection: "Store" },
    { path: "/p/docs/api.md", relative: "docs/api.md", collection: "Website" },
  ];

  it("returns all when the filter is empty", () => {
    expect(filterFiles(files, null)).toHaveLength(3);
  });
  it("narrows by collection", () => {
    const out = filterFiles(files, f({ collections: ["Website"] }));
    expect(out.map((x) => x.relative)).toEqual(["docs/a.md", "docs/api.md"]);
  });
  it("narrows by glob within collections", () => {
    const out = filterFiles(files, f({ collections: ["Website"], glob: "**/api*.md" }));
    expect(out.map((x) => x.relative)).toEqual(["docs/api.md"]);
  });
});

describe("filterLanguages", () => {
  it("returns all when none selected", () => {
    expect(filterLanguages(["de-DE", "fr-FR"], f({}))).toEqual(["de-DE", "fr-FR"]);
  });
  it("keeps only the selected languages", () => {
    expect(filterLanguages(["de-DE", "fr-FR", "ja-JP"], f({ languages: ["fr-FR"] }))).toEqual([
      "fr-FR",
    ]);
  });
});

describe("filterCollectionNames", () => {
  it("narrows to the selected collections", () => {
    expect(filterCollectionNames(["A", "B", "C"], f({ collections: ["A", "C"] }))).toEqual([
      "A",
      "C",
    ]);
  });
});
