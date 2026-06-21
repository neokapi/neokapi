// @vitest-environment jsdom
import { describe, expect, it } from "vitest";
import { render } from "@testing-library/react";
import ProjectExplorer, {
  FLOWS,
  TARGETS,
  formatFor,
  recipeFor,
  targetGlob,
} from "../ProjectExplorer";
import { workspaceSampleById } from "../workspaceSamples";

describe("ProjectExplorer helpers", () => {
  it("declares offline TM-leverage flows", () => {
    const ids = FLOWS.map((f) => f.id);
    expect(ids).toContain("translate");
    expect(ids).toContain("translate-exact");
    // Every flow's steps use the offline recycle tool (no LLM, no network).
    for (const f of FLOWS) {
      expect(f.yaml).toContain("tool: recycle");
    }
  });

  it("declares a real fr shipping target and never the qps test locale", () => {
    expect(TARGETS).toEqual(["fr"]);
    expect(TARGETS as readonly string[]).not.toContain("qps");
  });

  it("maps filenames to formats", () => {
    expect(formatFor("messages.json")).toBe("json");
    expect(formatFor("welcome.docx")).toBe("openxml");
    expect(formatFor("report.xlsx")).toBe("openxml");
  });

  it("builds a per-locale target glob preserving the extension", () => {
    expect(targetGlob("messages.json")).toBe("out/{lang}/messages.json");
    expect(targetGlob("welcome.docx")).toBe("out/{lang}/welcome.docx");
  });

  it("renders a complete recipe with content + every declared flow", () => {
    const recipe = recipeFor(workspaceSampleById("json"));
    expect(recipe).toContain("source_language: en");
    expect(recipe).toContain("target_languages: [fr]");
    // qps is a pseudo-translate test locale, never a recipe shipping target.
    expect(recipe).not.toContain("qps");
    expect(recipe).toContain("path: messages.json");
    expect(recipe).toContain('target: "out/{lang}/messages.json"');
    for (const f of FLOWS) {
      expect(recipe).toContain(`  ${f.id}:`);
    }
  });

  it("provides a per-sample en→fr TMX matching the sample's source text", () => {
    const json = workspaceSampleById("json");
    expect(json.tmx).toContain('<tuv xml:lang="en"><seg>Welcome to Acme</seg></tuv>');
    expect(json.tmx).toContain('xml:lang="fr"');
    const docx = workspaceSampleById("docx");
    expect(docx.tmx).toContain("Sign in to continue");
    const xlsx = workspaceSampleById("xlsx");
    expect(xlsx.tmx).toContain("Total revenue");
  });

  it("renders a recipe for a binary (Office) sample as openxml", () => {
    const recipe = recipeFor(workspaceSampleById("docx"));
    expect(recipe).toContain("format: openxml");
    expect(recipe).toContain("path: welcome.docx");
  });
});

describe("ProjectExplorer component", () => {
  it("mounts inert when no WASM assets are provided", () => {
    // assets=null means the runtime never boots (status stays idle), so the
    // component renders its booting/idle shell without touching WASM — safe in
    // jsdom and proves the wiring compiles + renders.
    const { container } = render(<ProjectExplorer assets={null} defaultSampleId="json" />);
    expect(container.querySelector("pre, div")).toBeTruthy();
  });
});
