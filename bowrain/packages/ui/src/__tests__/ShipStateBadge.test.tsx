import { describe, it, expect } from "vite-plus/test";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { ShipStateBadge } from "../components/ShipStateBadge";

describe("ShipStateBadge", () => {
  it("renders the governed state with its label", () => {
    render(<ShipStateBadge state="governed" />);
    const badge = screen.getByTestId("ship-state-governed");
    expect(badge).toHaveTextContent("Governed");
  });

  it("renders the ai_shippable state with its label", () => {
    render(<ShipStateBadge state="ai_shippable" />);
    expect(screen.getByTestId("ship-state-ai_shippable")).toHaveTextContent("AI-shippable");
  });

  it("renders the pending state with its label", () => {
    render(<ShipStateBadge state="pending" />);
    expect(screen.getByTestId("ship-state-pending")).toHaveTextContent("Pending");
  });

  it("compact variant renders icon-only with an accessible label", () => {
    render(<ShipStateBadge state="governed" compact />);
    const badge = screen.getByTestId("ship-state-governed");
    expect(badge).toHaveAttribute("aria-label", "Governed");
    expect(badge).not.toHaveTextContent("Governed");
  });

  it("shows the explanation and count details in the tooltip on hover", async () => {
    const user = userEvent.setup();
    render(
      <ShipStateBadge
        state="ai_shippable"
        approvedBlocks={12}
        totalBlocks={50}
        failingChecks={0}
      />,
    );
    await user.hover(screen.getByTestId("ship-state-ai_shippable"));
    const tip = await screen.findAllByText(/machine review only/i);
    expect(tip.length).toBeGreaterThan(0);
    expect((await screen.findAllByText(/12 of 50 blocks human-approved/)).length).toBeGreaterThan(
      0,
    );
  });

  it("mentions failing checks in the tooltip when present", async () => {
    const user = userEvent.setup();
    render(
      <ShipStateBadge state="pending" approvedBlocks={3} totalBlocks={10} failingChecks={2} />,
    );
    await user.hover(screen.getByTestId("ship-state-pending"));
    expect((await screen.findAllByText(/2 failing checks/)).length).toBeGreaterThan(0);
  });
});
