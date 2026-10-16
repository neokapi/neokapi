import { describe, it, expect, vi } from "vitest";
import userEvent from "@testing-library/user-event";
import { Clipboard } from "@wailsio/runtime";
import { render, screen, waitFor } from "./testUtils";

import {
  WorkspaceProjectRow,
  lastSegment,
  parentOf,
  projectReach,
  reachDetail,
  sharingWord,
} from "../components/WorkspaceProjectRow";
import { LOCATION } from "../stories/fixtures/standings";
import type { WorkspaceCheckout, WorkspaceProject } from "../types/api";

function checkout(dir: string, missing = false): WorkspaceCheckout {
  return { path: `/fakehome/src/${dir}`, recipe: `/fakehome/src/${dir}/kapi.yaml`, missing };
}

const PROJECT: WorkspaceProject = {
  key: "prj_fernwell",
  name: "Fernwell",
  last_active: new Date(Date.now() - 3 * 3_600_000).toISOString(),
  checkouts: [checkout("fernwell")],
};

function renderRow(project: Partial<WorkspaceProject> = {}) {
  const handlers = { onOpenCheckout: vi.fn(), onOpenContext: vi.fn(), onRemove: vi.fn() };
  render(<WorkspaceProjectRow project={{ ...PROJECT, ...project }} {...handlers} />);
  return handlers;
}

const cell = (name: string) =>
  document.querySelector(`[data-slot='workspace-project-${name}']`)?.textContent;

/** Open the row's menu and list its entries by their text. */
async function openMenu() {
  await userEvent.click(screen.getByRole("button", { name: /More for/ }));
  return screen.getAllByRole("menuitem").map((item) => item.textContent);
}

describe("a project on the workspace home", () => {
  it("is one row with the name, the voice, the rules, the reach and the last activity", () => {
    renderRow({
      voice: "Fernwell",
      context: { project: "prj_fernwell", rules: 23, suggested: 0, conflicts: 0, new: 0 },
      sharing: { kind: "local", from: "default", workspace: LOCATION },
    });
    expect(cell("name")).toBe("Fernwell");
    expect(cell("voice")).toBe("Fernwell");
    expect(cell("rules")).toBe("23 rules");
    expect(cell("reach")).toBe("this Mac");
    expect(cell("active")).toBe("3 hours ago");
    expect(document.querySelector("[data-slot='workspace-project-rules']")?.className).toContain(
      "tabular-nums",
    );
  });

  it("says when no voice is bound and leaves the columns it does not know empty", () => {
    renderRow({});
    expect(cell("voice")).toBe("no voice");
    expect(cell("rules")).toBe("");
    expect(cell("reach")).toBe("");
  });

  it("uses the singular for one rule", () => {
    renderRow({
      context: { project: "prj_fernwell", rules: 1, suggested: 0, conflicts: 0, new: 0 },
    });
    expect(cell("rules")).toBe("1 rule");
  });

  it("keeps the project key, the paths and the count of what is new off the row", () => {
    renderRow({
      context: { project: "prj_fernwell", rules: 2, suggested: 0, conflicts: 0, new: 7 },
    });
    expect(screen.queryByText("prj_fernwell")).not.toBeInTheDocument();
    expect(screen.queryByText("/fakehome/src/fernwell")).not.toBeInTheDocument();
    expect(screen.queryByText(/7 new/)).not.toBeInTheDocument();
  });

  it("opens its checkout from Open", async () => {
    const { onOpenCheckout } = renderRow();
    await userEvent.click(screen.getByRole("button", { name: "Open" }));
    expect(onOpenCheckout).toHaveBeenCalledWith("/fakehome/src/fernwell/kapi.yaml");
  });

  it("opens on its context when no checkout is here", async () => {
    const { onOpenContext } = renderRow({ checkouts: [] });
    expect(screen.queryByRole("button", { name: "Open" })).not.toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: /Open context/ }));
    expect(onOpenContext).toHaveBeenCalledWith("prj_fernwell");
  });

  it("marks context files the store has never read", () => {
    renderRow({
      context_files: { files: ["kapi/terms.yaml"], command: "kapi store import", message: "" },
    });
    expect(screen.getByTestId("workspace-context-files")).toHaveTextContent("Unread files");
  });
});

