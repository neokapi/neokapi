import { describe, it, expect, vi } from "vite-plus/test";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { DeliveryPanel, type DeliveryConnectorStatus } from "../components/DeliveryPanel";
import type { LocaleTranslationStats } from "../types/api";

function locale(overrides: Partial<LocaleTranslationStats>): LocaleTranslationStats {
  return {
    locale: "fr-FR",
    display_name: "French (France)",
    translated_blocks: 10,
    total_blocks: 10,
    translated_words: 100,
    total_words: 100,
    percentage: 100,
    approved_blocks: 10,
    failing_checks: 0,
    ship_state: "governed",
    ...overrides,
  };
}

const connectors: DeliveryConnectorStatus[] = [
  { id: "c1", name: "Marketing WordPress", lastSync: "2026-07-17T09:12:00Z" },
  { id: "c2", name: "docs-repo", lastSync: "", lastError: "push failed: remote rejected ref" },
];

describe("DeliveryPanel", () => {
  it("renders a ship-state badge per locale", () => {
    render(
      <DeliveryPanel
        localeStats={[
          locale({}),
          locale({
            locale: "de-DE",
            display_name: "German (Germany)",
            approved_blocks: 2,
            ship_state: "ai_shippable",
          }),
        ]}
        connectors={[]}
      />,
    );
    expect(screen.getByTestId("ship-state-governed")).toBeInTheDocument();
    expect(screen.getByTestId("ship-state-ai_shippable")).toBeInTheDocument();
  });

  it("shows last sync time and never-synced per connector", () => {
    render(<DeliveryPanel localeStats={[locale({})]} connectors={connectors} />);
    expect(screen.getByText("Marketing WordPress")).toBeInTheDocument();
    expect(screen.getByText(/^Synced /)).toBeInTheDocument();
    expect(screen.getByText("Never synced")).toBeInTheDocument();
  });

  it("surfaces the connector's last delivery error", () => {
    render(<DeliveryPanel localeStats={[locale({})]} connectors={connectors} />);
    expect(screen.getByText("push failed: remote rejected ref")).toBeInTheDocument();
  });

  it("shows the empty message when no connectors exist", () => {
    render(<DeliveryPanel localeStats={[locale({})]} connectors={[]} />);
    expect(screen.getByText(/no connectors deliver this project yet/i)).toBeInTheDocument();
  });

  it("fires the connectors deep link", async () => {
    const user = userEvent.setup();
    const onOpenConnectors = vi.fn();
    render(
      <DeliveryPanel
        localeStats={[locale({})]}
        connectors={[]}
        onOpenConnectors={onOpenConnectors}
      />,
    );
    await user.click(screen.getByTestId("delivery-open-connectors"));
    expect(onOpenConnectors).toHaveBeenCalledTimes(1);
  });

  it("offers the review deep link only while something awaits review", async () => {
    const user = userEvent.setup();
    const onOpenReview = vi.fn();

    // Fully approved: no review link.
    const { rerender } = render(
      <DeliveryPanel localeStats={[locale({})]} connectors={[]} onOpenReview={onOpenReview} />,
    );
    expect(screen.queryByTestId("delivery-open-review")).toBeNull();

    // A locale with unreviewed translations: link appears and navigates.
    rerender(
      <DeliveryPanel
        localeStats={[locale({ approved_blocks: 4, ship_state: "ai_shippable" })]}
        connectors={[]}
        onOpenReview={onOpenReview}
      />,
    );
    await user.click(screen.getByTestId("delivery-open-review"));
    expect(onOpenReview).toHaveBeenCalledTimes(1);
  });

  it("falls back to the completion percentage when the server sends no ship state", () => {
    render(
      <DeliveryPanel
        localeStats={[
          locale({ ship_state: undefined, approved_blocks: undefined, percentage: 72 }),
        ]}
        connectors={[]}
      />,
    );
    expect(screen.getByText("72%")).toBeInTheDocument();
  });
});
