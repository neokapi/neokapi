import { describe, it, expect } from "vite-plus/test";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { ShipStateBadge, termsNotGoverned } from "../components/ShipStateBadge";

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

  it("renders the approved state with its label", () => {
    render(<ShipStateBadge state="approved" />);
    expect(screen.getByTestId("ship-state-approved")).toHaveTextContent("Approved");
  });

  it("says why an approved locale is not governed", async () => {
    const user = userEvent.setup();
    render(
      <ShipStateBadge state="approved" approvedBlocks={10} totalBlocks={10} termsNotGoverned />,
    );
    await user.hover(screen.getByTestId("ship-state-approved"));
    expect(
      (await screen.findAllByText(/No terms apply to this language, so it is not governed/)).length,
    ).toBeGreaterThan(0);
  });

  it("names ungoverned terminology on a locale that ships on machine review", async () => {
    const user = userEvent.setup();
    render(<ShipStateBadge state="ai_shippable" termsNotGoverned />);
    await user.hover(screen.getByTestId("ship-state-ai_shippable"));
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
