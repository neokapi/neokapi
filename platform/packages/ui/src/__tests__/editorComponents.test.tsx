import { describe, it, expect, vi } from "vite-plus/test";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { SpanInfo, BlockTermMatch, EntityInfo, FileQAResult } from "../types/api";

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function span(
  spanType: "opening" | "closing" | "placeholder",
  type: string,
  data: string,
  id = "1",
): SpanInfo {
  return { span_type: spanType, type, id, data };
}

function termMatch(overrides: Partial<BlockTermMatch> = {}): BlockTermMatch {
  return {
    source_term: "hello",
    target_terms: ["hola"],
    domain: "",
    status: "preferred",
    start: 0,
    end: 5,
    ...overrides,
  };
}

function entity(overrides: Partial<EntityInfo> = {}): EntityInfo {
  return {
    key: "e1",
    text: "Acme",
    type: "entity:organization",
    start: 0,
    end: 4,
    dnt: false,
    ...overrides,
  };
}

// ---------------------------------------------------------------------------
// HighlightedSource
// ---------------------------------------------------------------------------

describe("HighlightedSource", () => {
  async function renderComponent(
    props: Parameters<typeof import("../components/editor/HighlightedSource").HighlightedSource>[0],
  ) {
    const { HighlightedSource } = await import("../components/editor/HighlightedSource");
    return render(<HighlightedSource {...props} />);
  }

  it("renders plain text when no matches", async () => {
    const { container } = await renderComponent({
      text: "Hello world",
      termMatches: [],
    });
    expect(container.textContent).toBe("Hello world");
  });

  it("renders empty string for empty text", async () => {
    const { container } = await renderComponent({
      text: "",
      termMatches: [],
    });
    expect(container.textContent).toBe("");
  });

  it("highlights term matches with title tooltip", async () => {
    const { container } = await renderComponent({
      text: "Say hello friend",
      termMatches: [termMatch({ source_term: "hello", target_terms: ["hola"], start: 4, end: 9, status: "preferred" })],
    });
    const highlighted = container.querySelector(".underline.decoration-dotted");
    expect(highlighted).not.toBeNull();
    expect(highlighted!.textContent).toBe("hello");
    expect(highlighted!.getAttribute("title")).toContain("hola");
  });

  it("highlights entity with entity type label in title", async () => {
    const { container } = await renderComponent({
      text: "Visit Acme Corp today",
      termMatches: [],
      entities: [entity({ text: "Acme Corp", type: "entity:organization", start: 6, end: 15 })],
    });
    expect(container.textContent).toContain("Acme Corp");
    const entitySpan = container.querySelector("[title]");
    expect(entitySpan).not.toBeNull();
    expect(entitySpan!.getAttribute("title")).toContain("Organization");
  });
});

// ---------------------------------------------------------------------------
// FormattedSourceDisplay
// ---------------------------------------------------------------------------

describe("FormattedSourceDisplay", () => {
  async function renderComponent(
    props: Parameters<typeof import("../components/editor/FormattedSourceDisplay").FormattedSourceDisplay>[0],
  ) {
    const { FormattedSourceDisplay } = await import("../components/editor/FormattedSourceDisplay");
    return render(<FormattedSourceDisplay {...props} />);
  }

  it("renders plain text with no spans", async () => {
    const { container } = await renderComponent({ codedText: "Hello world", spans: [] });
    expect(container.textContent).toBe("Hello world");
  });

  it("renders text with bold formatting applied", async () => {
    const spans: SpanInfo[] = [
      span("opening", "fmt:bold", "<b>"),
      span("closing", "fmt:bold", "</b>"),
    ];
    const coded = "\uE001bold\uE002";
    const { container } = await renderComponent({ codedText: coded, spans });
    expect(container.textContent).toBe("bold");
  });

  it("renders empty text", async () => {
    const { container } = await renderComponent({ codedText: "", spans: [] });
    expect(container.textContent).toBe("");
  });
});

// ---------------------------------------------------------------------------
// SourceCellDisplay
// ---------------------------------------------------------------------------

