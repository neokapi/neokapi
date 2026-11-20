import { beforeEach, describe, expect, it, vi } from "vite-plus/test";
import { deferred, act, renderHook, waitFor } from "./testUtils";
import { useProjectHistory } from "../hooks/useProjectHistory";
import { useSaveProjectTab } from "../hooks/useSaveProjectTab";
import { api } from "../hooks/useApi";

const project = { version: "v1", name: "One" };

function setup() {
  const tm = { updateTab: vi.fn(), checkPluginStatus: vi.fn().mockResolvedValue(undefined) };
  const report = vi.fn();
  const hook = renderHook(
    ({ tab }) => {
      const history = useProjectHistory(project, tab);
      const save = useSaveProjectTab(history, tm, report);
      return { history, save };
    },
    { initialProps: { tab: "a" } },
  );
  return { ...hook, tm, report };
}

describe("saving edited projects", () => {
  beforeEach(() => {
    vi.spyOn(api, "saveProjectSnapshot").mockImplementation(async (id, recipe) => ({
      tab: { id, name: recipe.name, path: "/one/kapi.yaml" },
      name: recipe.name,
    }));
  });

  it("saves each submitted snapshot in order while another tab is active", async () => {
    const { result, rerender } = setup();
    const writing = deferred<Awaited<ReturnType<typeof api.saveProjectSnapshot>>>();
    vi.mocked(api.saveProjectSnapshot).mockReturnValueOnce(writing.promise);
    act(() => result.current.history.set({ ...project, name: "Edit" }));
    const first = result.current.save("a");
    await waitFor(() => expect(api.saveProjectSnapshot).toHaveBeenCalled());
    act(() => result.current.history.set({ ...project, name: "Later" }));
    rerender({ tab: "b" });
    const second = result.current.save("a");
    expect(api.saveProjectSnapshot).toHaveBeenCalledTimes(1);
    await act(async () => {
      writing.resolve({ tab: { id: "a", name: "Edit", path: "/one/kapi.yaml" }, name: "Edit" });
      expect(await first).toBe(true);
      expect(await second).toBe(true);
    });
    expect(api.saveProjectSnapshot).toHaveBeenNthCalledWith(
      1,
      "a",
      { ...project, name: "Edit" },
      false,
    );
    expect(api.saveProjectSnapshot).toHaveBeenNthCalledWith(
      2,
      "a",
      { ...project, name: "Later" },
      false,
    );
    expect(result.current.history.hasChanges()).toBe(false);
    expect(result.current.history.project.name).toBe("One");
  });

  it("retains changes when the save dialog is canceled or the write fails", async () => {
    const { result, report } = setup();
    act(() => result.current.history.set({ ...project, name: "Edited" }));
    vi.mocked(api.saveProjectSnapshot).mockResolvedValueOnce(null);
    await act(async () => expect(await result.current.save("a", true)).toBe(false));
    vi.mocked(api.saveProjectSnapshot).mockRejectedValueOnce(new Error("disk full"));
    await act(async () => expect(await result.current.save("a")).toBe(false));
    expect(report).toHaveBeenCalledWith("Failed to save project", expect.any(Error));
    expect(result.current.history.isDirty).toBe(true);
  });

  it("keeps newer edits and undo history when Save As changes the project name", async () => {
    const { result } = setup();
    const dialog = deferred<Awaited<ReturnType<typeof api.saveProjectSnapshot>>>();
    vi.mocked(api.saveProjectSnapshot).mockReturnValueOnce(dialog.promise);
    act(() => result.current.history.set({ ...project, preset: "web" }));
    const saving = result.current.save("a", true);
    await waitFor(() => expect(api.saveProjectSnapshot).toHaveBeenCalled());
    act(() => result.current.history.set({ ...project, preset: "docs" }));
    await act(async () => {
      dialog.resolve({ tab: { id: "a", name: "Copy", path: "/copy/kapi.yaml" }, name: "Copy" });
      expect(await saving).toBe(true);
    });
    expect(result.current.history.project).toEqual({ ...project, name: "Copy", preset: "docs" });
    expect(result.current.history.isDirty).toBe(true);
    expect(result.current.history.canUndo).toBe(true);
  });
});
