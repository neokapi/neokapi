// @vitest-environment jsdom
import React from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import GateOverlay from "./GateOverlay";
import type { RunGate } from "./useRunGate";

vi.mock("./RunGate", () => ({ default: () => <button>Run experiment</button> }));
afterEach(cleanup);
const gate: RunGate = {
  armed: false,
  ready: false,
  status: "idle",
  bootProgress: null,
  error: null,
  requires: [],
  run: () => {},
};

function Example({
  ready = false,
  extra = false,
}: {
  ready?: boolean;
  extra?: boolean;
}): React.ReactElement {
  return (
    <div>
      <div data-testid="preview">
        <button>Covered action</button>
      </div>
      <div data-testid="already-inert" inert>
        <button>Disabled content</button>
      </div>
      {extra && <button data-testid="late">Late action</button>}
      <GateOverlay gate={{ ...gate, ready }} />
    </div>
  );
}

describe("GateOverlay", () => {
  it("makes covered controls inert and restores their previous state on ready", () => {
    const { rerender } = render(<Example />);
    expect(screen.getByTestId("preview").hasAttribute("inert")).toBe(true);
    expect(screen.getByRole("button", { name: "Run experiment" }).closest("[inert]")).toBeNull();
    rerender(<Example ready />);
    expect(screen.getByTestId("preview").hasAttribute("inert")).toBe(false);
    expect(screen.getByTestId("already-inert").hasAttribute("inert")).toBe(true);
  });

  it("also protects content added during loading", async () => {
    const { rerender } = render(<Example />);
    rerender(<Example extra />);
    await waitFor(() => expect(screen.getByTestId("late").hasAttribute("inert")).toBe(true));
  });
});