describe("SourceCellDisplay", () => {
  async function renderComponent(
    props: Parameters<typeof import("../components/editor/SourceCellDisplay").SourceCellDisplay>[0],
  ) {
    const { SourceCellDisplay } = await import("../components/editor/SourceCellDisplay");
    return render(<SourceCellDisplay {...props} />);
  }

  it("renders plain text with no spans", async () => {
    const { container } = await renderComponent({ codedText: "Hello world", spans: [] });
    expect(container.textContent).toBe("Hello world");
  });

  it("renders text segments and tag chips for coded text", async () => {
    const spans: SpanInfo[] = [
      span("opening", "fmt:bold", "<b>"),
      span("closing", "fmt:bold", "</b>"),
    ];
    const coded = "before \uE001bold\uE002 after";
    const { container } = await renderComponent({ codedText: coded, spans });
    expect(container.textContent).toContain("before");
    expect(container.textContent).toContain("bold");
    expect(container.textContent).toContain("after");
    const chips = container.querySelectorAll("[data-tag-chip]");
    expect(chips.length).toBe(2);
  });

  it("renders entity highlights on text segments", async () => {
    const entities: EntityInfo[] = [
      entity({ text: "hello", type: "entity:person", start: 0, end: 5 }),
    ];
    const { container } = await renderComponent({
      codedText: "hello world",
      spans: [],
      entities,
    });
    const entityEl = container.querySelector("[title]");
    expect(entityEl).not.toBeNull();
    expect(entityEl!.getAttribute("title")).toContain("person");
  });
});

// ---------------------------------------------------------------------------
// TagChipComponent
// ---------------------------------------------------------------------------

describe("TagChipComponent", () => {
  async function renderComponent(
    props: Parameters<typeof import("../components/editor/TagChipComponent").TagChipComponent>[0],
  ) {
    const { TagChipComponent } = await import("../components/editor/TagChipComponent");
    return render(<TagChipComponent {...props} />);
  }

  it("renders a tag chip with label", async () => {
    const { container } = await renderComponent({
      spanInfo: span("opening", "fmt:bold", "<b>"),
    });
    const chip = container.querySelector("[data-tag-chip]");
    expect(chip).not.toBeNull();
    expect(chip!.textContent!.length).toBeGreaterThan(0);
  });

  it("renders index when provided", async () => {
    const { container } = await renderComponent({
      spanInfo: span("opening", "fmt:bold", "<b>"),
      index: 3,
    });
    expect(container.textContent).toContain("3");
  });

  it("renders pair badge when pairIndex is provided", async () => {
    const { container } = await renderComponent({
      spanInfo: span("opening", "fmt:bold", "<b>"),
      pairIndex: 2,
    });
    expect(container.textContent).toContain("2");
  });

  it("sets data-span-type and data-category attributes", async () => {
    const { container } = await renderComponent({
      spanInfo: span("opening", "fmt:bold", "<b>"),
    });
    const chip = container.querySelector("[data-tag-chip]");
    expect(chip!.getAttribute("data-span-type")).toBe("fmt:bold");
    expect(chip!.getAttribute("data-category")).toBeTruthy();
  });

  it("applies highlight glow via box-shadow when highlighted", async () => {
    const { container } = await renderComponent({
      spanInfo: span("opening", "fmt:bold", "<b>"),
      highlighted: true,
    });
    const chip = container.querySelector("[data-tag-chip]") as HTMLElement;
    expect(chip.style.boxShadow).toBeTruthy();
  });

  it("reduces opacity when dimmed", async () => {
    const { container } = await renderComponent({
      spanInfo: span("opening", "fmt:bold", "<b>"),
      dimmed: true,
    });
    const chip = container.querySelector("[data-tag-chip]") as HTMLElement;
    expect(chip.style.opacity).toBe("0.4");
  });

  it("uses dashed border when locked", async () => {
    const { container } = await renderComponent({
      spanInfo: span("opening", "fmt:bold", "<b>"),
      locked: true,
    });
    const chip = container.querySelector("[data-tag-chip]") as HTMLElement;
    expect(chip.style.borderStyle).toBe("dashed");
  });

  it("shows constraint indicator when showConstraints and non-deletable", async () => {
    const { container } = await renderComponent({
      spanInfo: span("placeholder", "code:variable", "{name}"),
      showConstraints: true,
    });
    const required = container.querySelector("[aria-label='required']");
    expect(required).not.toBeNull();
  });

  it("renders different tag types (placeholder)", async () => {
    const { container } = await renderComponent({
      spanInfo: span("placeholder", "struct:break", "<br/>"),
    });
    const chip = container.querySelector("[data-tag-chip]");
    expect(chip).not.toBeNull();
    expect(chip!.getAttribute("data-span-type")).toBe("struct:break");
  });
});

