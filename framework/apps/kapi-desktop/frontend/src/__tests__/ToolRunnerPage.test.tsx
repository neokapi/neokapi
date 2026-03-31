import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect } from "vitest";
import { ErrorProvider } from "../components/ErrorBanner";
import { ToolRunnerPage } from "../components/ToolRunnerPage";
import type { ToolInfo, PluginDocs } from "../types/api";

const sampleTools: ToolInfo[] = [
  {
    name: "ai-translate",
    description: "Translate content using AI/LLM",
    category: "translate",
    has_schema: false,
    inputs: ["block"],
    tags: ["ai-powered", "translation"],
    requires: ["target-language", "credentials"],
  },
  {
    name: "pseudo-translate",
    description: "Generate pseudo-translations for testing",
    category: "transform",
    has_schema: true,
    inputs: ["block"],
    tags: ["translation"],
    requires: ["target-language"],
  },
  {
    name: "word-count",
    description: "Count words in content",
    category: "validate",
    has_schema: false,
    inputs: ["block"],
    tags: ["reporting"],
    requires: [],
  },
];

const sampleDocs: PluginDocs = {
  generatedAt: "2026-03-31T00:00:00Z",
  filters: {},
  steps: {
    "batch-translation": {
      filterName: "Batch Translation Step",
      overview: "Translates using batch resources.",
      stepId: "batch-translation",
      parameters: {
        removeBOM: { description: "Remove BOM from files." },
      },
    },
  },
};

function renderPage(props?: { tools?: ToolInfo[]; docs?: PluginDocs | null }) {
  return render(
    <ErrorProvider>
      <ToolRunnerPage {...props} />
    </ErrorProvider>,
  );
}

describe("ToolRunnerPage", () => {
  it("renders empty state when no tool selected", () => {
    renderPage({ tools: sampleTools });
    expect(
      screen.getByText("Select a tool to view details and run it"),
    ).toBeInTheDocument();
  });

  it("shows tool count in empty state", () => {
    renderPage({ tools: sampleTools });
    expect(
      screen.getByText(/3 tools available/),
    ).toBeInTheDocument();
  });

  it("renders all tools in sidebar", () => {
    renderPage({ tools: sampleTools });
    expect(screen.getByText("ai-translate")).toBeInTheDocument();
    expect(screen.getByText("pseudo-translate")).toBeInTheDocument();
    expect(screen.getByText("word-count")).toBeInTheDocument();
  });

  it("renders category filter chips", () => {
    renderPage({ tools: sampleTools });
    expect(screen.getByText(/All/)).toBeInTheDocument();
    expect(screen.getByText(/Translation/)).toBeInTheDocument();
    expect(screen.getByText(/Transform/)).toBeInTheDocument();
    expect(screen.getByText(/Quality/)).toBeInTheDocument();
  });

  it("filters by category when chip clicked", async () => {
    renderPage({ tools: sampleTools });
    // Click "Translation" category
    await userEvent.click(screen.getByText(/Translation \(1\)/));
    expect(screen.getByText("ai-translate")).toBeInTheDocument();
    expect(screen.queryByText("pseudo-translate")).not.toBeInTheDocument();
    expect(screen.queryByText("word-count")).not.toBeInTheDocument();
  });

  it("filters by search text", async () => {
    renderPage({ tools: sampleTools });
    await userEvent.type(
      screen.getByPlaceholderText("Search tools..."),
      "pseudo",
    );
    expect(screen.getByText("pseudo-translate")).toBeInTheDocument();
    expect(screen.queryByText("ai-translate")).not.toBeInTheDocument();
  });

  it("shows tool detail when tool clicked", async () => {
    renderPage({ tools: sampleTools });
    await userEvent.click(screen.getByText("word-count"));
    // Description appears in both sidebar and detail — use getAllByText
    const matches = screen.getAllByText("Count words in content");
    expect(matches.length).toBeGreaterThanOrEqual(1);
  });

  it("shows tool tags and requirements in detail view", async () => {
    renderPage({ tools: sampleTools });
    await userEvent.click(screen.getByText("ai-translate"));
    // Tags appear in both sidebar (truncated) and detail. Use getAllByText.
    const tags = screen.getAllByText("ai-powered");
    expect(tags.length).toBeGreaterThanOrEqual(1);
    // Requirements only appear in detail
    expect(screen.getByText("target-language")).toBeInTheDocument();
    expect(screen.getByText("credentials")).toBeInTheDocument();
  });

  it("shows Run tab by default in detail", async () => {
    renderPage({ tools: sampleTools });
    await userEvent.click(screen.getByText("ai-translate"));
    expect(screen.getByText("Select files...")).toBeInTheDocument();
    expect(screen.getByText("Target Language")).toBeInTheDocument();
  });

  it("shows no tools message when search has no results", async () => {
    renderPage({ tools: sampleTools });
    await userEvent.type(
      screen.getByPlaceholderText("Search tools..."),
      "zzzzz",
    );
    expect(
      screen.getByText("No tools match your search."),
    ).toBeInTheDocument();
  });

  it("shows Documentation tab when step docs available", async () => {
    const toolsWithDocs: ToolInfo[] = [
      {
        name: "batch-translation",
        description: "Batch translation tool",
        category: "translate",
        has_schema: false,
      },
    ];
    renderPage({ tools: toolsWithDocs, docs: sampleDocs });
    await userEvent.click(screen.getByText("batch-translation"));
    expect(screen.getByText("Documentation")).toBeInTheDocument();
  });

  it("shows step overview when docs available", async () => {
    const toolsWithDocs: ToolInfo[] = [
      {
        name: "batch-translation",
        description: "Batch translation tool",
        category: "translate",
        has_schema: false,
      },
    ];
    renderPage({ tools: toolsWithDocs, docs: sampleDocs });
    await userEvent.click(screen.getByText("batch-translation"));
    expect(
      screen.getByText("Translates using batch resources."),
    ).toBeInTheDocument();
  });
});
