import { describe, expect, it } from "vitest";
import type { ProjectInfo } from "../../types/api";
import { workspaceSourceLocale } from "./workspace-source-locale";

const project = (id: string, sourceLanguage: string): ProjectInfo =>
  ({
    id,
    name: id,
    default_source_language: sourceLanguage,
    target_languages: [],
  }) as unknown as ProjectInfo;

describe("workspaceSourceLocale", () => {
  it("is empty with no projects", () => {
    expect(workspaceSourceLocale(undefined)).toBe("");
    expect(workspaceSourceLocale([])).toBe("");
  });

  it("takes the locale most projects declare", () => {
    const projects = [project("a", "de-DE"), project("b", "en-US"), project("c", "en-US")];
    expect(workspaceSourceLocale(projects)).toBe("en-US");
  });

  it("breaks a tie on the locale that sorts first", () => {
    expect(workspaceSourceLocale([project("a", "fr-FR"), project("b", "de-DE")])).toBe("de-DE");
    expect(workspaceSourceLocale([project("b", "de-DE"), project("a", "fr-FR")])).toBe("de-DE");
  });

  it("ignores projects that declare no source language", () => {
    expect(
      workspaceSourceLocale([project("a", ""), project("b", "  "), project("c", "nb-NO")]),
    ).toBe("nb-NO");
  });
});
