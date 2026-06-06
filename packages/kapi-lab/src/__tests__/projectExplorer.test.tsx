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
  it("declares offline pseudo flows", () => {
    const ids = FLOWS.map((f) => f.id);
    expect(ids).toContain("pseudo");
    expect(ids).toContain("accent");
    // Every flow's steps use the offline, deterministic pseudo-translate tool.
    for (const f of FLOWS) {
      expect(f.yaml).toContain("tool: pseudo-translate");
    }
  });

  it("declares multiple target locales", () => {
    expect(TARGETS).toEqual(["fr", "qps"]);
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
    expect(recipe).toContain("target_languages: [fr, qps]");
    expect(recipe).toContain("path: messages.json");
    expect(recipe).toContain('target: "out/{lang}/messages.json"');
    for (const f of FLOWS) {
      expect(recipe).toContain(`  ${f.id}:`);
    }
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
