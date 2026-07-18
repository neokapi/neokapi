import { describe, it, expect } from "vite-plus/test";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { TranslationDashboard } from "../components/TranslationDashboard";
import {
  sampleDashboardStats,
  shipStateDashboardStats,
  onBrandDashboardStats,
} from "../stories/fixtures";
import type { TranslationDashboardStats } from "../types/api";

describe("TranslationDashboard", () => {
  it("shows empty state when stats is null", () => {
    render(<TranslationDashboard stats={null} projectName="Test" />);
    expect(screen.getByText(/no translation data yet/i)).toBeInTheDocument();
  });

  it("renders the project name in the header", () => {
    render(<TranslationDashboard stats={sampleDashboardStats} projectName="Demo App" />);
    expect(screen.getByText(/Demo App — Translation Dashboard/)).toBeInTheDocument();
  });

  it("renders generic header when no project name given", () => {
    render(<TranslationDashboard stats={sampleDashboardStats} />);
    expect(screen.getByText("Translation Dashboard")).toBeInTheDocument();
  });

  it("renders all four summary stat cards", () => {
    render(<TranslationDashboard stats={sampleDashboardStats} />);
    expect(screen.getByText("Source Words")).toBeInTheDocument();
    expect(screen.getByText("Translatable Blocks")).toBeInTheDocument();
    expect(screen.getByText("Target Languages")).toBeInTheDocument();
    expect(screen.getByText("Overall Completion")).toBeInTheDocument();
  });

  it("shows correct target language count", () => {
    render(<TranslationDashboard stats={sampleDashboardStats} />);
    // sampleDashboardStats has 3 locales
    expect(screen.getByText("3")).toBeInTheDocument();
  });

  it("shows overall completion percentage in header", () => {
    render(<TranslationDashboard stats={sampleDashboardStats} />);
    // The header shows "XX% complete"
    expect(screen.getByText(/% complete$/)).toBeInTheDocument();
  });

  it("renders file progress table with all items", () => {
    render(<TranslationDashboard stats={sampleDashboardStats} />);
    expect(screen.getByText("messages")).toBeInTheDocument();
    expect(screen.getByText("ui-strings")).toBeInTheDocument();
    expect(screen.getByText("landing-page")).toBeInTheDocument();
  });

  it("renders collection heatmap", () => {
    render(<TranslationDashboard stats={sampleDashboardStats} />);
    expect(screen.getByText("Collection Progress")).toBeInTheDocument();
    expect(screen.getByText("Default")).toBeInTheDocument();
    expect(screen.getByText("Website")).toBeInTheDocument();
  });

  it("renders chart titles", () => {
    render(<TranslationDashboard stats={sampleDashboardStats} />);
    expect(screen.getByText("Completion by Language")).toBeInTheDocument();
    expect(screen.getByText("Word Count by Language")).toBeInTheDocument();
  });

  it("hides charts when no locale stats", () => {
    const empty: TranslationDashboardStats = {
      ...sampleDashboardStats,
      locale_stats: [],
    };
    render(<TranslationDashboard stats={empty} />);
    expect(screen.queryByText("Completion by Language")).toBeNull();
    expect(screen.queryByText("Word Count by Language")).toBeNull();
  });

  it("hides collection heatmap when no collections", () => {
    const noColl: TranslationDashboardStats = {
      ...sampleDashboardStats,
      collection_stats: [],
    };
    render(<TranslationDashboard stats={noColl} />);
    expect(screen.queryByText("Collection Progress")).toBeNull();
  });

  it("hides file table when no items", () => {
    const noItems: TranslationDashboardStats = {
      ...sampleDashboardStats,
      item_stats: [],
    };
    render(<TranslationDashboard stats={noItems} />);
    expect(screen.queryByText("File Progress")).toBeNull();
  });

  it("renders the ship-readiness band when the server derives ship states", () => {
    render(<TranslationDashboard stats={shipStateDashboardStats} />);
    const card = screen.getByTestId("ship-readiness");
    expect(within(card).getByText("Ship readiness")).toBeInTheDocument();
    expect(within(card).getByTestId("ship-state-governed")).toBeInTheDocument();
    expect(within(card).getByTestId("ship-state-ai_shippable")).toBeInTheDocument();
    expect(within(card).getByTestId("ship-state-pending")).toBeInTheDocument();
  });

  it("hides the ship-readiness band for legacy stats without ship states", () => {
    render(<TranslationDashboard stats={sampleDashboardStats} />);
    expect(screen.queryByTestId("ship-readiness")).toBeNull();
  });

  it("shows compact rollup indicators in the collection heatmap", () => {
    render(<TranslationDashboard stats={shipStateDashboardStats} />);
    const heatmap = screen.getByText("Collection Progress").closest("[data-slot=card]")!;
    // Both collections carry a pending ja-JP rollup.
    expect(
      within(heatmap as HTMLElement).getAllByTestId("ship-state-pending").length,
    ).toBeGreaterThan(0);
  });

  it("renders an on-brand rate chip per locale when the server derives it", () => {
    render(<TranslationDashboard stats={onBrandDashboardStats} />);
    const card = screen.getByTestId("ship-readiness");
    const chips = within(card).getAllByTestId("on-brand-rate");
    // Every locale in the fixture has translated blocks, so every row gets a chip.
    expect(chips.length).toBe(onBrandDashboardStats.locale_stats.length);
    expect(within(card).getByText("92% on-brand")).toBeInTheDocument();
    // The basis rides on the chip so tests (and analytics) can tell voice-informed
    // rates from checks-only ones.
    expect(chips.some((c) => c.dataset.basis === "voice+checks")).toBe(true);
    expect(chips.some((c) => c.dataset.basis === "checks")).toBe(true);
  });

  it("hides the on-brand rate chip when servers do not send the field", () => {
    render(<TranslationDashboard stats={shipStateDashboardStats} />);
    expect(screen.queryByTestId("on-brand-rate")).toBeNull();
  });

  it("explains the on-brand basis in the chip tooltip", async () => {
    const user = userEvent.setup();
    render(<TranslationDashboard stats={onBrandDashboardStats} />);
    const card = screen.getByTestId("ship-readiness");
    const voiceChip = within(card)
      .getAllByTestId("on-brand-rate")
      .find((c) => c.dataset.basis === "voice+checks")!;
    await user.hover(voiceChip);
    // SimpleTooltip renders duplicate (trigger + portal) content.
    expect(
      (await screen.findAllByText(/brand voice scores measured against/i)).length,
    ).toBeGreaterThan(0);
    expect((await screen.findAllByText(/translated blocks on-brand/i)).length).toBeGreaterThan(0);
  });

  it("renders the delivery slot next to ship readiness", () => {
    render(
      <TranslationDashboard
        stats={shipStateDashboardStats}
        delivery={<div data-testid="delivery-slot" />}
      />,
    );
    expect(screen.getByTestId("delivery-slot")).toBeInTheDocument();
    expect(screen.getByTestId("ship-readiness")).toBeInTheDocument();
  });

  it("sorts file table by name descending when clicking File header", async () => {
    const user = userEvent.setup();
    render(<TranslationDashboard stats={sampleDashboardStats} />);

    // Find the "File Progress" card and the "File" column header.
    const fileProgressCard = screen.getByText("File Progress").closest("[data-slot=card]")!;
    const headers = within(fileProgressCard).getAllByRole("columnheader");
    const fileHeader = headers[0]; // First column is "File"

    // Default is asc by name, clicking toggles to desc.
    await user.click(fileHeader);

    // In desc order: ui-strings.xliff > messages.json > landing-page.html
    const rows = within(fileProgressCard).getAllByRole("row");
    expect(within(rows[1]).getByText("ui-strings")).toBeInTheDocument();
  });
});
