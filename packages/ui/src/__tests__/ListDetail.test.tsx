// @vitest-environment jsdom
import {
  act,
  cleanup,
  fireEvent,
  render,
  renderHook,
  screen,
  within,
} from "@testing-library/react";
import { useState } from "react";
import { afterEach, beforeAll, describe, expect, it, vi } from "vitest";
import {
  ListDetail,
  ListDetailEmpty,
  ListDetailGroup,
  ListDetailItem,
  ListDetailList,
  ListDetailPane,
  ListDetailToolbar,
} from "../components/ui/list-detail";
import { navigationTarget, useListNavigation } from "../hooks/useListNavigation";

beforeAll(() => {
  if (typeof globalThis.ResizeObserver === "undefined") {
    globalThis.ResizeObserver = class {
      observe() {}
      unobserve() {}
      disconnect() {}
    };
  }
  Element.prototype.scrollIntoView ??= () => {};
});

afterEach(cleanup);

/** The What's new list: a disagreement, suggestions, and one established rule. */
function Suggestions({
  initial = null,
  onSelect,
}: {
  initial?: string | null;
  onSelect?: (id: string) => void;
}) {
  const [selected, setSelected] = useState<string | null>(initial);
  return (
    <ListDetail
      list={
        <ListDetailList
          aria-label="Suggestions"
          selectedId={selected}
          onSelect={(id) => {
            onSelect?.(id);
            setSelected(id);
          }}
        >
          <ListDetailGroup label="Needs you" count={1}>
            <ListDetailItem id="c1">sign in or log in</ListDetailItem>
          </ListDetailGroup>
          <ListDetailGroup label="Suggested" count={2}>
            <ListDetailItem id="s1">KapiMart, not Kapi Mart</ListDetailItem>
            <ListDetailItem id="s2">
              PayPal, not Paypal <button type="button">More</button>
            </ListDetailItem>
          </ListDetailGroup>
          <ListDetailGroup label="Established" count={1} />
          <ListDetailItem id="e1">studio, not business</ListDetailItem>
        </ListDetailList>
      }
      detail={
        selected ? (
          <ListDetailPane toolbar={<ListDetailToolbar title={selected} />}>Detail</ListDetailPane>
        ) : (
          <ListDetailEmpty title="Choose a suggestion to see it here" />
        )
      }
    />
  );
}

function listbox() {
  return screen.getByRole("listbox", { name: "Suggestions" });
}

function selectedText() {
  const selected = within(listbox())
    .getAllByRole("option")
    .filter((o) => o.getAttribute("aria-selected") === "true");
  return selected.map((o) => o.textContent);
}

function press(key: string, init: Partial<KeyboardEventInit> = {}) {
  return fireEvent.keyDown(listbox(), { key, ...init });
}

describe("ListDetailList", () => {
  it("is a focusable listbox of options with nothing selected to begin with", () => {
    render(<Suggestions />);
    const list = listbox();
    expect(list.tabIndex).toBe(0);
    expect(within(list).getAllByRole("option")).toHaveLength(4);
    expect(selectedText()).toEqual([]);
    expect(list.getAttribute("aria-activedescendant")).toBeNull();
    expect(screen.getByText("Choose a suggestion to see it here")).toBeTruthy();
  });

  it("moves the selection with the arrow keys, and selects the first row from nothing", () => {
    render(<Suggestions />);
    press("ArrowDown");
    expect(selectedText()).toEqual(["sign in or log in"]);
    press("ArrowDown");
    expect(selectedText()).toEqual(["KapiMart, not Kapi Mart"]);
    press("ArrowUp");
    expect(selectedText()).toEqual(["sign in or log in"]);
  });

  it("moves the selection with j and k", () => {
    render(<Suggestions />);
    press("j");
    press("j");
    press("j");
    expect(selectedText()).toEqual(["PayPal, not Paypal More"]);
    press("k");
    expect(selectedText()).toEqual(["KapiMart, not Kapi Mart"]);
  });

  it("jumps to the ends with Home and End, and does not wrap past them", () => {
    render(<Suggestions initial="s1" />);
    press("End");
    expect(selectedText()).toEqual(["studio, not business"]);
    press("ArrowDown");
    expect(selectedText()).toEqual(["studio, not business"]);
    press("Home");
    expect(selectedText()).toEqual(["sign in or log in"]);
    press("ArrowUp");
    expect(selectedText()).toEqual(["sign in or log in"]);
  });

  it("selects the focused row with Enter and Space", () => {
    const onSelect = vi.fn();
    render(<Suggestions onSelect={onSelect} />);
    press("Enter");
    expect(onSelect).toHaveBeenLastCalledWith("c1");
    press("ArrowDown");
    press(" ");
    expect(onSelect).toHaveBeenLastCalledWith("s1");
    expect(selectedText()).toEqual(["KapiMart, not Kapi Mart"]);
  });

  it("keeps aria-selected and aria-activedescendant on the selected row", () => {
    render(<Suggestions />);
    press("ArrowDown");
    press("ArrowDown");
    const list = listbox();
    const options = within(list).getAllByRole("option");
    const active = list.getAttribute("aria-activedescendant");
    expect(active).toBe(options[1].id);
    expect(document.getElementById(active ?? "")?.textContent).toBe("KapiMart, not Kapi Mart");
    expect(options.map((o) => o.getAttribute("aria-selected"))).toEqual([
      "false",
      "true",
      "false",
      "false",
    ]);
  });

  it("selects a row that is clicked", () => {
    render(<Suggestions />);
    fireEvent.click(screen.getByText("studio, not business"));
    expect(selectedText()).toEqual(["studio, not business"]);
  });

  it("never selects a group header, by click or by key", () => {
    const onSelect = vi.fn();
    render(<Suggestions initial="c1" onSelect={onSelect} />);
    const header = screen.getByText("Suggested");
    expect(header.closest("[role=option]")).toBeNull();
    fireEvent.click(header);
    expect(onSelect).not.toHaveBeenCalled();
    // From the last row of one group the next key lands on the next group's first row.
    press("ArrowDown");
    expect(onSelect).toHaveBeenLastCalledWith("s1");
    press("ArrowDown");
    press("ArrowDown");
    expect(onSelect).toHaveBeenLastCalledWith("e1");
  });

  it("names each wrapping group by its label", () => {
    render(<Suggestions />);
    expect(within(listbox()).getAllByRole("group")).toHaveLength(2);
    const needsYou = within(listbox()).getByRole("group", { name: "Needs you" });
    expect(
      within(needsYou)
        .getAllByRole("option")
        .map((o) => o.textContent),
    ).toEqual(["sign in or log in"]);
    expect(within(listbox()).getByRole("group", { name: "Suggested" })).toBeTruthy();
  });

  it("does nothing on any other letter, and leaves the key to the page", () => {
    const onSelect = vi.fn();
    render(<Suggestions initial="s1" onSelect={onSelect} />);
    for (const key of ["a", "c", "K", "x"]) {
      const notCancelled = press(key);
      expect(notCancelled, key).toBe(true);
    }
    expect(onSelect).not.toHaveBeenCalled();
    expect(selectedText()).toEqual(["KapiMart, not Kapi Mart"]);
  });

  it("leaves a key held with Cmd, Ctrl or Alt to the page", () => {
    const onSelect = vi.fn();
    render(<Suggestions initial="s1" onSelect={onSelect} />);
    press("j", { metaKey: true });
    press("ArrowDown", { ctrlKey: true });
    press("k", { altKey: true });
    expect(onSelect).not.toHaveBeenCalled();
  });

  it("prevents the default of the keys it handles, so Space does not scroll the pane", () => {
    render(<Suggestions />);
    expect(press(" ")).toBe(false);
    expect(press("ArrowDown")).toBe(false);
  });

  it("leaves a key pressed on a control inside a row to that control", () => {
    const onSelect = vi.fn();
    render(<Suggestions initial="s2" onSelect={onSelect} />);
    fireEvent.keyDown(screen.getByRole("button", { name: "More" }), { key: "Enter" });
    fireEvent.keyDown(screen.getByRole("button", { name: "More" }), { key: "ArrowDown" });
    expect(onSelect).not.toHaveBeenCalled();
  });
});

