import { describe, it, expect } from "vite-plus/test";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { ShipStateBadge, termsNotGoverned } from "../components/ShipStateBadge";

describe("ShipStateBadge", () => {
  it("renders the established state with its label", () => {
    render(<ShipStateBadge state="established" />);
    expect(screen.getByTestId("ship-state-established")).toHaveTextContent("Established");
  });

  it("renders the translated state with its label", () => {
    render(<ShipStateBadge state="translated" />);
    expect(screen.getByTestId("ship-state-translated")).toHaveTextContent("Translated");
  });

  it("names ungoverned terminology on a locale that ships translated", async () => {
    const user = userEvent.setup();
    render(<ShipStateBadge state="translated" termsNotGoverned />);
    await user.hover(screen.getByTestId("ship-state-translated"));
    expect((await screen.findAllByText(/terminology is not governed here/)).length).toBeGreaterThan(
      0,
    );
  });

  it("names terminology with no result as the reason a locale is pending", async () => {
    const user = userEvent.setup();
    render(
      <ShipStateBadge
        state="pending"
        approvedBlocks={10}
        totalBlocks={10}
        termsNotCheckedBlocks={2}
      />,
    );
    await user.hover(screen.getByTestId("ship-state-pending"));
    expect((await screen.findAllByText(/2 with no terminology result/)).length).toBeGreaterThan(0);
  });

  it("reads ungoverned terminology off the compliance basis", () => {
    expect(termsNotGoverned({ compliance_basis: "checks" })).toBe(true);
    expect(termsNotGoverned({ compliance_basis: "voice+checks" })).toBe(true);
    expect(termsNotGoverned({ compliance_basis: "checks+terms" })).toBe(false);
    expect(termsNotGoverned({ compliance_basis: "voice+checks+terms" })).toBe(false);
    expect(termsNotGoverned({})).toBe(false);
  });

  it("renders the pending state with its label", () => {
    render(<ShipStateBadge state="pending" />);
    expect(screen.getByTestId("ship-state-pending")).toHaveTextContent("Pending");
  });

  it("compact variant renders icon-only with an accessible label", () => {
    render(<ShipStateBadge state="established" compact />);
    const badge = screen.getByTestId("ship-state-established");
    expect(badge).toHaveAttribute("aria-label", "Established");
    expect(badge).not.toHaveTextContent("Established");
  });

  it("shows the explanation and count details in the tooltip on hover", async () => {
    const user = userEvent.setup();
    render(
      <ShipStateBadge state="translated" approvedBlocks={12} totalBlocks={50} failingChecks={0} />,
    );
    await user.hover(screen.getByTestId("ship-state-translated"));
    const tip = await screen.findAllByText(/AI-shippable/i);
    expect(tip.length).toBeGreaterThan(0);
    expect((await screen.findAllByText(/12 of 50 blocks established/)).length).toBeGreaterThan(0);
  });

  it("mentions failing checks in the tooltip when present", async () => {
    const user = userEvent.setup();
    render(
      <ShipStateBadge state="pending" approvedBlocks={3} totalBlocks={10} failingChecks={2} />,
    );
    await user.hover(screen.getByTestId("ship-state-pending"));
    expect((await screen.findAllByText(/2 failing checks/)).length).toBeGreaterThan(0);
  });

  it("names what the stale pairs are waiting on", async () => {
    const user = userEvent.setup();
    render(
      <ShipStateBadge
        state="pending"
        approvedBlocks={40}
        totalBlocks={50}
        staleAwaitingDraft={6}
        staleAwaitingReview={4}
      />,
    );
    await user.hover(screen.getByTestId("ship-state-pending"));
    expect((await screen.findAllByText(/6 stale, awaiting a draft/)).length).toBeGreaterThan(0);
    expect((await screen.findAllByText(/4 re-drafted, awaiting review/)).length).toBeGreaterThan(0);
  });

  it("names the pairs a reviewer turned down", async () => {
    const user = userEvent.setup();
    render(
      <ShipStateBadge
        state="pending"
        approvedBlocks={40}
        totalBlocks={50}
        rejectedAwaitingDraft={3}
      />,
    );
    await user.hover(screen.getByTestId("ship-state-pending"));
    expect((await screen.findAllByText(/3 turned down, awaiting a draft/)).length).toBeGreaterThan(
      0,
    );
    expect(screen.queryByText(/stale, awaiting a draft/)).toBeNull();
  });

  it("omits a half of the stale split that holds nothing", async () => {
    const user = userEvent.setup();
    render(
      <ShipStateBadge
        state="pending"
        approvedBlocks={40}
        totalBlocks={50}
        staleAwaitingDraft={0}
        staleAwaitingReview={4}
      />,
    );
    await user.hover(screen.getByTestId("ship-state-pending"));
    expect((await screen.findAllByText(/4 re-drafted, awaiting review/)).length).toBeGreaterThan(0);
    expect(screen.queryByText(/awaiting a draft/)).toBeNull();
  });
});
