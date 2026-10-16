import { describe, it, expect, vi } from "vitest";
import { render } from "./testUtils";

import { WorkspaceProjectRow, sharingWord } from "../components/WorkspaceProjectRow";
import { LOCATION } from "../stories/fixtures/standings";
import type { WorkspaceProject } from "../types/api";

const PROJECT: WorkspaceProject = {
  key: "prj_fernwell",
  name: "Fernwell",
  checkouts: [],
} as unknown as WorkspaceProject;

function renderRow(news?: Parameters<typeof WorkspaceProjectRow>[0]["news"]) {
  render(
    <WorkspaceProjectRow
      project={PROJECT}
      news={news}
      onOpenCheckout={vi.fn()}
      onOpenContext={vi.fn()}
      onRemove={vi.fn()}
    />,
  );
  return document.querySelector("[data-slot='context-news']");
}

describe("a project on the workspace home", () => {
  it("says what is new since the person last looked", () => {
    const since = new Date(Date.now() - 3 * 86_400_000).toISOString();
    const badge = renderRow({ project: "prj_fernwell", new: 4, conflicts: 0, since });
    expect(badge?.textContent).toMatch(/^4 new since /);
  });

  it("says how much is new when the person never looked", () => {
    expect(renderRow({ project: "prj_fernwell", new: 2, conflicts: 0 })?.textContent).toBe("2 new");
  });

  it("shows nothing when there is no news", () => {
    expect(renderRow()).toBeNull();
    expect(renderRow({ project: "prj_fernwell", new: 0, conflicts: 1 })).toBeNull();
  });
});

describe("the line under a project's name", () => {
  function facts(project: Partial<WorkspaceProject>) {
    render(
      <WorkspaceProjectRow
        project={{ ...PROJECT, ...project }}
        onOpenCheckout={vi.fn()}
        onOpenContext={vi.fn()}
        onRemove={vi.fn()}
      />,
    );
    return document.querySelector("[data-slot='workspace-project-facts']")?.textContent;
  }

  it("names the voice, the rules and how far the context reaches", () => {
    expect(
      facts({
        voice: "Fernwell",
        context: { project: "prj_fernwell", rules: 23, suggested: 0, conflicts: 0, new: 0 },
        sharing: { kind: "local", from: "default", workspace: LOCATION },
      }),
    ).toBe("voice Fernwell·23 rules·on this Mac");
  });

  it("says when no voice is bound and leaves out what it does not know", () => {
    expect(facts({})).toBe("no voice bound");
  });

  it("uses the singular for one rule", () => {
    expect(
      facts({ context: { project: "prj_fernwell", rules: 1, suggested: 0, conflicts: 0, new: 0 } }),
    ).toBe("no voice bound·1 rule");
  });
});

describe("the sharing word", () => {
  const at = (kind: string, venue_host?: string) =>
    sharingWord({ kind, from: "recipe", workspace: LOCATION, venue_host });

  it("names each place the context can be shared through", () => {
    expect(at("local")).toBe("on this Mac");
    expect(at("git")).toBe("shared through git");
    expect(at("file")).toBe("shared through a folder");
    expect(at("s3")).toBe("shared through S3");
    expect(at("git", "app.bowrain.cloud")).toBe("Bowrain app.bowrain.cloud");
    expect(sharingWord(undefined)).toBe("");
  });
});
