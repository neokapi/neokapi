import { projectSubNavConfig, subNavConfig, type ProjectInfo } from "@neokapi/ui";
import { describe, expect, it } from "vite-plus/test";

import { projectSectionTargets, subNavTarget, subNavTargets } from "./sub-nav-targets";

// A sidebar item that renders and goes nowhere is invisible to every other kind
// of test: it has a label, an icon and a hover state, and only the click is
// missing. It shipped twice — "Content memory" and "Locale demand" — because
// adding an item to subNavConfig() and adding its destination are separate
// edits in separate packages.
describe("sub-nav destinations", () => {
  const renderedIds = Object.values(subNavConfig()).flatMap((items) =>
    items.map((item) => item.id),
  );

  it("every rendered sub-nav item has one", () => {
    const dead = renderedIds.filter((id) => !subNavTarget(id));
    expect(dead, `sub-nav items that would click through to nothing: ${dead.join(", ")}`).toEqual(
      [],
    );
  });

  it("has no destination for an item that is no longer rendered", () => {
    // The other direction: a leftover entry is a route we intend to reach and
    // no longer offer, which is how a rename leaves a dangling target behind.
    const orphaned = Object.keys(subNavTargets).filter((id) => !renderedIds.includes(id));
    expect(orphaned, `destinations with no sidebar item: ${orphaned.join(", ")}`).toEqual([]);
  });

  it("routes every destination through the workspace param", () => {
    // Each target is interpolated with { workspace }, so one that does not name
    // the param would navigate to a literal "$workspace" path.
    for (const [id, to] of Object.entries(subNavTargets)) {
      expect(to, `${id} must be workspace-scoped`).toMatch(/^\/\$workspace(\/|$)/);
    }
  });

  it("returns undefined for an unknown id rather than a bare path", () => {
    expect(subNavTarget("not-a-section")).toBeUndefined();
  });
});

// The same invariant for a project's own sections: every one the panel lists
// navigates somewhere, in the order the panel lists them, Map first.
describe("project section destinations", () => {
  const noop = () => {};
  const listed = projectSubNavConfig({
    level: "project",
    project: { id: "p1", name: "Fernwell" } as ProjectInfo,
    activeStream: "main",
    activeProjectView: "map",
    onOpenMap: noop,
    onOpenDashboard: noop,
    onOpenRules: noop,
    onOpenSource: noop,
    onOpenFile: noop,
    onStreamChange: noop,
    onOpenAutomations: noop,
    onOpenRuns: noop,
    onOpenConnectors: noop,
  }).map((item) => item.id);

  it("projectSectionTargetsAreComplete: one per listed section, in the panel's order", () => {
    expect(listed).toEqual(Object.keys(projectSectionTargets));
  });

  it("opens on the Map, with Overview one entry down", () => {
    expect(listed.slice(0, 3)).toEqual(["map", "dashboard", "rules"]);
  });

  it("routes every destination through the project and stream params", () => {
    for (const [id, to] of Object.entries(projectSectionTargets)) {
      expect(to, `${id} must be stream-scoped`).toMatch(
        /^\/\$workspace\/p\/\$projectId\/s\/\$stream(\/|$)/,
      );
    }
  });
});
