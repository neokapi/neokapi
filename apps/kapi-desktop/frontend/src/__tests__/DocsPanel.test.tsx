import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect } from "vitest";
import { DocsPanel, ParamHelp } from "../components/DocsPanel";
import type { FilterDoc, StepDoc } from "../types/api";

const sampleFilterDoc: FilterDoc = {
  filterName: "JSON Filter",
  overview: "Extracts translatable strings from JSON files.",
  filterId: "okf_json",
  wikiUrl: "https://example.com/json-filter",
  parameters: {
    extraction: {
      description: "Controls what content is extracted.",
    },
    "extraction.extractAll": {
      description: "When true, extracts all strings.",
      notes: ["Overridden by pathRules if specified."],
      dependsOn: [{ property: "extraction.pathRules", condition: "must not be set" }],
    },
    "keys.useFullPath": {
      description: "Use full hierarchical key path.",
      introducedIn: "M39",
    },
  },
  limitations: ["Cannot handle binary JSON (BSON)."],
  processingNotes: ["Supports JSON5 comments."],
  examples: [
    {
      title: "Basic extraction",
      description: "Extract all key-value pairs.",
      input: '{"hello": "world"}',
      output: "1 text unit extracted",
    },
  ],
};

const sampleStepDoc: StepDoc = {
  filterName: "Batch Translation Step",
  overview: "Translates content using a batch process.",
  stepId: "batch-translation",
  parameters: {
    removeBOM: {
      description: "When true, removes BOM from files.",
    },
  },
};

