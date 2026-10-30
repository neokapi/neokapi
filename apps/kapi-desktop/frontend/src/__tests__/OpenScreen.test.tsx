import { render, screen, waitFor } from "./testUtils";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vite-plus/test";
import { Events } from "@wailsio/runtime";
import { OpenScreen, droppedPath, firstScreen, recentFolders } from "../components/OpenScreen";
import type { WorkspaceHome, WorkspaceProject } from "../types/api";

function project(key: string, dir: string, lastActive: string, missing = false): WorkspaceProject {
  return {
    key,
    name: dir.split("/").pop() ?? key,
    last_active: lastActive,
    checkouts: [{ path: dir, recipe: `${dir}/kapi.yaml`, missing }],
  };
}

function workspaceOf(...projects: WorkspaceProject[]): WorkspaceHome {
  return { location: "/fakehome/.local/share/kapi/workspaces/default", read_only: false, projects };
}

/** Resolve the handler the screen registered for a Wails event. */
async function handlerFor(name: string): Promise<(e: { data: unknown }) => void> {
  const on = vi.mocked(Events.On);
  return await waitFor(() => {
    const call = on.mock.calls.find((c) => c[0] === name);
    if (!call) throw new Error(`no subscription for ${name}`);
    return call[1] as (e: { data: unknown }) => void;
  });
}

const props = () => ({
  workspace: workspaceOf(),
  onChooseFolder: vi.fn(),
  onOpenPath: vi.fn(),
  onTrySample: vi.fn(),
  onOpenSettings: vi.fn(),
});

describe("OpenScreen", () => {
  beforeEach(() => vi.mocked(Events.On).mockClear());

  it("leads with the title and the two ways in", async () => {
    const p = props();
    render(<OpenScreen {...p} />);
    expect(
      screen.getByRole("heading", { name: "Drop a folder to see what it writes" }),
    ).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "Choose a folder" }));
    expect(p.onChooseFolder).toHaveBeenCalledOnce();
    await userEvent.click(screen.getByRole("button", { name: "Try with KapiMart" }));
    expect(p.onTrySample).toHaveBeenCalledOnce();
  });

  it("reaches the app's settings from its footer", async () => {
    const p = props();
    render(<OpenScreen {...p} />);
    await userEvent.click(screen.getByRole("button", { name: "Settings" }));
    expect(p.onOpenSettings).toHaveBeenCalledOnce();
  });

  it("says where recent folders will appear when there are none", () => {
    render(<OpenScreen {...props()} />);
    expect(screen.getByText("Recent folders appear here.")).toBeInTheDocument();
    expect(screen.queryAllByTestId("open-recent-folder")).toHaveLength(0);
  });

  it("lists the folders the workspace has seen and opens one", async () => {
    const p = props();
    p.workspace = workspaceOf(
      project("prj_docs", "/fakehome/src/docs", "2026-10-01T10:00:00Z"),
      project("prj_site", "/fakehome/src/site", "2026-10-20T10:00:00Z"),
    );
    render(<OpenScreen {...p} />);
    const rows = screen.getAllByTestId("open-recent-folder");
    expect(rows.map((r) => r.textContent)).toEqual([
      expect.stringContaining("site"),
      expect.stringContaining("docs"),
    ]);
    await userEvent.click(rows[0]);
    expect(p.onOpenPath).toHaveBeenCalledWith("/fakehome/src/site/kapi.yaml");
  });

  it("marks the whole screen as the window's drop target", () => {
    const { container } = render(<OpenScreen {...props()} />);
    expect(container.querySelector('[data-slot="open-screen"]')).toHaveAttribute(
      "data-file-drop-target",
    );
  });

  it("opens the folder the window reports dropped", async () => {
    const p = props();
    render(<OpenScreen {...p} />);
    const onDropped = await handlerFor("files-dropped");
    onDropped({ data: ["/fakehome/src/handbook"] });
    expect(p.onOpenPath).toHaveBeenCalledWith("/fakehome/src/handbook");
  });

  it("hands a dropped file to the open path, which opens the folder it is in", async () => {
    const p = props();
    render(<OpenScreen {...p} />);
    const onDropped = await handlerFor("files-dropped");
    onDropped({ data: ["/fakehome/src/handbook/docs/intro.md", "/fakehome/other.md"] });
    expect(p.onOpenPath).toHaveBeenCalledOnce();
    expect(p.onOpenPath).toHaveBeenCalledWith("/fakehome/src/handbook/docs/intro.md");
  });

  it("ignores a drop that carries no path", async () => {
    const p = props();
    render(<OpenScreen {...p} />);
    const onDropped = await handlerFor("files-dropped");
    onDropped({ data: null });
    onDropped({ data: [] });
    expect(p.onOpenPath).not.toHaveBeenCalled();
  });
});

describe("recentFolders", () => {
  it("leaves out a project with no folder on this machine", () => {
    const rows = recentFolders(
      workspaceOf(
        project("prj_gone", "/fakehome/src/gone", "2026-10-21T10:00:00Z", true),
        project("prj_docs", "/fakehome/src/docs", "2026-10-01T10:00:00Z"),
      ),
    );
    expect(rows.map((r) => r.key)).toEqual(["prj_docs"]);
  });

  it("is empty while the workspace is read", () => {
    expect(recentFolders(null)).toEqual([]);
  });
});

describe("droppedPath", () => {
  it("takes the first path of a drop", () => {
    expect(droppedPath(["", "/a", "/b"])).toBe("/a");
    expect(droppedPath("nope")).toBeNull();
  });
});

describe("firstScreen", () => {
  const one = workspaceOf(project("prj_docs", "/fakehome/src/docs", "2026-10-01T10:00:00Z"));
  const two = workspaceOf(
    project("prj_docs", "/fakehome/src/docs", "2026-10-01T10:00:00Z"),
    project("prj_site", "/fakehome/src/site", "2026-10-02T10:00:00Z"),
  );
  const base = { workspace: one, tabCount: 0, restoring: false, view: "home" };

  it("holds the title bar alone while a remembered session is reopened", () => {
    expect(firstScreen({ ...base, restoring: true })).toBe("blank");
  });

  it("holds the title bar alone while the workspace is read", () => {
    expect(firstScreen({ ...base, workspace: null })).toBe("blank");
  });

  it("shows the Open screen with fewer than two projects and no folder open", () => {
    expect(firstScreen(base)).toBe("open");
    expect(firstScreen({ ...base, workspace: workspaceOf() })).toBe("open");
    expect(firstScreen({ ...base, workspace: null, workspaceError: new Error("x") })).toBe("open");
  });

  it("shows the app once a folder is open, two projects exist, or settings are asked for", () => {
    expect(firstScreen({ ...base, tabCount: 1, restoring: true })).toBe("app");
    expect(firstScreen({ ...base, workspace: two })).toBe("app");
    expect(firstScreen({ ...base, view: "app-settings" })).toBe("app");
  });
});
