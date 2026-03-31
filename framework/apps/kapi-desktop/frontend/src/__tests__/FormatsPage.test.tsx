import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect } from "vitest";
import { ErrorProvider } from "../components/ErrorBanner";
import { FormatsPage } from "../components/FormatsPage";
import type { PluginDocs } from "../types/api";

const sampleDocs: PluginDocs = {
  generatedAt: "2026-03-31T00:00:00Z",
  wikiBaseUrl: "https://example.com/wiki/",
  filters: {
    okf_json: {
      filterName: "JSON Filter",
      overview: "Extracts translatable strings from JSON files.",
      filterId: "okf_json",
      wikiUrl: "https://example.com/json",
      parameters: {
        extraction: { description: "Controls extraction behavior." },
      },
    },
    okf_html: {
      filterName: "HTML Filter",
      overview: "Processes HTML documents for translation.",
      filterId: "okf_html",
    },
  },
  steps: {},
  aliases: { okf_baseplaintext: "okf_plaintext" },
};

function renderPage(docs?: PluginDocs | null) {
  return render(
    <ErrorProvider>
      <FormatsPage docs={docs} />
    </ErrorProvider>,
  );
}

describe("FormatsPage", () => {
  it("renders page title", () => {
    renderPage();
    expect(screen.getByText("Formats")).toBeInTheDocument();
  });

  it("renders search input", () => {
    renderPage();
    expect(
      screen.getByPlaceholderText("Search formats by name or extension..."),
    ).toBeInTheDocument();
  });

  it("shows documented formats count when docs provided", () => {
    renderPage(sampleDocs);
    expect(screen.getByText("2 documented formats")).toBeInTheDocument();
  });

  it("shows empty state after loading with no API", async () => {
    renderPage();
    await waitFor(() => {
      expect(
        screen.getByText("No formats available."),
      ).toBeInTheDocument();
    });
  });

  it("shows search empty state", async () => {
    renderPage();
    await waitFor(() => {
      expect(
        screen.queryByText("Loading configuration schema..."),
      ).not.toBeInTheDocument();
    });
    await userEvent.type(
      screen.getByPlaceholderText("Search formats by name or extension..."),
      "nonexistent",
    );
    expect(
      screen.getByText("No formats match your search."),
    ).toBeInTheDocument();
  });
});
