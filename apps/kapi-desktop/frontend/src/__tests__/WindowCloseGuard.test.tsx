import { beforeEach, describe, expect, it, vi } from "vite-plus/test";
import { act, fireEvent, render, renderHook, screen, waitFor } from "./testUtils";
import {
  closeRows,
  saveTabs,
  useWindowCloseGuard,
  type CloseRequest,
  type UnsavedTab,
} from "../hooks/useWindowCloseGuard";
import { WindowCloseDialog } from "../components/WindowCloseDialog";
import { api } from "../hooks/useApi";

const tabs: UnsavedTab[] = [
  { id: "a", name: "compass" },
  { id: "b", name: "northsea" },
  { id: "c", name: "kapimart" },
];

/** The payload the backend sends for these unsaved and running tabs. */
function request(unsaved: string[], running: string[] = []): CloseRequest {
  const ids = [...new Set([...unsaved, ...running])];
  return {
    route: "window",
    blockers: ids.map((id) => ({
      id,
      name: `backend ${id}`,
      reasons: [
        ...(unsaved.includes(id) ? ["unsaved"] : []),
        ...(running.includes(id) ? ["running"] : []),
      ],
    })),
  };
}

interface Opts {
  dirty?: string[];
  finished?: string[];
  saveTab?: (id: string) => Promise<boolean>;
  cancelRun?: (id: string) => Promise<unknown>;
}

function setup({ dirty = [], finished = [], saveTab = vi.fn(), cancelRun = vi.fn() }: Opts) {
  return renderHook(() =>
    useWindowCloseGuard({
      tabs,
      hasChanges: (id) => dirty.includes(id),
      saveTab,
      finishedTabIDs: finished,
      cancelRun,
    }),
  );
}

/** The guard and its dialog together, as App renders them. */
function Harness({
  dirty = [],
  finished = [],
  saveTab = vi.fn(),
  cancelRun = vi.fn().mockResolvedValue(undefined),
  payload,
}: Opts & { payload: CloseRequest }) {
  const guard = useWindowCloseGuard({
    tabs,
    hasChanges: (id) => dirty.includes(id),
    saveTab,
    finishedTabIDs: finished,
    cancelRun,
  });
  return (
    <>
      <button type="button" onClick={() => void guard.onCloseRequested(payload)}>
        close window
      </button>
      {guard.open && (
        <WindowCloseDialog
          rows={guard.rows}
          failed={guard.failed}
          saving={guard.saving}
          onSaveAll={() => void guard.saveAll()}
          onDiscardAll={() => void guard.discardAll()}
          onCancel={() => void guard.cancel()}
        />
      )}
    </>
  );
}

const list = () => screen.getByRole("list", { name: "Projects that keep the window open" });
const closeWindow = async () => {
  fireEvent.click(screen.getByText("close window"));
  await screen.findByTestId("window-close-dialog");
};

