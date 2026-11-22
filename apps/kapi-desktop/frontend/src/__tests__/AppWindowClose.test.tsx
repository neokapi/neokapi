import { beforeEach, describe, expect, it, vi } from "vite-plus/test";
import { Events } from "@wailsio/runtime";
import { act, fireEvent, render, screen, waitFor, within } from "./testUtils";
import App from "../App";
import { api } from "../hooks/useApi";
import { CLOSE_REQUESTED_EVENT } from "../hooks/useWindowCloseGuard";
import type { KapiProject, TabInfo } from "../types/api";

// The real App, with the backend answering for two open projects. Nothing
// between the tab history, the save path and the close guard is stubbed, so a
// wrong hasChanges or saveTab wiring in App.tsx fails here.
const tabs: TabInfo[] = [
  { id: "a", name: "compass", path: "/p/compass/kapi.yaml", recipe_written: true },
  { id: "b", name: "northsea", path: "/p/northsea/kapi.yaml", recipe_written: true },
];
const projects: Record<string, KapiProject> = {
  a: { version: "v1", name: "compass" },
  b: { version: "v1", name: "northsea" },
};

/** Delivers a backend event to the handlers the app registered. */
function emit(name: string, data: unknown) {
  for (const [event, handler] of vi.mocked(Events.On).mock.calls) {
    if (event === name) (handler as (e: { data: unknown }) => void)({ data });
  }
}

describe("App: closing the window with one dirty and one clean tab", () => {
  beforeEach(() => {
    vi.mocked(Events.On).mockClear();
    vi.spyOn(api, "getSessionState").mockResolvedValue({
      lastOpenProjects: [],
      activeProject: "",
    } as never);
    vi.spyOn(api, "listTabs").mockResolvedValue(tabs);
    vi.spyOn(api, "getProject").mockImplementation(async (id: string) => projects[id]);
    vi.spyOn(api, "setUnsavedTabs").mockResolvedValue(undefined);
    vi.spyOn(api, "acknowledgeClose").mockResolvedValue(true);
    vi.spyOn(api, "confirmClose").mockResolvedValue(undefined);
    vi.spyOn(api, "cancelClose").mockResolvedValue(undefined);
    vi.spyOn(api, "saveProjectSnapshot").mockImplementation(async (id, recipe) => ({
      tab: tabs.find((t) => t.id === id)!,
      name: recipe.name,
    }));
  });

  it("lists only the dirty tab and saves it through the real save path", async () => {
    render(<App />);
    const strip = await screen.findByRole("tablist", { name: "Open projects" });

    // Rename northsea in its tab: an edit to that project's history.
    fireEvent.doubleClick(within(strip).getByText("northsea"));
    const input = within(strip).getByRole("textbox");
    fireEvent.change(input, { target: { value: "northsea edited" } });
    fireEvent.keyDown(input, { key: "Enter" });
    await waitFor(() => expect(api.setUnsavedTabs).toHaveBeenLastCalledWith(["b"]));

    act(() =>
      emit(CLOSE_REQUESTED_EVENT, {
        route: "window",
        blockers: [{ id: "b", name: "northsea", reasons: ["unsaved"] }],
      }),
    );
    expect(api.acknowledgeClose).toHaveBeenCalled();
    const list = await screen.findByRole("list", { name: "Projects that keep the window open" });
    expect(list).toHaveTextContent("northsea edited");
    expect(list).not.toHaveTextContent("compass");

    fireEvent.click(screen.getByRole("button", { name: "Save all" }));
    await waitFor(() => expect(api.confirmClose).toHaveBeenCalledTimes(1));
    expect(api.saveProjectSnapshot).toHaveBeenCalledTimes(1);
    expect(api.saveProjectSnapshot).toHaveBeenCalledWith(
      "b",
      expect.objectContaining({ name: "northsea edited" }),
      false,
    );
    await waitFor(() => expect(api.setUnsavedTabs).toHaveBeenLastCalledWith([]));
  });

  // The page loads while the backend runs a flow in compass (a reload): it
  // picks the run up, and a quit asks about it instead of closing at once.
  it("after a reload during a run, a quit asks about the run", async () => {
    vi.spyOn(api, "getRunState").mockResolvedValue("running");
    vi.spyOn(api, "getRunEvents").mockResolvedValue([
      { tab_id: "a", type: "state", flow_id: "converge", message: "running" },
    ] as never);
    render(<App />);
    await screen.findByRole("tablist", { name: "Open projects" });
    await waitFor(() => expect(api.getRunEvents).toHaveBeenCalled());

    act(() =>
      emit(CLOSE_REQUESTED_EVENT, {
        route: "quit",
        blockers: [{ id: "a", name: "compass", reasons: ["running"] }],
      }),
    );
    const list = await screen.findByRole("list", { name: "Projects that keep the window open" });
    expect(list).toHaveTextContent("compass");
    expect(list).toHaveTextContent("Flow running");
    expect(api.confirmClose).not.toHaveBeenCalled();

    fireEvent.click(screen.getByRole("button", { name: "Keep running" }));
    await waitFor(() => expect(api.cancelClose).toHaveBeenCalledTimes(1));
    expect(api.confirmClose).not.toHaveBeenCalled();
  });
});
