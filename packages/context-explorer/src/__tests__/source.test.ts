import { describe, expect, it } from "vitest";
import { MINIMAL_CAPABILITIES, deriveCapabilities } from "../source";
import type { ContextDataSource } from "../source";
import type { GovernanceAnswer } from "../types";

const answer: GovernanceAnswer = {
  point: { default: true },
  scope: "project",
  terms: [],
  profiles: [],
  notes: [],
};

const bare: ContextDataSource = { governs: () => answer };

describe("deriveCapabilities", () => {
  it("advertises nothing a bare source cannot answer", () => {
    expect(deriveCapabilities(bare)).toEqual(MINIMAL_CAPABILITIES);
  });

  it("turns on a pane when its method is present", () => {
    const caps = deriveCapabilities({
      ...bare,
      lives: () => ({
        point: {},
        scope: "project",
        collections: [],
        items: [],
        coverage: [],
        notes: [],
      }),
      search: () => ({ query: "", scope: "project", groups: [], notes: [] }),
    });
    expect(caps.content).toBe(true);
    expect(caps.search).toBe(true);
  });

  it("assumes the narrower reach when relations are implemented but not declared", () => {
    const caps = deriveCapabilities({
      ...bare,
      relates: (subject) => ({
        subject,
        reach: "workspace",
        occurrences: [],
        projects: [],
        blessings: [],
        notes: [],
      }),
    });
    expect(caps.relations).toBe("project");
  });

  it("lets a source declare the reach its methods cannot express", () => {
    const caps = deriveCapabilities({
      ...bare,
      capabilities: { relations: "workspace" },
      relates: (subject) => ({
        subject,
        reach: "workspace",
        occurrences: [],
        projects: [],
        blessings: [],
        notes: [],
      }),
    });
    expect(caps.relations).toBe("workspace");
  });

  it("carries the free dimensions a source declares", () => {
    const caps = deriveCapabilities({ ...bare, capabilities: { free: ["project", "locale"] } });
    expect(caps.free).toEqual(["project", "locale"]);
  });
});
