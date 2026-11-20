import { beforeEach, describe, expect, it, vi } from "vite-plus/test";
import { act, fireEvent, render, screen, waitFor } from "./testUtils";
import { ActiveFilterProvider, useActiveFilter } from "../context/ActiveFilterContext";
import { api } from "../hooks/useApi";

function Selection({ onError }: { onError: (error: unknown) => void }) {
  const { activeId, setActive } = useActiveFilter();
  return (
    <>
      <output>{activeId}</output>
      <button onClick={() => void setActive("draft").catch(onError)}>Select</button>
    </>
  );
}

describe("saved filter selection", () => {
  let rejectSelection: (error: Error) => void;
  const onError = vi.fn();
  const tree = (tab: string) => (
    <ActiveFilterProvider tabID={tab} enabled>
      <Selection onError={onError} />
    </ActiveFilterProvider>
  );

  beforeEach(() => {
    vi.restoreAllMocks();
    onError.mockClear();
    vi.spyOn(api, "getProjectFilters").mockImplementation(async (tab) => ({
      active: tab,
      filters: [
        { id: tab, name: tab },
        { id: "draft", name: "Draft" },
      ],
    }));
    vi.spyOn(api, "setActiveFilter").mockImplementation(
      () =>
        new Promise((_, reject) => {
          rejectSelection = reject;
        }),
    );
  });

  it("restores the saved filter after a rejected selection", async () => {
    render(tree("first"));
    await screen.findByText("first");
    fireEvent.click(screen.getByRole("button"));
    await screen.findByText("draft");
    await act(() => rejectSelection(new Error("write failed")));
    await screen.findByText("first");
    expect(onError).toHaveBeenCalledOnce();
  });

  it("keeps a pending selection in the project that requested it", async () => {
    const view = render(tree("first"));
    await screen.findByText("first");
    fireEvent.click(screen.getByRole("button"));
    await screen.findByText("draft");
    vi.mocked(api.getProjectFilters).mockImplementation(() => new Promise(() => {}));
    view.rerender(tree("second"));
    expect(screen.getByRole("status").textContent).toBe("");
    await act(() => rejectSelection(new Error("first project closed")));
    await waitFor(() => expect(onError).toHaveBeenCalledOnce());
    expect(screen.getByRole("status").textContent).toBe("");
  });
});