describe("ListDetail panes", () => {
  it("draws the list and the detail side by side, the detail's toolbar as a heading", () => {
    const { container } = render(<Suggestions initial="s1" />);
    expect(container.querySelector("[data-pane=list]")).not.toBeNull();
    expect(container.querySelector("[data-pane=detail]")).not.toBeNull();
    expect(screen.getByRole("heading", { level: 2 }).textContent).toBe("s1");
  });

  it("puts the toolbar's actions beside its title", () => {
    render(
      <ListDetailPane
        toolbar={
          <ListDetailToolbar
            title="KapiMart, not Kapi Mart"
            actions={
              <>
                <button type="button">Keep</button>
                <button type="button">Change</button>
                <button type="button">Drop</button>
              </>
            }
          />
        }
      >
        Seen in src/en/store.properties
      </ListDetailPane>,
    );
    const toolbar = screen.getByRole("heading", { name: "KapiMart, not Kapi Mart" }).parentElement;
    expect(
      within(toolbar as HTMLElement)
        .getAllByRole("button")
        .map((b) => b.textContent),
    ).toEqual(["Keep", "Change", "Drop"]);
  });
});

describe("useListNavigation", () => {
  it("drives a selection from outside a list", () => {
    let selected: string | null = null;
    const onSelect = vi.fn((id: string) => {
      selected = id;
    });
    const { result, rerender } = renderHook(() =>
      useListNavigation({ ids: ["a", "b", "c"], selectedId: selected, onSelect }),
    );
    act(() => result.current.next());
    expect(selected).toBe("a");
    rerender();
    act(() => result.current.last());
    expect(selected).toBe("c");
    rerender();
    act(() => result.current.previous());
    expect(selected).toBe("b");
    rerender();
    expect(result.current.handleKey({ key: "q" } as KeyboardEvent)).toBe(false);
    expect(result.current.handleKey({ key: "j" } as KeyboardEvent)).toBe(true);
    expect(selected).toBe("c");
  });

  it("keeps one identity across renders", () => {
    const { result, rerender } = renderHook(
      ({ selectedId }) => useListNavigation({ ids: ["a"], selectedId, onSelect: () => {} }),
      { initialProps: { selectedId: null as string | null } },
    );
    const first = result.current;
    rerender({ selectedId: "a" });
    expect(result.current).toBe(first);
  });
});

describe("navigationTarget", () => {
  const ids = ["a", "b", "c"];
  it("lands on the first or the last from nothing", () => {
    expect(navigationTarget(ids, null, "next")).toBe("a");
    expect(navigationTarget(ids, null, "previous")).toBe("c");
    expect(navigationTarget(ids, "gone", "next")).toBe("a");
    expect(navigationTarget(ids, null, "current")).toBe("a");
  });
  it("stops at the ends", () => {
    expect(navigationTarget(ids, "c", "next")).toBe("c");
    expect(navigationTarget(ids, "a", "previous")).toBe("a");
  });
  it("has nowhere to go in an empty list", () => {
    expect(navigationTarget([], null, "next")).toBeUndefined();
  });
});
