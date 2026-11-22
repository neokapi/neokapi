import { beforeEach, describe, expect, it, vi } from "vite-plus/test";
import { act, fireEvent, render, renderHook, screen, waitFor } from "./testUtils";
import { saveTabs, useWindowCloseGuard, type UnsavedTab } from "../hooks/useWindowCloseGuard";
import { WindowCloseDialog } from "../components/WindowCloseDialog";
import { api } from "../hooks/useApi";

const tabs: UnsavedTab[] = [
  { id: "a", name: "compass" },
  { id: "b", name: "northsea" },
  { id: "c", name: "kapimart" },
];

function setup(dirty: string[], saveTab: (id: string) => Promise<boolean>) {
  return renderHook(() =>
    useWindowCloseGuard({ tabs, hasChanges: (id) => dirty.includes(id), saveTab }),
  );
}

/** The guard and its dialog together, as App renders them. */
function Harness({
  dirty,
  saveTab,
}: {
  dirty: string[];
  saveTab: (id: string) => Promise<boolean>;
}) {
  const guard = useWindowCloseGuard({ tabs, hasChanges: (id) => dirty.includes(id), saveTab });
  return (
    <>
      <button type="button" onClick={() => guard.onCloseRequested()}>
        close window
      </button>
      {guard.request && (
        <WindowCloseDialog
          tabs={guard.request}
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

describe("closing the window with unsaved changes", () => {
  beforeEach(() => {
    vi.spyOn(api, "setUnsavedTabs").mockResolvedValue(undefined);
    vi.spyOn(api, "confirmClose").mockResolvedValue(undefined);
    vi.spyOn(api, "cancelClose").mockResolvedValue(undefined);
  });

  it("reports the tabs with unsaved changes to the backend", async () => {
    setup(["a", "c"], vi.fn());
    await waitFor(() => expect(api.setUnsavedTabs).toHaveBeenCalledWith(["a", "c"]));
  });

  it("reports an empty list when nothing is unsaved", async () => {
    setup([], vi.fn());
    await waitFor(() => expect(api.setUnsavedTabs).toHaveBeenCalledWith([]));
  });

  it("lets a stale request close at once when no tab has changes", async () => {
    const { result } = setup([], vi.fn());
    act(() => result.current.onCloseRequested());
    expect(result.current.request).toBeNull();
    await waitFor(() => expect(api.confirmClose).toHaveBeenCalledTimes(1));
  });

  it("lists every tab with unsaved changes in one dialog", () => {
    render(<Harness dirty={["a", "b"]} saveTab={vi.fn()} />);
    fireEvent.click(screen.getByText("close window"));
    const list = screen.getByRole("list", { name: "Projects with unsaved changes" });
    expect(list).toHaveTextContent("compass");
    expect(list).toHaveTextContent("northsea");
    expect(list).not.toHaveTextContent("kapimart");
    expect(screen.getByRole("button", { name: "Save all" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Discard all" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Cancel" })).toBeInTheDocument();
  });

  it("Save all saves each listed tab, then closes", async () => {
    const saveTab = vi.fn().mockResolvedValue(true);
    render(<Harness dirty={["a", "b"]} saveTab={saveTab} />);
    fireEvent.click(screen.getByText("close window"));
    fireEvent.click(screen.getByRole("button", { name: "Save all" }));
    await waitFor(() => expect(api.confirmClose).toHaveBeenCalledTimes(1));
    expect(saveTab.mock.calls).toEqual([["a"], ["b"]]);
    expect(screen.queryByTestId("window-close-dialog")).toBeNull();
  });

  it("a failed save keeps the window open and names the tab", async () => {
    const saveTab = vi.fn(async (id: string) => id !== "b");
    render(<Harness dirty={["a", "b"]} saveTab={saveTab} />);
    fireEvent.click(screen.getByText("close window"));
    fireEvent.click(screen.getByRole("button", { name: "Save all" }));
    await screen.findByRole("alert");
    expect(screen.getByRole("alert")).toHaveTextContent("Could not save northsea");
    // compass saved, so only northsea is still listed, marked as not saved.
    const list = screen.getByRole("list", { name: "Projects with unsaved changes" });
    expect(list).toHaveTextContent("northsea");
    expect(list).not.toHaveTextContent("compass");
    expect(saveTab.mock.calls).toEqual([["a"], ["b"]]);
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
    render(<Harness dirty={["a", "b"]} saveTab={saveTab} />);
    fireEvent.click(screen.getByText("close window"));
    fireEvent.click(screen.getByRole("button", { name: "Discard all" }));
    await waitFor(() => expect(api.confirmClose).toHaveBeenCalledTimes(1));
    expect(saveTab).not.toHaveBeenCalled();
  });

  it("Cancel keeps the window and every change", async () => {
    const saveTab = vi.fn();
    render(<Harness dirty={["a"]} saveTab={saveTab} />);
    fireEvent.click(screen.getByText("close window"));
    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
    await waitFor(() => expect(api.cancelClose).toHaveBeenCalledTimes(1));
    expect(api.confirmClose).not.toHaveBeenCalled();
    expect(saveTab).not.toHaveBeenCalled();
    expect(screen.queryByTestId("window-close-dialog")).toBeNull();
  });
});
