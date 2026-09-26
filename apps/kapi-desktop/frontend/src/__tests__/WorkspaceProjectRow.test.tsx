import { describe, it, expect, vi } from "vitest";
import { render } from "./testUtils";

import { WorkspaceProjectRow } from "../components/WorkspaceProjectRow";
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
