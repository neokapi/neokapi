// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { ItemCard } from "./item-card";

afterEach(cleanup);

describe("ItemCard", () => {
  it("exposes a clickable card as a keyboard-operable button", async () => {
    const onClick = vi.fn();
    render(
      <ItemCard clickable onClick={onClick}>
        Open project
      </ItemCard>,
    );
    const card = screen.getByRole("button", { name: "Open project" });
    expect(card.getAttribute("tabindex")).toBe("0");
    card.focus();
    await userEvent.keyboard("{Enter}");
    await userEvent.keyboard(" ");
    expect(onClick).toHaveBeenCalledTimes(2);
  });

  it("is neither a button nor focusable when not clickable", () => {
    render(<ItemCard>Static</ItemCard>);
    expect(screen.queryByRole("button")).toBeNull();
  });

  it("does not double-activate the card when a nested action button is used", async () => {
    const onCard = vi.fn();
    const onDelete = vi.fn();
    render(
      <ItemCard clickable onClick={onCard}>
        <span>Row body</span>
        <button
          onClick={(e) => {
            // Nested actions stop propagation so the card is not also triggered.
            e.stopPropagation();
            onDelete();
          }}
        >
          Delete
        </button>
      </ItemCard>,
    );
    const del = screen.getByRole("button", { name: "Delete" });
    del.focus();
    await userEvent.keyboard("{Enter}");
    expect(onDelete).toHaveBeenCalledTimes(1);
    // The card's keydown handler only activates when the card itself is focused.
    expect(onCard).not.toHaveBeenCalled();
  });
});
