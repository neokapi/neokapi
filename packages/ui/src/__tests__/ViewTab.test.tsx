// @vitest-environment jsdom
import { useState } from "react";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ViewTab, ViewTabGroup } from "../components/ui/view-tab";

afterEach(cleanup);

function Switch() {
  const [view, setView] = useState<"a" | "b">("a");
  return (
    <ViewTabGroup aria-label="Reading">
      <ViewTab active={view === "a"} onClick={() => setView("a")}>
        First
      </ViewTab>
      <ViewTab active={view === "b"} onClick={() => setView("b")}>
        Second
      </ViewTab>
    </ViewTabGroup>
  );
}

describe("ViewTab", () => {
  it("marks the active choice with aria-pressed and switches on click", async () => {
    render(<Switch />);
    const group = screen.getByRole("group", { name: "Reading" });
    const first = screen.getByRole("button", { name: "First" });
    const second = screen.getByRole("button", { name: "Second" });
    expect(group.contains(first)).toBe(true);
    expect(first.getAttribute("aria-pressed")).toBe("true");
    expect(second.getAttribute("aria-pressed")).toBe("false");

    await userEvent.click(second);
    expect(first.getAttribute("aria-pressed")).toBe("false");
    expect(second.getAttribute("aria-pressed")).toBe("true");
  });

  it("renders as a plain button so a click reaches the handler", async () => {
    const onClick = vi.fn();
    render(
      <ViewTab active={false} onClick={onClick}>
        Only
      </ViewTab>,
    );
    const btn = screen.getByRole("button", { name: "Only" });
    expect(btn.getAttribute("type")).toBe("button");
    await userEvent.click(btn);
    expect(onClick).toHaveBeenCalledTimes(1);
  });
});