describe("DocsPanel", () => {
  it("renders overview section expanded by default", () => {
    render(<DocsPanel doc={sampleFilterDoc} />);
    expect(screen.getByText("Overview")).toBeInTheDocument();
    expect(screen.getByText("Extracts translatable strings from JSON files.")).toBeInTheDocument();
  });

  it("renders wiki link when available", () => {
    render(<DocsPanel doc={sampleFilterDoc} />);
    expect(screen.getByText("View full documentation")).toBeInTheDocument();
  });

  it("renders section headers for parameters, examples, limitations, notes", () => {
    render(<DocsPanel doc={sampleFilterDoc} />);
    expect(screen.getByText("Parameters")).toBeInTheDocument();
    expect(screen.getByText("Examples")).toBeInTheDocument();
    expect(screen.getByText("Limitations")).toBeInTheDocument();
    expect(screen.getByText("Processing Notes")).toBeInTheDocument();
  });

  it("shows parameter count badge", () => {
    render(<DocsPanel doc={sampleFilterDoc} />);
    // 3 parameters
    expect(screen.getByText("3")).toBeInTheDocument();
  });

  it("expands parameters section on click", async () => {
    render(<DocsPanel doc={sampleFilterDoc} />);
    await userEvent.click(screen.getByText("Parameters"));
    expect(screen.getByText("Controls what content is extracted.")).toBeInTheDocument();
  });

  it("shows introducedIn badge for versioned parameters", async () => {
    render(<DocsPanel doc={sampleFilterDoc} />);
    await userEvent.click(screen.getByText("Parameters"));
    expect(screen.getByText("M39")).toBeInTheDocument();
  });

  it("shows dependency badges after expanding parent parameter", async () => {
    render(<DocsPanel doc={sampleFilterDoc} />);
    await userEvent.click(screen.getByText("Parameters"));
    // extraction has children — click to expand it
    await userEvent.click(screen.getByText("extraction"));
    expect(screen.getByText("extraction.pathRules")).toBeInTheDocument();
    expect(screen.getByText("must not be set")).toBeInTheDocument();
  });

  it("shows parameter notes after expanding parent parameter", async () => {
    render(<DocsPanel doc={sampleFilterDoc} />);
    await userEvent.click(screen.getByText("Parameters"));
    // extraction has children — click to expand it
    await userEvent.click(screen.getByText("extraction"));
    expect(screen.getByText("Overridden by pathRules if specified.")).toBeInTheDocument();
  });

  it("expands examples section on click", async () => {
    render(<DocsPanel doc={sampleFilterDoc} />);
    await userEvent.click(screen.getByText("Examples"));
    expect(screen.getByText("Basic extraction")).toBeInTheDocument();
    expect(screen.getByText('{"hello": "world"}')).toBeInTheDocument();
    expect(screen.getByText("1 text unit extracted")).toBeInTheDocument();
  });

  it("expands limitations section on click", async () => {
    render(<DocsPanel doc={sampleFilterDoc} />);
    await userEvent.click(screen.getByText("Limitations"));
    expect(screen.getByText("Cannot handle binary JSON (BSON).")).toBeInTheDocument();
  });

  it("renders in inline mode without card wrapper", () => {
    const { container } = render(<DocsPanel doc={sampleFilterDoc} inline />);
    // In inline mode the outermost element should be a plain div, not a card
    const outerDiv = container.firstElementChild;
    expect(outerDiv?.tagName).toBe("DIV");
    expect(outerDiv?.className).not.toContain("rounded-lg border");
  });

  it("filters parameters by visibleParams", async () => {
    render(<DocsPanel doc={sampleFilterDoc} visibleParams={["extraction"]} />);
    await userEvent.click(screen.getByText("Parameters"));
    // Should show extraction and (after expanding) extraction.extractAll
    expect(screen.getByText("Controls what content is extracted.")).toBeInTheDocument();
    // keys.useFullPath should NOT be present (filtered out)
    expect(screen.queryByText("Use full hierarchical key path.")).not.toBeInTheDocument();
    // Expand extraction to see its child
    await userEvent.click(screen.getByText("extraction"));
    expect(screen.getByText("When true, extracts all strings.")).toBeInTheDocument();
  });

  it("renders step doc (not just filter doc)", () => {
    render(<DocsPanel doc={sampleStepDoc} />);
    expect(screen.getByText("Overview")).toBeInTheDocument();
    expect(screen.getByText("Translates content using a batch process.")).toBeInTheDocument();
  });

  it("hides sections with no data", () => {
    const minimalDoc: FilterDoc = {
      filterName: "Minimal",
      overview: "A minimal filter.",
    };
    render(<DocsPanel doc={minimalDoc} />);
    expect(screen.queryByText("Parameters")).not.toBeInTheDocument();
    expect(screen.queryByText("Examples")).not.toBeInTheDocument();
    expect(screen.queryByText("Limitations")).not.toBeInTheDocument();
    expect(screen.queryByText("Processing Notes")).not.toBeInTheDocument();
  });
});

describe("ParamHelp", () => {
  it("renders nothing for nonexistent parameter", () => {
    const { container } = render(<ParamHelp paramKey="nonexistent" doc={sampleFilterDoc} />);
    expect(container.firstElementChild).toBeNull();
  });

  it("renders nothing when doc is undefined", () => {
    const { container } = render(<ParamHelp paramKey="extraction" doc={undefined} />);
    expect(container.firstElementChild).toBeNull();
  });

  it("renders info button for existing parameter", () => {
    render(<ParamHelp paramKey="extraction" doc={sampleFilterDoc} />);
    expect(screen.getByTitle("Show parameter documentation")).toBeInTheDocument();
  });

  it("shows tooltip on click", async () => {
    render(<ParamHelp paramKey="extraction" doc={sampleFilterDoc} />);
    await userEvent.click(screen.getByTitle("Show parameter documentation"));
    expect(screen.getByText("Controls what content is extracted.")).toBeInTheDocument();
  });

  it("hides tooltip on second click", async () => {
    render(<ParamHelp paramKey="extraction" doc={sampleFilterDoc} />);
    const btn = screen.getByTitle("Show parameter documentation");
    await userEvent.click(btn);
    expect(screen.getByText("Controls what content is extracted.")).toBeInTheDocument();
    await userEvent.click(btn);
    expect(screen.queryByText("Controls what content is extracted.")).not.toBeInTheDocument();
  });
});
