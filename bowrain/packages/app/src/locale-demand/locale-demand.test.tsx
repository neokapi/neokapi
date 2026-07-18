import { describe, it, expect, vi } from "vite-plus/test";
import { render, screen, fireEvent, within } from "@testing-library/react";
import { describeCoverage } from "./CoverageBadge";
import { LanguageDemandTable, sortLanguages } from "./LanguageDemandTable";
import { DemandDrillDownPanel } from "./DemandDrillDownPanel";
import { WorldDemandMap, demandFill } from "./WorldDemandMap";
import { sampleDemandDataSource, languageByCode, countryByCode } from "./locale-demand-fixtures";

const snapshot = sampleDemandDataSource.getSnapshot("30d");

describe("coverage badge logic", () => {
  it("labels covered languages", () => {
    expect(describeCoverage({ status: "covered" }).label).toBe("Covered");
  });

  it("labels partial coverage with its percent", () => {
    expect(describeCoverage({ status: "partial", percent: 62 }).label).toBe("Partial 62%");
  });

  it("labels uncovered languages", () => {
    expect(describeCoverage({ status: "not-covered" }).label).toBe("Not covered");
  });
});

describe("sample dataset", () => {
  it("marks the project locales as covered/partial and the rest as gaps", () => {
    expect(languageByCode(snapshot, "en")?.coverage.status).toBe("covered");
    expect(languageByCode(snapshot, "ja")?.coverage.status).toBe("partial");
    expect(languageByCode(snapshot, "pt-BR")?.coverage.status).toBe("not-covered");
    expect(languageByCode(snapshot, "pt-BR")?.planEstimate).toBeDefined();
    expect(languageByCode(snapshot, "en")?.planEstimate).toBeUndefined();
  });

  it("keeps country and language views coherent", () => {
    // Brazil's demand is dominated by pt-BR, so pt-BR's top country is Brazil.
    const ptBR = languageByCode(snapshot, "pt-BR")!;
    expect(ptBR.countries[0].code).toBe("BR");
  });
});

describe("sortLanguages", () => {
  it("sorts by demand share descending", () => {
    const sorted = sortLanguages(snapshot.languages, "share", "desc");
    for (let i = 1; i < sorted.length; i++) {
      expect(sorted[i - 1].share).toBeGreaterThanOrEqual(sorted[i].share);
    }
  });

  it("sorts by sessions ascending when toggled", () => {
    const sorted = sortLanguages(snapshot.languages, "sessions", "asc");
    for (let i = 1; i < sorted.length; i++) {
      expect(sorted[i - 1].sessions).toBeLessThanOrEqual(sorted[i].sessions);
    }
  });
});

describe("LanguageDemandTable", () => {
  it("renders one row per language, sorted by demand by default", () => {
    render(<LanguageDemandTable languages={snapshot.languages} />);
    const rows = screen
      .getAllByTestId(/language-row-/)
      .map((r) => r.getAttribute("data-testid")!.replace("language-row-", ""));
    expect(rows).toHaveLength(snapshot.languages.length);
    expect(rows[0]).toBe("en"); // English dominates the sample dataset
  });

  it("re-sorts when a sortable header is clicked", () => {
    render(<LanguageDemandTable languages={snapshot.languages} />);
    // First click switches to sessions desc, second click flips to asc.
    fireEvent.click(screen.getByRole("button", { name: /sessions/i }));
    fireEvent.click(screen.getByRole("button", { name: /sessions/i }));
    const rows = screen
      .getAllByTestId(/language-row-/)
      .map((r) => r.getAttribute("data-testid")!.replace("language-row-", ""));
    const leastDemanded = [...snapshot.languages].sort((a, b) => a.sessions - b.sessions)[0];
    expect(rows[0]).toBe(leastDemanded.code);
  });

  it("shows the gap estimate for uncovered languages only", () => {
    render(<LanguageDemandTable languages={snapshot.languages} />);
    const ptBR = within(screen.getByTestId("language-row-pt-BR"));
    expect(ptBR.getByText(/units ·/)).toBeInTheDocument();
    const en = within(screen.getByTestId("language-row-en"));
    expect(en.queryByText(/units ·/)).not.toBeInTheDocument();
  });

  it("reports row clicks", () => {
    const onSelect = vi.fn();
    render(<LanguageDemandTable languages={snapshot.languages} onSelectLanguage={onSelect} />);
    fireEvent.click(screen.getByTestId("language-row-ko"));
    expect(onSelect).toHaveBeenCalledWith("ko");
  });
});

describe("DemandDrillDownPanel", () => {
  it("opens a country drill-down with that country's data", () => {
    render(
      <DemandDrillDownPanel snapshot={snapshot} selection={{ kind: "country", code: "BR" }} />,
    );
    expect(screen.getByTestId("drilldown-title")).toHaveTextContent("Brazil");
    // Language breakdown within the country ("Brazilian Portuguese" per Intl.DisplayNames).
    expect(screen.getAllByText(/portuguese/i).length).toBeGreaterThan(0);
    // Payoff block prices the top uncovered language (pt-BR).
    const block = screen.getByTestId("add-locale-block");
    expect(countryByCode(snapshot, "BR")).toBeDefined();
    expect(within(block).getByText(/~4,120/)).toBeInTheDocument();
    expect(
      within(block).getByRole("button", { name: /add .*portuguese.* to the project/i }),
    ).toBeInTheDocument();
  });

  it("opens a language drill-down with country breakdown", () => {
    render(
      <DemandDrillDownPanel snapshot={snapshot} selection={{ kind: "language", code: "ko" }} />,
    );
    expect(screen.getByTestId("drilldown-title")).toHaveTextContent("Korean");
    expect(screen.getByText(/South Korea/)).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /add korean to the project/i })).toBeInTheDocument();
  });

  it("omits the payoff block for fully covered languages", () => {
    render(
      <DemandDrillDownPanel snapshot={snapshot} selection={{ kind: "language", code: "en" }} />,
    );
    expect(screen.queryByTestId("add-locale-block")).not.toBeInTheDocument();
  });

  it("notes the sample-data provenance", () => {
    render(
      <DemandDrillDownPanel snapshot={snapshot} selection={{ kind: "country", code: "JP" }} />,
    );
    expect(
      screen.getByText(/sample dataset \(web beacon \+ app telemetry ingest\)/i),
    ).toBeVisible();
  });
});

describe("demandFill", () => {
  it("keeps the lowest intensity well above the card background", () => {
    expect(demandFill(0)).toContain(" 25%");
  });

  it("yields distinguishable steps across the intensity range", () => {
    expect(demandFill(0.25)).toContain(" 55%");
    expect(demandFill(0.5)).toContain(" 67%");
    expect(demandFill(1)).toContain(" 85%");
  });
});

describe("WorldDemandMap", () => {
  it("outlines territories with the border token and the selection with the ring", () => {
    render(<WorldDemandMap countries={snapshot.countries} selectedCountry="BR" />);
    const map = screen.getByTestId("world-demand-map");
    const paths = [...map.querySelectorAll("path")];
    expect(paths.length).toBeGreaterThan(0);
    const selected = paths.filter((p) => p.getAttribute("data-country") === "BR");
    expect(selected).toHaveLength(1);
    expect(selected[0].getAttribute("stroke")).toBe("var(--ring)");
    for (const p of paths) {
      if (p === selected[0]) continue;
      expect(p.getAttribute("stroke")).toBe("var(--border)");
    }
  });
});