// ---------------------------------------------------------------------------
// TagPalette
// ---------------------------------------------------------------------------

describe("TagPalette", () => {
  async function renderComponent(
    props: Parameters<typeof import("../components/editor/TagPalette").TagPalette>[0],
  ) {
    const { TagPalette } = await import("../components/editor/TagPalette");
    return render(<TagPalette {...props} />);
  }

  it("returns null when sourceSpans is empty", async () => {
    const { container } = await renderComponent({
      sourceSpans: [],
      onInsert: vi.fn(),
    });
    expect(container.innerHTML).toBe("");
  });

  it("renders tag items for each source span", async () => {
    const spans: SpanInfo[] = [
      span("opening", "fmt:bold", "<b>"),
      span("closing", "fmt:bold", "</b>"),
    ];
    const { container } = await renderComponent({
      sourceSpans: spans,
      onInsert: vi.fn(),
    });
    expect(container.textContent).toContain("Tags:");
    const buttons = container.querySelectorAll("[data-testid^='tag-palette-']");
    expect(buttons.length).toBe(2);
  });

  it("calls onInsert when a tag button is clicked", async () => {
    const user = userEvent.setup();
    const onInsert = vi.fn();
    const spans: SpanInfo[] = [
      span("opening", "fmt:bold", "<b>"),
      span("closing", "fmt:bold", "</b>"),
    ];
    await renderComponent({ sourceSpans: spans, onInsert });
    const button = screen.getByTestId("tag-palette-0");
    await user.click(button);
    expect(onInsert).toHaveBeenCalledWith(spans[0]);
  });

  it("dims used spans and blocks non-cloneable ones", async () => {
    const spans: SpanInfo[] = [
      span("placeholder", "code:variable", "{name}"),
    ];
    const { container } = await renderComponent({
      sourceSpans: spans,
      onInsert: vi.fn(),
      usedSpans: spans,
    });
    const button = container.querySelector("[data-testid='tag-palette-0']") as HTMLButtonElement;
    expect(button.disabled).toBe(true);
  });
});

// ---------------------------------------------------------------------------
// ProblemsPanel
// ---------------------------------------------------------------------------

