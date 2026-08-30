import { render, screen, waitFor } from "./testUtils";
import { describe, it, expect, vi } from "vitest";

// The store pages read every value through the Wails-bridge API; mock it so the
// open-store dashboards render without a Wails runtime.
vi.mock("../hooks/useApi", () => ({
  api: {
    getProjectHandles: () => Promise.resolve({ memoryHandle: "mem-1", termsHandle: "tb-1" }),
    getProject: () => Promise.resolve({ defaults: { source_language: "en" } }),
    getMemoryStats: () => Promise.resolve({ count: 12 }),
    getMemoryActivityStats: () => Promise.resolve([]),
    getMemoryFacets: () => Promise.resolve(null),
    getMemoryFacetsFiltered: () => Promise.resolve(null),
    searchMemoryEntries: () => Promise.resolve({ entries: [], total_count: 0 }),
    searchMemoryEntriesFiltered: () => Promise.resolve({ entries: [], total_count: 0 }),
    listMemoryImportSessions: () => Promise.resolve([]),
    listNamedMemories: () => Promise.resolve([]),
    getTermsStats: () => Promise.resolve({ count: 5 }),
    getTermsActivityStats: () => Promise.resolve([]),
    searchTerms: () => Promise.resolve({ concepts: [], total_count: 0 }),
    listNamedTerms: () => Promise.resolve([]),
    getKnownLocales: () => Promise.resolve([]),
    listFilters: () => Promise.resolve([]),
  },
}));

import { MemoriesPage } from "../components/MemoriesPage";
import { TermsPage } from "../components/TermsPage";
import { ErrorProvider } from "../components/ErrorBanner";

/** Labels the app used to offer for moving store contents in and out as files. */
const INTERCHANGE_LABELS = [
  /import tmx/i,
  /export tmx/i,
  /import csv/i,
  /import json/i,
  /export json/i,
];

function expectNoInterchangeControls() {
  for (const label of INTERCHANGE_LABELS) {
    expect(screen.queryByRole("button", { name: label })).not.toBeInTheDocument();
    expect(screen.queryByText(label)).not.toBeInTheDocument();
  }
}

describe("content memory page", () => {
  it("offers no file interchange on the open store", async () => {
    render(
      <ErrorProvider>
        <MemoriesPage tabID="t1" />
      </ErrorProvider>,
    );
    await waitFor(() => expect(screen.getByText("Project Content Memory")).toBeInTheDocument());
    expectNoInterchangeControls();
  });

  it("keeps the store pickers", async () => {
    render(
      <ErrorProvider>
        <MemoriesPage resources={[]} />
      </ErrorProvider>,
    );
    await waitFor(() => expect(screen.getByText("Content Memories")).toBeInTheDocument());
    expect(screen.getAllByRole("button", { name: /open file/i }).length).toBeGreaterThan(0);
    expect(screen.getAllByRole("button", { name: /new memory/i }).length).toBeGreaterThan(0);
    expectNoInterchangeControls();
  });
});

describe("terms page", () => {
  it("offers no file interchange on the open store", async () => {
    render(
      <ErrorProvider>
        <TermsPage tabID="t1" />
      </ErrorProvider>,
    );
    await waitFor(() => expect(screen.getByText("Project Terms")).toBeInTheDocument());
    expectNoInterchangeControls();
  });

  it("keeps the store pickers", async () => {
    render(
      <ErrorProvider>
        <TermsPage resources={[]} />
      </ErrorProvider>,
    );
    await waitFor(() => expect(screen.getByText("Terms")).toBeInTheDocument());
    expect(screen.getAllByRole("button", { name: /open file/i }).length).toBeGreaterThan(0);
    expect(screen.getAllByRole("button", { name: /new terms/i }).length).toBeGreaterThan(0);
    expectNoInterchangeControls();
  });
});
