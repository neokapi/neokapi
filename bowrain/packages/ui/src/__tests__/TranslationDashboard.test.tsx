import { describe, it, expect, vi } from "vite-plus/test";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { TranslationDashboard } from "../components/TranslationDashboard";
import {
  sampleDashboardStats,
  shipStateDashboardStats,
  complianceDashboardStats,
} from "../stories/fixtures";
import type { TranslationDashboardStats } from "../types/api";

const noop = () => {};

describe("TranslationDashboard", () => {
  it("shows empty state when stats is null", () => {
    render(<TranslationDashboard stats={null} projectName="Test" />);
    expect(screen.getByText(/no content yet/i)).toBeInTheDocument();
  });

  it("renders the project name in the header", () => {
    render(<TranslationDashboard stats={sampleDashboardStats} projectName="Demo App" />);
    expect(screen.getByText(/Demo App · Overview/)).toBeInTheDocument();
  });

  it("renders generic header when no project name given", () => {
    render(<TranslationDashboard stats={sampleDashboardStats} />);
    expect(screen.getByText("Overview")).toBeInTheDocument();
  });

  it("renders all four summary stat cards", () => {
    render(<TranslationDashboard stats={sampleDashboardStats} />);
    expect(screen.getByText("Source Words")).toBeInTheDocument();
    expect(screen.getByText("Translatable Blocks")).toBeInTheDocument();
    expect(screen.getByText("Target Languages")).toBeInTheDocument();
    expect(screen.getByText("Overall Completion")).toBeInTheDocument();
  });

  it("shows overall completion percentage in header", () => {
    render(<TranslationDashboard stats={sampleDashboardStats} />);
    expect(screen.getByText(/% complete$/)).toBeInTheDocument();
  });

  it("leads with the collections, not the files", () => {
    render(<TranslationDashboard stats={sampleDashboardStats} onOpenCollection={noop} />);
    expect(screen.getByTestId("collection-overview")).toBeInTheDocument();
    expect(screen.getByText("Product app")).toBeInTheDocument();
    expect(screen.getByText("Website")).toBeInTheDocument();
    // The per-file table is the second level now; the overview holds no rows.
    expect(screen.queryByText("messages")).toBeNull();
    expect(screen.queryByText("ui-strings")).toBeNull();
  });

  it("groups collections by the coordinate axis that partitions them best", () => {
    render(<TranslationDashboard stats={sampleDashboardStats} onOpenCollection={noop} />);
    // Both collections share product=acme and differ on channel, so product is
    // the axis that groups; the heading names the axis and its value.
    const overview = screen.getByTestId("collection-overview");
    expect(within(overview).getByText("product")).toBeInTheDocument();
    expect(within(overview).getByText("acme")).toBeInTheDocument();
  });

  it("opens a collection by name and by its Items action", async () => {
    const user = userEvent.setup();
    const onOpenCollection = vi.fn();
    render(
      <TranslationDashboard stats={sampleDashboardStats} onOpenCollection={onOpenCollection} />,
    );
    await user.click(screen.getByText("Website"));
    expect(onOpenCollection).toHaveBeenCalledWith({
      collectionId: "coll-web",
      ungrouped: false,
    });

    const card = screen.getByTestId("collection-card-coll-web");
    await user.click(within(card).getByRole("button", { name: "Items" }));
    expect(onOpenCollection).toHaveBeenCalledTimes(2);
  });

  it("carries the collection into review", async () => {
    const user = userEvent.setup();
    const onReviewCollection = vi.fn();
    render(
      <TranslationDashboard
        stats={sampleDashboardStats}
        onOpenCollection={noop}
        onReviewCollection={onReviewCollection}
      />,
    );
    const card = screen.getByTestId("collection-card-coll-web");
    await user.click(within(card).getByRole("button", { name: "Review" }));
    expect(onReviewCollection).toHaveBeenCalledWith({
      collectionId: "coll-web",
      ungrouped: false,
    });
  });

  it("keeps the whole item list reachable behind an all-items entry", async () => {
    const user = userEvent.setup();
    const onOpenAllItems = vi.fn();
    render(
      <TranslationDashboard
        stats={sampleDashboardStats}
        onOpenCollection={noop}
        onOpenAllItems={onOpenAllItems}
      />,
    );
    await user.click(screen.getByTestId("open-all-items"));
    expect(onOpenAllItems).toHaveBeenCalledOnce();
  });

  it("names the collection-less bucket rather than calling it a collection", () => {
    const withUngrouped: TranslationDashboardStats = {
      ...sampleDashboardStats,
      collection_stats: [
        ...sampleDashboardStats.collection_stats,
        {
          collection_id: "",
          collection_name: "",
          ungrouped: true,
          item_count: 3,
          block_count: 5,
          word_count: 60,
          locales: [],
        },
      ],
    };
    render(<TranslationDashboard stats={withUngrouped} onOpenCollection={noop} />);
    expect(screen.getByTestId("collection-card-ungrouped")).toBeInTheDocument();
    expect(screen.getAllByText("In no collection").length).toBeGreaterThan(0);
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

  it("hides the collection overview when the project holds no collections", () => {
    const noColl: TranslationDashboardStats = {
      ...sampleDashboardStats,
      collection_stats: [],
    };
    render(<TranslationDashboard stats={noColl} onOpenCollection={noop} />);
    expect(screen.queryByTestId("collection-overview")).toBeNull();
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

  it("carries each locale's ship state onto its collection rail", () => {
    render(<TranslationDashboard stats={shipStateDashboardStats} onOpenCollection={noop} />);
    const overview = screen.getByTestId("collection-overview");
    expect(within(overview).getAllByTestId("ship-state-pending").length).toBeGreaterThan(0);
  });

  it("renders a compliance rate chip per locale when the server derives it", () => {
    render(<TranslationDashboard stats={complianceDashboardStats} />);
    const card = screen.getByTestId("ship-readiness");
    const chips = within(card).getAllByTestId("compliant-rate");
    // Every locale in the fixture has translated blocks, so every row gets a chip.
    expect(chips.length).toBe(complianceDashboardStats.locale_stats.length);
    expect(within(card).getByText("92% compliant")).toBeInTheDocument();
    // The basis rides on the chip so tests (and analytics) can tell voice-informed
    // rates from checks-only ones.
    expect(chips.some((c) => c.dataset.basis === "voice+checks")).toBe(true);
    expect(chips.some((c) => c.dataset.basis === "checks")).toBe(true);
  });

  it("hides the compliance rate chip when servers do not send the field", () => {
    render(<TranslationDashboard stats={shipStateDashboardStats} />);
    expect(screen.queryByTestId("compliant-rate")).toBeNull();
  });

  it("explains the compliance basis in the chip tooltip", async () => {
    const user = userEvent.setup();
    render(<TranslationDashboard stats={complianceDashboardStats} />);
    const card = screen.getByTestId("ship-readiness");
    const voiceChip = within(card)
      .getAllByTestId("compliant-rate")
      .find((c) => c.dataset.basis === "voice+checks")!;
    await user.hover(voiceChip);
    // SimpleTooltip renders duplicate (trigger + portal) content.
    expect((await screen.findAllByText(/voice scores measured against/i)).length).toBeGreaterThan(
      0,
    );
    expect((await screen.findAllByText(/translated blocks compliant/i)).length).toBeGreaterThan(0);
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
});