describe("ProblemsPanel", () => {
  async function renderComponent(
    props: Parameters<typeof import("../components/editor/ProblemsPanel").ProblemsPanel>[0],
  ) {
    const { ProblemsPanel } = await import("../components/editor/ProblemsPanel");
    return render(<ProblemsPanel {...props} />);
  }

  it("shows 'No issues found' when issues list is empty", async () => {
    await renderComponent({
      issues: [],
      onNavigateToBlock: vi.fn(),
      onClose: vi.fn(),
    });
    expect(screen.getByText("No issues found")).toBeInTheDocument();
  });

  it("renders problem messages", async () => {
    const issues: FileQAResult[] = [
      {
        blockId: "block-1",
        issues: [
          { type: "missing_tag", severity: "error", message: "Missing bold tag" },
        ],
      },
    ];
    await renderComponent({
      issues,
      onNavigateToBlock: vi.fn(),
      onClose: vi.fn(),
    });
    expect(screen.getByText("Missing bold tag")).toBeInTheDocument();
    expect(screen.getByText("Error")).toBeInTheDocument();
  });

  it("renders warnings", async () => {
    const issues: FileQAResult[] = [
      {
        blockId: "block-1",
        issues: [
          { type: "extra_tag", severity: "warning", message: "Extra italic tag" },
        ],
      },
    ];
    await renderComponent({
      issues,
      onNavigateToBlock: vi.fn(),
      onClose: vi.fn(),
    });
    expect(screen.getByText("Extra italic tag")).toBeInTheDocument();
    expect(screen.getByText("Warning")).toBeInTheDocument();
  });

  it("shows loading state", async () => {
    await renderComponent({
      issues: [],
      loading: true,
      onNavigateToBlock: vi.fn(),
      onClose: vi.fn(),
    });
    expect(screen.getByText("Running QA checks...")).toBeInTheDocument();
  });

  it("calls onClose when close button is clicked", async () => {
    const user = userEvent.setup();
    const onClose = vi.fn();
    await renderComponent({
      issues: [],
      onNavigateToBlock: vi.fn(),
      onClose,
    });
    const closeBtn = screen.getByLabelText("Close problems panel");
    await user.click(closeBtn);
    expect(onClose).toHaveBeenCalledOnce();
  });

  it("calls onNavigateToBlock when a row is clicked", async () => {
    const user = userEvent.setup();
    const onNavigate = vi.fn();
    const issues: FileQAResult[] = [
      {
        blockId: "blk-42",
        issues: [{ type: "check", severity: "error", message: "Problem here" }],
      },
    ];
    await renderComponent({
      issues,
      onNavigateToBlock: onNavigate,
      onClose: vi.fn(),
    });
    const row = screen.getByText("Problem here").closest("tr")!;
    await user.click(row);
    expect(onNavigate).toHaveBeenCalledWith("blk-42");
  });

  it("displays total issue count badge", async () => {
    const issues: FileQAResult[] = [
      {
        blockId: "b1",
        issues: [
          { type: "a", severity: "error", message: "err1" },
          { type: "b", severity: "warning", message: "warn1" },
        ],
      },
    ];
    await renderComponent({
      issues,
      onNavigateToBlock: vi.fn(),
      onClose: vi.fn(),
    });
    expect(screen.getByText("2")).toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// FormatVocabularyBadge
// ---------------------------------------------------------------------------

describe("FormatVocabularyBadge", () => {
  async function renderComponent(
    props: Parameters<typeof import("../components/editor/FormatVocabularyBadge").FormatVocabularyBadge>[0],
  ) {
    const { FormatVocabularyBadge } = await import("../components/editor/FormatVocabularyBadge");
    return render(<FormatVocabularyBadge {...props} />);
  }

  it("returns null when spans is empty", async () => {
    const { container } = await renderComponent({ spans: [] });
    expect(container.innerHTML).toBe("");
  });

  it("renders tag count for non-empty spans", async () => {
    const spans: SpanInfo[] = [
      span("opening", "fmt:bold", "<b>"),
      span("closing", "fmt:bold", "</b>"),
      span("placeholder", "struct:break", "<br/>"),
    ];
    const { container } = await renderComponent({ spans });
    expect(container.textContent).toContain("3 tags");
  });

  it("calls onClick when clicked", async () => {
    const user = userEvent.setup();
    const onClick = vi.fn();
    const spans: SpanInfo[] = [span("opening", "fmt:bold", "<b>")];
    await renderComponent({ spans, onClick });
    const button = screen.getByRole("button");
    await user.click(button);
    expect(onClick).toHaveBeenCalledOnce();
  });

  it("shows singular 'tag' for a single span", async () => {
    const spans: SpanInfo[] = [span("placeholder", "struct:break", "<br/>")];
    const { container } = await renderComponent({ spans });
    expect(container.textContent).toContain("1 tag");
    expect(container.textContent).not.toContain("1 tags");
  });
});

// ---------------------------------------------------------------------------
// InlinePreview
// ---------------------------------------------------------------------------

describe("InlinePreview", () => {
  async function renderComponent(
    props: Parameters<typeof import("../components/editor/InlinePreview").InlinePreview>[0],
  ) {
    const { InlinePreview } = await import("../components/editor/InlinePreview");
    return render(<InlinePreview {...props} />);
  }

  it("returns null for empty codedText", async () => {
    const { container } = await renderComponent({ codedText: "", spans: [] });
    expect(container.innerHTML).toBe("");
  });

  it("renders preview label and text content", async () => {
    const { container } = await renderComponent({ codedText: "Hello world", spans: [] });
    expect(container.textContent).toContain("Preview:");
    expect(container.textContent).toContain("Hello world");
  });

  it("renders formatted HTML from coded text", async () => {
    const spans: SpanInfo[] = [
      span("opening", "fmt:bold", "<b>"),
      span("closing", "fmt:bold", "</b>"),
    ];
    const coded = "\uE001bold text\uE002";
    const { container } = await renderComponent({ codedText: coded, spans });
    // The preview renders HTML content; bold tag should be present
    const bold = container.querySelector("b");
    expect(bold).not.toBeNull();
    expect(bold!.textContent).toBe("bold text");
  });
});

// ---------------------------------------------------------------------------
// EntityPopover
// ---------------------------------------------------------------------------

describe("EntityPopover", () => {
  async function renderComponent(
    props: Parameters<typeof import("../components/editor/EntityPopover").EntityPopover>[0],
  ) {
    const { EntityPopover } = await import("../components/editor/EntityPopover");
    return render(<EntityPopover {...props} />);
  }

  it("renders entity text and type selector", async () => {
    await renderComponent({
      entity: entity({ text: "Acme Corp" }),
      onClose: vi.fn(),
    });
    expect(screen.getByText("Acme Corp")).toBeInTheDocument();
    expect(screen.getByLabelText("Do Not Translate")).toBeInTheDocument();
  });

  it("shows Delete button when onDelete is provided", async () => {
    await renderComponent({
      entity: entity(),
      onClose: vi.fn(),
      onDelete: vi.fn(),
    });
    expect(screen.getByText("Delete")).toBeInTheDocument();
  });

  it("shows Promote button when onPromote is provided", async () => {
    await renderComponent({
      entity: entity(),
      onClose: vi.fn(),
      onPromote: vi.fn(),
    });
    expect(screen.getByText("Promote")).toBeInTheDocument();
  });

  it("calls onDelete and onClose when Delete is clicked", async () => {
    const user = userEvent.setup();
    const onDelete = vi.fn();
    const onClose = vi.fn();
    const e = entity({ key: "ent-99" });
    await renderComponent({ entity: e, onClose, onDelete });
    await user.click(screen.getByText("Delete"));
    expect(onDelete).toHaveBeenCalledWith("ent-99");
    expect(onClose).toHaveBeenCalled();
  });

  it("calls onPromote and onClose when Promote is clicked", async () => {
    const user = userEvent.setup();
    const onPromote = vi.fn();
    const onClose = vi.fn();
    const e = entity({ key: "ent-7" });
    await renderComponent({ entity: e, onClose, onPromote });
    await user.click(screen.getByText("Promote"));
    expect(onPromote).toHaveBeenCalledWith("ent-7");
    expect(onClose).toHaveBeenCalled();
  });
});

// ---------------------------------------------------------------------------
// EntityMarkPopover
// ---------------------------------------------------------------------------

describe("EntityMarkPopover", () => {
  async function renderComponent(
    props: Parameters<typeof import("../components/editor/EntityMarkPopover").EntityMarkPopover>[0],
  ) {
    const { EntityMarkPopover } = await import("../components/editor/EntityMarkPopover");
    return render(<EntityMarkPopover {...props} />);
  }

  it("renders selected text and Mark Entity button", async () => {
    await renderComponent({
      text: "Acme",
      start: 0,
      end: 4,
      position: { x: 100, y: 100 },
      onConfirm: vi.fn(),
      onCancel: vi.fn(),
    });
    expect(screen.getByText("Mark Entity")).toBeInTheDocument();
    expect(screen.getByText("Cancel")).toBeInTheDocument();
  });

  it("calls onCancel when Cancel is clicked", async () => {
    const user = userEvent.setup();
    const onCancel = vi.fn();
    await renderComponent({
      text: "Test",
      start: 0,
      end: 4,
      position: { x: 0, y: 0 },
      onConfirm: vi.fn(),
      onCancel,
    });
    await user.click(screen.getByText("Cancel"));
    expect(onCancel).toHaveBeenCalledOnce();
  });

  it("calls onConfirm with selected type when Mark Entity is clicked", async () => {
    const user = userEvent.setup();
    const onConfirm = vi.fn();
    await renderComponent({
      text: "Berlin",
      start: 0,
      end: 6,
      position: { x: 0, y: 0 },
      onConfirm,
      onCancel: vi.fn(),
    });
    await user.click(screen.getByText("Mark Entity"));
    expect(onConfirm).toHaveBeenCalledWith("entity:other", false);
  });
});

// ---------------------------------------------------------------------------
// TermSidebar
// ---------------------------------------------------------------------------

describe("TermSidebar", () => {
  async function renderComponent(
    props: Parameters<typeof import("../components/editor/TermSidebar").TermSidebar>[0],
  ) {
    const { TermSidebar } = await import("../components/editor/TermSidebar");
    return render(<TermSidebar {...props} />);
  }

  it("shows 'No terminology matches' when empty", async () => {
    await renderComponent({
      termMatches: [],
      onInsertTerm: vi.fn(),
    });
    expect(screen.getByText("No terminology matches")).toBeInTheDocument();
  });

  it("renders term matches with source term and status", async () => {
    const matches: BlockTermMatch[] = [
      termMatch({ source_term: "file", target_terms: ["archivo"], status: "preferred" }),
    ];
    await renderComponent({
      termMatches: matches,
      onInsertTerm: vi.fn(),
    });
    expect(screen.getByText("file")).toBeInTheDocument();
    expect(screen.getByText("preferred")).toBeInTheDocument();
    expect(screen.getByText("archivo")).toBeInTheDocument();
  });

  it("calls onInsertTerm when target term is clicked", async () => {
    const user = userEvent.setup();
    const onInsert = vi.fn();
    const matches: BlockTermMatch[] = [
      termMatch({ source_term: "save", target_terms: ["guardar", "salvar"] }),
    ];
    await renderComponent({ termMatches: matches, onInsertTerm: onInsert });
    await user.click(screen.getByText("guardar"));
    expect(onInsert).toHaveBeenCalledWith("guardar");
  });

  it("shows count badge when matches exist", async () => {
    const matches: BlockTermMatch[] = [
      termMatch({ source_term: "a" }),
      termMatch({ source_term: "b" }),
    ];
    await renderComponent({ termMatches: matches, onInsertTerm: vi.fn() });
    expect(screen.getByText("2")).toBeInTheDocument();
  });

  it("shows loading spinner when loading", async () => {
    const { container } = await renderComponent({
      termMatches: [],
      onInsertTerm: vi.fn(),
      loading: true,
    });
    const spinner = container.querySelector(".animate-spin");
    expect(spinner).not.toBeNull();
  });

  it("shows 'No target term defined' for matches without target terms", async () => {
    const matches: BlockTermMatch[] = [
      termMatch({ source_term: "orphan", target_terms: [] }),
    ];
    await renderComponent({ termMatches: matches, onInsertTerm: vi.fn() });
    expect(screen.getByText("No target term defined")).toBeInTheDocument();
  });

  it("shows domain badge when present", async () => {
    const matches: BlockTermMatch[] = [
      termMatch({ source_term: "term", domain: "legal" }),
    ];
    await renderComponent({ termMatches: matches, onInsertTerm: vi.fn() });
    expect(screen.getByText("legal")).toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// VocabularyExplorer
// ---------------------------------------------------------------------------

describe("VocabularyExplorer", () => {
  async function renderComponent(
    props: Parameters<typeof import("../components/editor/VocabularyExplorer").VocabularyExplorer>[0] = {},
  ) {
    const { VocabularyExplorer } = await import("../components/editor/VocabularyExplorer");
    return render(<VocabularyExplorer {...props} />);
  }

  it("renders category groups from the vocabulary registry", async () => {
    const { container } = await renderComponent();
    const categoryButtons = container.querySelectorAll("button");
    expect(categoryButtons.length).toBeGreaterThan(0);
  });

  it("expands a category when its header is clicked", async () => {
    const user = userEvent.setup();
    const { container } = await renderComponent();
    const firstButton = container.querySelector("button")!;
    await user.click(firstButton);
    const chips = container.querySelectorAll("[data-tag-chip]");
    expect(chips.length).toBeGreaterThan(0);
  });
});

// ---------------------------------------------------------------------------
// InlineCodeLegend
// ---------------------------------------------------------------------------

describe("InlineCodeLegend", () => {
  async function renderComponent(
    props: Parameters<typeof import("../components/editor/InlineCodeLegend").InlineCodeLegend>[0],
  ) {
    const { InlineCodeLegend } = await import("../components/editor/InlineCodeLegend");
    return render(<InlineCodeLegend {...props} />);
  }

  it("returns null when spans is empty", async () => {
    const { container } = await renderComponent({ spans: [], onClose: vi.fn() });
    expect(container.innerHTML).toBe("");
  });

  it("renders header and legend entries for non-empty spans", async () => {
    const spans: SpanInfo[] = [
      span("opening", "fmt:bold", "<b>"),
      span("closing", "fmt:bold", "</b>"),
    ];
    await renderComponent({ spans, onClose: vi.fn() });
    expect(screen.getByText("Inline Tags in This Segment")).toBeInTheDocument();
  });

  it("calls onClose when close button is clicked", async () => {
    const user = userEvent.setup();
    const onClose = vi.fn();
    const spans: SpanInfo[] = [span("placeholder", "struct:break", "<br/>")];
    const { container } = await renderComponent({ spans, onClose });
    const closeBtn = container.querySelector("button")!;
    await user.click(closeBtn);
    expect(onClose).toHaveBeenCalledOnce();
  });
});