describe("the row's menu", () => {
  it("copies the path and removes, for a project with one checkout", async () => {
    const { onRemove } = renderRow();
    expect(await openMenu()).toEqual(["Copy path", "Remove"]);
    await userEvent.click(screen.getByRole("menuitem", { name: "Copy path" }));
    await waitFor(() => expect(Clipboard.SetText).toHaveBeenCalledWith("/fakehome/src/fernwell"));

    await openMenu();
    await userEvent.click(screen.getByRole("menuitem", { name: "Remove" }));
    expect(onRemove).toHaveBeenCalledWith(expect.objectContaining({ key: "prj_fernwell" }));
  });

  it("opens the other checkouts of a project with several", async () => {
    const { onOpenCheckout } = renderRow({
      checkouts: [checkout("fernwell"), checkout("fernwell-fix")],
    });
    expect(screen.getAllByTestId("workspace-project")).toHaveLength(1);
    expect((await openMenu()).slice(1)).toEqual(["Copy path", "Remove"]);
    // The folder's own name tells two worktrees apart; the folder it sits in is under it.
    const other = screen.getByTestId("checkout-open");
    expect(other).toHaveTextContent("fernwell-fix");
    expect(other).toHaveTextContent("/fakehome/src");
    await userEvent.click(other);
    expect(onOpenCheckout).toHaveBeenCalledWith("/fakehome/src/fernwell-fix/kapi.yaml");
  });

  it("lists a checkout that went away as missing, and opens the next one there", async () => {
    const { onOpenCheckout } = renderRow({
      checkouts: [checkout("fernwell", true), checkout("fernwell-fix")],
    });
    await userEvent.click(screen.getByRole("button", { name: "Open" }));
    expect(onOpenCheckout).toHaveBeenCalledWith("/fakehome/src/fernwell-fix/kapi.yaml");
    await openMenu();
    const missing = screen.getByTestId("checkout-missing");
    expect(missing).toHaveTextContent("Missing");
    expect(missing).toHaveAttribute("data-disabled");
  });

  it("opens on its context when every checkout went away", () => {
    renderRow({ checkouts: [checkout("fernwell", true)] });
    expect(screen.queryByRole("button", { name: "Open" })).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: /Open context/ })).toBeInTheDocument();
  });

  it("has no path to copy for a project with no checkout here", async () => {
    renderRow({ checkouts: [] });
    expect(await openMenu()).toEqual(["Remove"]);
  });

  it("copies the command that reads unread context files", async () => {
    renderRow({
      context_files: { files: ["kapi/terms.yaml"], command: "kapi store import", message: "" },
    });
    await openMenu();
    await userEvent.click(
      screen.getByRole("menuitem", { name: "Copy the command that reads the context files" }),
    );
    await waitFor(() => expect(Clipboard.SetText).toHaveBeenCalledWith("kapi store import"));
  });
});

describe("how far a project's context reaches", () => {
  const at = (kind: string, venue_host?: string, location?: string) => ({
    kind,
    from: "recipe",
    workspace: LOCATION,
    venue_host,
    location,
  });

  it("names each place in the one word the column has room for", () => {
    expect(sharingWord(at("local"))).toBe("this Mac");
    expect(sharingWord(at("git"))).toBe("git");
    expect(sharingWord(at("file"))).toBe("folder");
    expect(sharingWord(at("s3"))).toBe("S3");
    expect(sharingWord(at("git", "app.bowrain.cloud"))).toBe("Bowrain");
    expect(sharingWord(undefined)).toBe("");
    expect(projectReach(undefined)).toBeNull();
  });

  it("spells the reach out, with where the shared copy is, for the tooltip", () => {
    expect(reachDetail(at("git", undefined, "origin refs/kapi/context"))).toBe(
      "Shared through git, at origin refs/kapi/context",
    );
    expect(reachDetail(at("file"))).toBe("Shared through a folder");
    expect(reachDetail(at("s3", "app.bowrain.cloud"))).toBe(
      "Connected to Bowrain at app.bowrain.cloud",
    );
    expect(reachDetail(at("local"))).toBe("On this Mac");
  });
});

describe("the parts of a path", () => {
  it("takes the folder's own name on either separator", () => {
    expect(lastSegment("/fakehome/.local/share/kapi/workspaces/default")).toBe("default");
    expect(lastSegment("D:\\work\\kapi\\workspaces\\team\\")).toBe("team");
    expect(lastSegment("default")).toBe("default");
  });

  it("takes the folder a path sits in", () => {
    expect(parentOf("/fakehome/src/kapimart-release")).toBe("/fakehome/src");
    expect(parentOf("/fakehome/src/kapimart/")).toBe("/fakehome/src");
    expect(parentOf("C:\\src\\kapimart")).toBe("C:\\src");
    expect(parentOf("/kapimart")).toBe("/");
  });
});
