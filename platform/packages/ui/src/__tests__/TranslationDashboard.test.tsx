import { describe, it, expect } from "vite-plus/test";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { TranslationDashboard } from "../components/TranslationDashboard";
import { sampleDashboardStats } from "../stories/fixtures";
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
    expect(screen.getByText("messages.json")).toBeInTheDocument();
    expect(screen.getByText("ui-strings.xliff")).toBeInTheDocument();
    expect(screen.getByText("landing-page.html")).toBeInTheDocument();
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
    expect(within(rows[1]).getByText("ui-strings.xliff")).toBeInTheDocument();
  });
});
