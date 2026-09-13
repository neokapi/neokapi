import { describe, it, expect } from "vite-plus/test";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { TooltipProvider } from "@neokapi/ui-primitives";

import { ComplianceRateChip } from "../components/ComplianceRateChip";

function renderChip(props: Parameters<typeof ComplianceRateChip>[0]) {
  return render(
    <TooltipProvider>
      <ComplianceRateChip {...props} />
    </TooltipProvider>,
  );
}

describe("ComplianceRateChip", () => {
  it("names not-checked and not-governed blocks apart, beside the rate", () => {
    renderChip({
      rate: 0.5,
      basis: "voice+checks",
      compliantBlocks: 2,
      translatedBlocks: 7,
      notCheckedBlocks: 2,
      notGovernedBlocks: 1,
    });
    const chip = screen.getByTestId("compliant-rate");
    expect(within(chip).getByText("50% compliant")).toBeInTheDocument();
    expect(within(chip).getByTestId("compliance-not-checked").textContent).toBe("2 not checked");
    expect(within(chip).getByTestId("compliance-not-governed").textContent).toBe("1 not governed");
  });

  it("shows the counts alone when no block has a verdict", () => {
    renderChip({ basis: "checks", translatedBlocks: 4, notGovernedBlocks: 4 });
    const chip = screen.getByTestId("compliant-rate");
    expect(chip.textContent).not.toContain("%");
    expect(within(chip).queryByTestId("compliance-not-checked")).toBeNull();
    expect(within(chip).getByTestId("compliance-not-governed").textContent).toBe("4 not governed");
  });

  it("explains each count in words of its own", async () => {
    const user = userEvent.setup();
    renderChip({
      rate: 1,
      basis: "voice+checks+terms",
      compliantBlocks: 3,
      translatedBlocks: 6,
      notCheckedBlocks: 2,
      notGovernedBlocks: 1,
    });
    await user.hover(screen.getByTestId("compliant-rate"));
    // SimpleTooltip renders duplicate (trigger + portal) content.
    expect(
      (await screen.findAllByText(/no result for a check that applies to this language/i)).length,
    ).toBeGreaterThan(0);
    expect(
      (await screen.findAllByText(/so only the rule-based checks judged it/i)).length,
    ).toBeGreaterThan(0);
    expect((await screen.findAllByText(/3 of 3 judged blocks compliant/i)).length).toBeGreaterThan(
      0,
    );
  });
});
