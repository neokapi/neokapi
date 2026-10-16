import { describe, it, expect } from "vite-plus/test";
import type { MemoryEntryInfo, ProjectInfo } from "../../types/api";
import { entryStream, isProjectScoped, pairId, pairTestId, workspaceLanguages } from "./memoryRows";

function project(partial: Partial<ProjectInfo>): ProjectInfo {
  return {
    id: "p-1",
    name: "Website",
    default_source_language: "en-US",
    target_languages: [],
    created_at: "2026-06-01T00:00:00Z",
    modified_at: "2026-06-01T00:00:00Z",
    ...partial,
  };
}

function entry(partial: Partial<MemoryEntryInfo>): MemoryEntryInfo {
  return {
    id: "e-1",
    source: "Proceed to checkout",
    target: "Weiter zur Kasse",
    source_language: "en-US",
    target_language: "de-DE",
    updated_at: "2026-06-13T10:00:00Z",
    ...partial,
  };
}

describe("workspaceLanguages", () => {
  it("reads the shared source and every target, the most widely declared first", () => {
    const languages = workspaceLanguages([
      project({ id: "p-1", target_languages: ["fr-FR", "de-DE"] }),
      project({ id: "p-2", target_languages: ["de-DE", "nb-NO"] }),
      project({ id: "p-3", default_source_language: "de-DE", target_languages: ["en-US"] }),
    ]);
    expect(languages.source).toBe("en-US");
    // de-DE is declared twice; fr-FR and nb-NO once each, in the order listed.
    // en-US is the source, so it is not offered as a target.
    expect(languages.targets).toEqual(["de-DE", "fr-FR", "nb-NO"]);
  });

  it("reads nothing from a workspace with no projects", () => {
    expect(workspaceLanguages(undefined)).toEqual({ source: "", targets: [] });
    expect(workspaceLanguages([])).toEqual({ source: "", targets: [] });
  });

  it("drops blank and repeated targets", () => {
    const languages = workspaceLanguages([project({ target_languages: ["", " fr-FR ", "fr-FR"] })]);
    expect(languages.targets).toEqual(["fr-FR"]);
  });
});

describe("pair ids", () => {
  it("names a row by its entry and its target language", () => {
    expect(pairId(entry({}))).toBe("e-1::de-DE");
    expect(pairTestId(entry({}))).toBe("tm-entry-e-1-de-DE");
    expect(pairTestId(entry({ target_language: "" }))).toBe("tm-entry-e-1");
  });
});

describe("scope and stream", () => {
  it("tells a project's entry from the workspace's", () => {
    expect(isProjectScoped(entry({ project_id: "p-1" }))).toBe(true);
    expect(isProjectScoped(entry({}))).toBe(false);
  });

  it("names a stream only when it is not the main line", () => {
    expect(entryStream(entry({ stream: "release-2" }))).toBe("release-2");
    expect(entryStream(entry({ stream: "main" }))).toBeUndefined();
    expect(entryStream(entry({}))).toBeUndefined();
  });
});
