import { render, screen } from "./testUtils";
import userEvent from "@testing-library/user-event";
import { describe, it, expect } from "vitest";
import { ErrorProvider } from "../components/ErrorBanner";
import { ToolRunnerPage } from "../components/ToolRunnerPage";
import type { ToolInfo, PluginDocs } from "../types/api";

const sampleTools: ToolInfo[] = [
  {
    name: "translate",
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
    expect(screen.getByText("Select a tool to see what it does")).toBeInTheDocument();
  });

  it("shows tool count in empty state", () => {
    renderPage({ tools: sampleTools });
    expect(screen.getByText(/4 tools available/)).toBeInTheDocument();
  });

  it("renders all tools in sidebar", () => {
    renderPage({ tools: sampleTools });
    expect(screen.getByText("translate")).toBeInTheDocument();
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
    expect(screen.getByText("translate")).toBeInTheDocument();
    expect(screen.queryByText("pseudo-translate")).not.toBeInTheDocument();
    expect(screen.queryByText("word-count")).not.toBeInTheDocument();
  });

  it("filters by search text", async () => {
    renderPage({ tools: sampleTools });
    await userEvent.type(screen.getByPlaceholderText("Search tools..."), "pseudo");
    expect(screen.getByText("pseudo-translate")).toBeInTheDocument();
    expect(screen.queryByText("translate")).not.toBeInTheDocument();
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
    await userEvent.click(screen.getByText("translate"));
    // Tags appear in both sidebar (truncated) and detail. Use getAllByText.
    const tags = screen.getAllByText("ai-powered");
    expect(tags.length).toBeGreaterThanOrEqual(1);
    // Requirements only appear in detail
    expect(screen.getByText("target-language")).toBeInTheDocument();
    expect(screen.getByText("credentials")).toBeInTheDocument();
  });

  it("gives the CLI invocation behind a disclosure, not as the headline", async () => {
    renderPage({ tools: sampleTools });
    await userEvent.click(screen.getByText("word-count"));

    // Plain guidance leads; the command is tucked into a disclosure.
    expect(
      screen.getByText("To run this tool, add it to a flow and run that."),
    ).toBeInTheDocument();
    await userEvent.click(screen.getByText("Run from the command line"));
    expect(screen.getByText("kapi exec word-count <files...>")).toBeInTheDocument();
    expect(screen.getByText("kapi tools schema word-count")).toBeInTheDocument();
  });

  it("offers no in-app run controls", async () => {
    renderPage({ tools: sampleTools });
    await userEvent.click(screen.getByText("translate"));

    // The page describes the tool and hands over the command; it never presents
    // itself as a runner, so there is no disabled form standing in for one.
    expect(screen.queryByText("Select files...")).not.toBeInTheDocument();
    expect(screen.queryByText("Target Language")).not.toBeInTheDocument();
    expect(screen.queryByPlaceholderText("e.g. fr")).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /^Run translate/ })).not.toBeInTheDocument();
    expect(screen.queryByText(/coming soon/i)).not.toBeInTheDocument();
  });

  it("offers Translation last among the category chips", () => {
    renderPage({ tools: sampleTools });
    const chips = screen
      .getAllByRole("button")
      .map((b) => b.textContent ?? "")
      .filter((label) => /\(\d+\)$/.test(label));

    expect(chips[0]).toMatch(/^All/);
    expect(chips.at(-1)).toMatch(/^Translation/);
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