describe("closing the window with unsaved changes or a running flow", () => {
  beforeEach(() => {
    vi.spyOn(api, "setUnsavedTabs").mockResolvedValue(undefined);
    vi.spyOn(api, "confirmClose").mockResolvedValue(undefined);
    vi.spyOn(api, "cancelClose").mockResolvedValue(undefined);
    vi.spyOn(api, "acknowledgeClose").mockResolvedValue(true);
  });

  it("reports the tabs with unsaved changes to the backend", async () => {
    setup({ dirty: ["a", "c"] });
    await waitFor(() => expect(api.setUnsavedTabs).toHaveBeenCalledWith(["a", "c"]));
  });

  it("acknowledges each close request", async () => {
    const { result } = setup({ dirty: ["a"] });
    await act(() => result.current.onCloseRequested(request(["a"])));
    expect(api.acknowledgeClose).toHaveBeenCalledTimes(1);
  });

  it("shows nothing when the backend's native dialog already asks", async () => {
    vi.mocked(api.acknowledgeClose).mockResolvedValue(false);
    const { result } = setup({ dirty: ["a"] });
    await act(() => result.current.onCloseRequested(request(["a"])));
    expect(result.current.open).toBe(false);
    expect(api.confirmClose).not.toHaveBeenCalled();
  });

  it("stops holding the window when it unmounts", async () => {
    const { unmount } = setup({ dirty: ["a"] });
    await waitFor(() => expect(api.setUnsavedTabs).toHaveBeenCalledWith(["a"]));
    unmount();
    expect(api.setUnsavedTabs).toHaveBeenLastCalledWith([]);
  });

  it("stops holding the window when the page unloads", async () => {
    setup({ dirty: ["a"] });
    await waitFor(() => expect(api.setUnsavedTabs).toHaveBeenCalledWith(["a"]));
    window.dispatchEvent(new Event("pagehide"));
    expect(api.setUnsavedTabs).toHaveBeenLastCalledWith([]);
  });

  it("closes at once when the backend sends no blocker", async () => {
    const { result } = setup({});
    await act(() => result.current.onCloseRequested(request([])));
    expect(result.current.open).toBe(false);
    expect(api.confirmClose).toHaveBeenCalledTimes(1);
  });

  // A page reloaded during a run has no unsaved tabs and may not yet know the
  // run; the backend's blockers still decide, so the run is asked about.
  it("decides from the backend's blockers, not the page's own view", async () => {
    const { result } = setup({});
    await act(() => result.current.onCloseRequested(request([], ["c"])));
    expect(result.current.open).toBe(true);
    expect(result.current.rows).toEqual([
      { id: "c", name: "kapimart", unsaved: false, running: true },
    ]);
    expect(api.confirmClose).not.toHaveBeenCalled();
  });

  it("keeps the backend's name for a tab the page does not show", async () => {
    const { result } = setup({});
    await act(() => result.current.onCloseRequested(request([], ["z"])));
    expect(result.current.rows).toEqual([
      { id: "z", name: "backend z", unsaved: false, running: true },
    ]);
  });

  it("merges unsaved and running tabs into one row each, in tab order", () => {
    expect(closeRows(tabs, [tabs[2], tabs[0]], ["c", "b"])).toEqual([
      { ...tabs[0], unsaved: true, running: false },
      { ...tabs[1], unsaved: false, running: true },
      { ...tabs[2], unsaved: true, running: true },
    ]);
  });

  describe("only unsaved changes", () => {
    it("lists each dirty tab and offers Save all, Discard all and Cancel", async () => {
      render(<Harness dirty={["a", "b"]} payload={request(["a", "b"])} />);
      await closeWindow();
      expect(screen.getByText("Unsaved changes", { selector: "h2" })).toBeInTheDocument();
      expect(list()).toHaveTextContent("compass");
      expect(list()).toHaveTextContent("northsea");
      expect(list()).not.toHaveTextContent("kapimart");
      expect(screen.getByRole("button", { name: "Discard all" })).toBeInTheDocument();
      expect(screen.getByRole("button", { name: "Cancel" })).toBeInTheDocument();
      await waitFor(() => expect(screen.getByRole("button", { name: "Save all" })).toHaveFocus());
    });

    it("Save all saves each listed tab, then closes", async () => {
      const saveTab = vi.fn().mockResolvedValue(true);
      render(<Harness dirty={["a", "b"]} saveTab={saveTab} payload={request(["a", "b"])} />);
      await closeWindow();
      fireEvent.click(screen.getByRole("button", { name: "Save all" }));
      await waitFor(() => expect(api.confirmClose).toHaveBeenCalledTimes(1));
      expect(saveTab.mock.calls).toEqual([["a"], ["b"]]);
    });

    it("a failed save keeps the window open and names the tab", async () => {
      const saveTab = vi.fn(async (id: string) => id !== "b");
      render(<Harness dirty={["a", "b"]} saveTab={saveTab} payload={request(["a", "b"])} />);
      await closeWindow();
      fireEvent.click(screen.getByRole("button", { name: "Save all" }));
      await screen.findByRole("alert");
      expect(screen.getByRole("alert")).toHaveTextContent("Could not save");
      expect(list()).toHaveTextContent("northsea");
      expect(list()).not.toHaveTextContent("compass");
      expect(api.confirmClose).not.toHaveBeenCalled();
    });

    it("a save that throws counts as failed", async () => {
      const failed = await saveTabs(tabs.slice(0, 2), async (id) => {
        if (id === "a") throw new Error("disk full");
        return true;
      });
      expect(failed).toEqual([tabs[0]]);
    });

    it("Discard all closes without saving", async () => {
      const saveTab = vi.fn();
      render(<Harness dirty={["a", "b"]} saveTab={saveTab} payload={request(["a", "b"])} />);
      await closeWindow();
      fireEvent.click(screen.getByRole("button", { name: "Discard all" }));
      await waitFor(() => expect(api.confirmClose).toHaveBeenCalledTimes(1));
      expect(saveTab).not.toHaveBeenCalled();
    });

    it("Cancel keeps the window and every change", async () => {
      const saveTab = vi.fn();
      render(<Harness dirty={["a"]} saveTab={saveTab} payload={request(["a"])} />);
      await closeWindow();
      fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
      await waitFor(() => expect(api.cancelClose).toHaveBeenCalledTimes(1));
      expect(api.confirmClose).not.toHaveBeenCalled();
      expect(screen.queryByTestId("window-close-dialog")).toBeNull();
    });
  });

  describe("only a running flow", () => {
    it("offers what closing a tab offers: Cancel Flow & Close or Keep running", async () => {
      render(<Harness payload={request([], ["c"])} />);
      await closeWindow();
      expect(screen.getByText("A flow is running", { selector: "h2" })).toBeInTheDocument();
      expect(list()).toHaveTextContent("kapimart");
      expect(list()).toHaveTextContent("Flow running");
      expect(screen.queryByRole("button", { name: "Save all" })).toBeNull();
      await waitFor(() =>
        expect(screen.getByRole("button", { name: "Keep running" })).toHaveFocus(),
      );
    });

    it("Cancel Flow & Close cancels the run, then closes", async () => {
      const cancelRun = vi.fn().mockResolvedValue(undefined);
      render(<Harness cancelRun={cancelRun} payload={request([], ["c"])} />);
      await closeWindow();
      fireEvent.click(screen.getByRole("button", { name: "Cancel Flow & Close" }));
      await waitFor(() => expect(api.confirmClose).toHaveBeenCalledTimes(1));
      expect(cancelRun).toHaveBeenCalledWith("c");
      expect(cancelRun.mock.invocationCallOrder[0]).toBeLessThan(
        vi.mocked(api.confirmClose).mock.invocationCallOrder[0],
      );
    });

    it("Keep running cancels the close and leaves the flow alone", async () => {
      const cancelRun = vi.fn();
      render(<Harness cancelRun={cancelRun} payload={request([], ["c"])} />);
      await closeWindow();
      fireEvent.click(screen.getByRole("button", { name: "Keep running" }));
      await waitFor(() => expect(api.cancelClose).toHaveBeenCalledTimes(1));
      expect(cancelRun).not.toHaveBeenCalled();
      expect(api.confirmClose).not.toHaveBeenCalled();
    });
  });

  describe("unsaved changes and a running flow", () => {
    it("lists both with their reasons, and Save all saves then stops the flow", async () => {
      const saveTab = vi.fn().mockResolvedValue(true);
      const cancelRun = vi.fn().mockResolvedValue(undefined);
      render(
        <Harness
          dirty={["a"]}
          saveTab={saveTab}
          cancelRun={cancelRun}
          payload={request(["a"], ["c"])}
        />,
      );
      await closeWindow();
      expect(
        screen.getByText("Unsaved changes and a running flow", { selector: "h2" }),
      ).toBeInTheDocument();
      const rows = list().querySelectorAll("li");
      expect(rows).toHaveLength(2);
      expect(rows[0]).toHaveTextContent("compass");
      expect(rows[0]).toHaveTextContent("Unsaved changes");
      expect(rows[1]).toHaveTextContent("kapimart");
      expect(rows[1]).toHaveTextContent("Flow running");

      fireEvent.click(screen.getByRole("button", { name: "Save all" }));
      await waitFor(() => expect(api.confirmClose).toHaveBeenCalledTimes(1));
      expect(saveTab.mock.calls).toEqual([["a"]]);
      expect(cancelRun).toHaveBeenCalledWith("c");
    });

    it("a failed save stops nothing and closes nothing", async () => {
      const cancelRun = vi.fn();
      render(
        <Harness
          dirty={["a"]}
          saveTab={vi.fn().mockResolvedValue(false)}
          cancelRun={cancelRun}
          payload={request(["a"], ["c"])}
        />,
      );
      await closeWindow();
      fireEvent.click(screen.getByRole("button", { name: "Save all" }));
      await screen.findByRole("alert");
      expect(cancelRun).not.toHaveBeenCalled();
      expect(api.confirmClose).not.toHaveBeenCalled();
    });
  });

  describe("a flow that finishes while the dialog is open", () => {
    it("drops its row and keeps asking about the unsaved tab", async () => {
      const payload = request(["a"], ["c"]);
      const { rerender } = render(<Harness dirty={["a"]} payload={payload} />);
      await closeWindow();
      expect(list()).toHaveTextContent("kapimart");
      rerender(<Harness dirty={["a"]} finished={["c"]} payload={payload} />);
      expect(list()).not.toHaveTextContent("kapimart");
      expect(list()).toHaveTextContent("compass");
      expect(api.confirmClose).not.toHaveBeenCalled();
    });

    it("lets the close go ahead when it was the only thing holding the window", async () => {
      const cancelRun = vi.fn();
      const payload = request([], ["c"]);
      const { rerender } = render(<Harness cancelRun={cancelRun} payload={payload} />);
      await closeWindow();
      rerender(<Harness cancelRun={cancelRun} finished={["c"]} payload={payload} />);
      await waitFor(() => expect(api.confirmClose).toHaveBeenCalledTimes(1));
      expect(cancelRun).not.toHaveBeenCalled();
      expect(screen.queryByTestId("window-close-dialog")).toBeNull();
    });
  });
});
