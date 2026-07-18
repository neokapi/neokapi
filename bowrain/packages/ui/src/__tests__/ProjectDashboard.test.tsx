import { describe, it, expect } from "vite-plus/test";
import { render, screen, within } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ReactNode } from "react";
import { ProjectDashboard } from "../components/ProjectDashboard";
import { ApiProvider } from "../context/ApiContext";
import type { ApiAdapter } from "../api/adapter";
import type { ProjectInfo } from "../types/api";

// ProjectDashboard reaches the adapter only through useLocales.
const stubAdapter = {
  getKnownLocales: async () => [],
} as unknown as ApiAdapter;

function renderDashboard(projects: ProjectInfo[]) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const wrapper = ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={qc}>
      <ApiProvider adapter={stubAdapter}>{children}</ApiProvider>
    </QueryClientProvider>
  );
  return render(
    <ProjectDashboard
      projects={projects}
      onCreateProject={() => {}}
      onOpenProject={() => {}}
      workspaceName="Acme"
    />,
    { wrapper },
  );
}

function makeProject(overrides: Partial<ProjectInfo> = {}): ProjectInfo {
  return {
    id: "p1",
    name: "Website",
    default_source_language: "en",
    target_languages: ["fr", "de"],
    created_at: "2026-01-01T00:00:00Z",
    modified_at: "2026-01-02T00:00:00Z",
    ...overrides,
  };
}

describe("ProjectDashboard (summary aggregates)", () => {
  it("renders card and stats totals from server aggregates without items[]", () => {
    // The summary list shape: aggregates only, items empty.
    renderDashboard([
      makeProject({
        id: "p1",
        name: "Website",
        items: [],
        item_count: 12,
        block_count: 300,
        word_count: 4500,
        stream_count: 2,
      }),
      makeProject({
        id: "p2",
        name: "Docs",
        target_languages: ["fr", "ja"],
        items: [],
        item_count: 3,
        word_count: 500,
        stream_count: 1,
      }),
    ]);

    const card = screen.getByTestId("project-card-p1");
    expect(within(card).getByText("12 files")).toBeInTheDocument();
    expect(within(card).getByText("4.5k words")).toBeInTheDocument();
    expect(within(card).getByText("2 streams")).toBeInTheDocument();

    // Stats bar: 15 files, 5.0k words, 3 unique locales across both projects.
    expect(screen.getByText("Files")).toBeInTheDocument();
    expect(screen.getByText("15")).toBeInTheDocument();
    expect(screen.getByText("5.0k")).toBeInTheDocument();
    expect(screen.getByText("3")).toBeInTheDocument();
  });

  it("falls back to embedded items for adapters without aggregates", () => {
    // Legacy/full shape: no aggregate fields, embedded items.
    renderDashboard([
      makeProject({
        id: "p1",
        items: [
          {
            id: "i1",
            name: "a.json",
            format: "json",
            type: "file",
            size: 0,
            block_count: 5,
            word_count: 120,
          },
          {
            id: "i2",
            name: "b.json",
            format: "json",
            type: "file",
            size: 0,
            block_count: 2,
            word_count: 80,
          },
        ],
      }),
    ]);

    const card = screen.getByTestId("project-card-p1");
    expect(within(card).getByText("2 files")).toBeInTheDocument();
    expect(within(card).getByText("200 words")).toBeInTheDocument();
  });
});
