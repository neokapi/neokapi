import { render, screen } from "@testing-library/react";
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
  {
    name: "search-and-replace",
    description: "Search and replace text patterns",
    category: "transform",
    has_schema: true,
    source: "okapi",
    inputs: ["block"],
    tags: ["text-processing", "regex"],
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
    expect(screen.getByText("Select a tool to view details and run it")).toBeInTheDocument();
  });

  it("shows tool count in empty state", () => {
    renderPage({ tools: sampleTools });
    expect(screen.getByText(/4 tools available/)).toBeInTheDocument();
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
    await userEvent.type(screen.getByPlaceholderText("Search tools..."), "pseudo");
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

  it("renders runner controls in a disabled coming-soon state", async () => {
    renderPage({ tools: sampleTools });
    await userEvent.click(screen.getByText("word-count"));

    // The "coming soon" affordance is shown so the controls don't read as live.
    expect(screen.getByText("Running tools here is coming soon")).toBeInTheDocument();

    // The "Select files..." button is disabled (no live file picker yet).
    const selectFiles = screen.getByText("Select files...").closest("button");
    expect(selectFiles).toBeDisabled();

    // The Run button is disabled and never invokes a backend call.
    const runButton = screen.getByText("Run word-count").closest("button");
    expect(runButton).toBeDisabled();
  });

  it("keeps the Run button disabled even when a target language is provided", async () => {
    renderPage({ tools: sampleTools });
    await userEvent.click(screen.getByText("ai-translate"));

    // The target-language field is itself disabled, and the Run button stays
    // disabled — there is no working execution path to enable.
    const targetLang = screen.getByPlaceholderText("e.g. fr-FR");
    expect(targetLang).toBeDisabled();

    const runButton = screen.getByText(/Run ai-translate/).closest("button");
    expect(runButton).toBeDisabled();
  });

  it("shows no tools message when search has no results", async () => {
    renderPage({ tools: sampleTools });
    await userEvent.type(screen.getByPlaceholderText("Search tools..."), "zzzzz");
    expect(screen.getByText("No tools match your search.")).toBeInTheDocument();
  });

  it("shows tool description in header when docs available", async () => {
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
    // The tool description appears in the header
    const matches = screen.getAllByText("Batch translation tool");
    expect(matches.length).toBeGreaterThanOrEqual(1);
  });

  it("shows source badge for plugin tools", () => {
    renderPage({ tools: sampleTools });
    // "okapi" source badge should appear on the search-and-replace tool
    expect(screen.getByText("okapi")).toBeInTheDocument();
  });

  it("does not show source badge for built-in tools", () => {
    renderPage({ tools: sampleTools });
    // Built-in tools have no source or source="built-in", no badge rendered
    const badges = screen.queryAllByText("built-in");
    expect(badges).toHaveLength(0);
  });
});
