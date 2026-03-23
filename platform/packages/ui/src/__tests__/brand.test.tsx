import { describe, it, expect, vi } from "vite-plus/test";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { BrandScoreGauge } from "../brand/BrandScoreGauge";
import { BrandDimensionBreakdown } from "../brand/BrandDimensionBreakdown";
import { BrandExamplePair } from "../brand/BrandExamplePair";
import { BrandFindingsList } from "../brand/BrandFindingsList";
import { BrandProfileCard } from "../brand/BrandProfileCard";
import { BrandProfileList } from "../brand/BrandProfileList";
import { BreadcrumbProvider } from "../context/BreadcrumbContext";
import type { DimensionScore, VoiceExample, BrandVoiceFinding, VoiceProfile } from "../brand/types";

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function makeProfile(overrides: Partial<VoiceProfile> = {}): VoiceProfile {
  return {
    id: "prof-1",
    name: "Friendly Brand",
    description: "A warm and approachable voice",
    tone: {
      personality: ["friendly", "helpful", "optimistic"],
      formality: "casual",
      emotion: "warm",
      humor: "light",
    },
    style: {
      active_voice: true,
      sentence_length: "short",
      person_pov: "second",
      contractions: "always",
    },
    vocabulary: {
      preferred_terms: [{ term: "help" }],
      forbidden_terms: [{ term: "synergy" }],
    },
    examples: [],
    workspace_id: "ws-1",
    version: 2,
    created_at: "2025-01-01T00:00:00Z",
    updated_at: "2025-01-02T00:00:00Z",
    ...overrides,
  };
}

function makeDimension(overrides: Partial<DimensionScore> = {}): DimensionScore {
  return {
    dimension: "tone",
    score: 85,
    penalty: 5,
    issues: 0,
    ...overrides,
  };
}

function makeFinding(overrides: Partial<BrandVoiceFinding> = {}): BrandVoiceFinding {
  return {
    dimension: "tone",
    severity: "minor",
    message: "Tone is too formal for the target audience",
    position: { start: 0, end: 10 },
    ...overrides,
  };
}

// ---------------------------------------------------------------------------
// BrandScoreGauge
// ---------------------------------------------------------------------------

describe("BrandScoreGauge", () => {
  it("renders the score number", () => {
    render(<BrandScoreGauge score={75} />);
    expect(screen.getByText("75")).toBeInTheDocument();
  });

  it("clamps score below 0 to 0", () => {
    render(<BrandScoreGauge score={-20} />);
    expect(screen.getByText("0")).toBeInTheDocument();
  });

  it("clamps score above 100 to 100", () => {
    render(<BrandScoreGauge score={150} />);
    expect(screen.getByText("100")).toBeInTheDocument();
  });

  it("renders label when provided", () => {
    render(<BrandScoreGauge score={80} label="Overall" />);
    expect(screen.getByText("Overall")).toBeInTheDocument();
  });

  it("does not render label when not provided", () => {
    const { container } = render(<BrandScoreGauge score={80} />);
    expect(container.querySelector(".text-xs.text-muted-foreground")).toBeNull();
  });
});

// ---------------------------------------------------------------------------
// BrandDimensionBreakdown
// ---------------------------------------------------------------------------

describe("BrandDimensionBreakdown", () => {
  const dimensions: DimensionScore[] = [
    makeDimension({ dimension: "tone", score: 90, issues: 0 }),
    makeDimension({ dimension: "style", score: 72, issues: 2 }),
    makeDimension({ dimension: "vocabulary", score: 55, issues: 3 }),
    makeDimension({ dimension: "clarity", score: 40, issues: 1 }),
    makeDimension({ dimension: "brand_compliance", score: 88, issues: 0 }),
  ];

  it("renders dimension labels", () => {
    render(<BrandDimensionBreakdown dimensions={dimensions} />);
    expect(screen.getByText("Tone")).toBeInTheDocument();
    expect(screen.getByText("Style")).toBeInTheDocument();
    expect(screen.getByText("Vocabulary")).toBeInTheDocument();
    expect(screen.getByText("Clarity")).toBeInTheDocument();
    expect(screen.getByText("Brand")).toBeInTheDocument();
  });

  it("renders scores", () => {
    render(<BrandDimensionBreakdown dimensions={dimensions} />);
    expect(screen.getByText("90")).toBeInTheDocument();
    expect(screen.getByText("72")).toBeInTheDocument();
    expect(screen.getByText("55")).toBeInTheDocument();
    expect(screen.getByText("40")).toBeInTheDocument();
    expect(screen.getByText("88")).toBeInTheDocument();
  });

  it("shows issue count when greater than 0", () => {
    render(<BrandDimensionBreakdown dimensions={dimensions} />);
    expect(screen.getByText("2 issues")).toBeInTheDocument();
    expect(screen.getByText("3 issues")).toBeInTheDocument();
    expect(screen.getByText("1 issue")).toBeInTheDocument();
  });

  it("does not show issue count when issues is 0", () => {
    render(
      <BrandDimensionBreakdown
        dimensions={[makeDimension({ dimension: "tone", score: 90, issues: 0 })]}
      />,
    );
    expect(screen.queryByText(/issue/)).toBeNull();
  });
});

// ---------------------------------------------------------------------------
// BrandExamplePair
// ---------------------------------------------------------------------------

describe("BrandExamplePair", () => {
  it("renders before and after text", () => {
    const example: VoiceExample = {
      before: "We are pleased to inform you",
      after: "Great news!",
    };
    render(<BrandExamplePair example={example} />);
    expect(screen.getByText("We are pleased to inform you")).toBeInTheDocument();
    expect(screen.getByText("Great news!")).toBeInTheDocument();
    expect(screen.getByText("Before")).toBeInTheDocument();
    expect(screen.getByText("After")).toBeInTheDocument();
  });

  it("renders explanation when provided", () => {
    const example: VoiceExample = {
      before: "Old text",
      after: "New text",
      explanation: "More casual tone matches our brand",
    };
    render(<BrandExamplePair example={example} />);
    expect(screen.getByText("More casual tone matches our brand")).toBeInTheDocument();
  });

  it("hides explanation when not provided", () => {
    const example: VoiceExample = { before: "Old text", after: "New text" };
    const { container } = render(<BrandExamplePair example={example} />);
    // Only the before/after paragraphs, no explanation paragraph
    const paragraphs = container.querySelectorAll("p");
    expect(paragraphs).toHaveLength(2);
  });
});

// ---------------------------------------------------------------------------
// BrandFindingsList
// ---------------------------------------------------------------------------

describe("BrandFindingsList", () => {
  it("renders findings with severity badges", () => {
    const findings = [
      makeFinding({ severity: "minor", message: "Slightly too formal" }),
      makeFinding({ severity: "critical", message: "Forbidden term used" }),
    ];
    render(<BrandFindingsList findings={findings} />);
    expect(screen.getByText("Slightly too formal")).toBeInTheDocument();
    expect(screen.getByText("Forbidden term used")).toBeInTheDocument();
    expect(screen.getByText("minor")).toBeInTheDocument();
    expect(screen.getByText("critical")).toBeInTheDocument();
  });

  it("shows suggestion when present", () => {
    const findings = [makeFinding({ message: "Too wordy", suggestion: "Use shorter sentences" })];
    render(<BrandFindingsList findings={findings} />);
    expect(screen.getByText("Suggestion: Use shorter sentences")).toBeInTheDocument();
  });

  it("shows empty message when no findings", () => {
    render(<BrandFindingsList findings={[]} />);
    expect(screen.getByText("No findings. Content is fully compliant.")).toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// BrandProfileCard
// ---------------------------------------------------------------------------

describe("BrandProfileCard", () => {
  it("renders profile name and description", () => {
    const profile = makeProfile();
    render(<BrandProfileCard profile={profile} onClick={vi.fn()} />);
    expect(screen.getByText("Friendly Brand")).toBeInTheDocument();
    expect(screen.getByText("A warm and approachable voice")).toBeInTheDocument();
  });

  it("shows personality tags and formality badge", () => {
    const profile = makeProfile();
    render(<BrandProfileCard profile={profile} onClick={vi.fn()} />);
    expect(screen.getByText("friendly")).toBeInTheDocument();
    expect(screen.getByText("helpful")).toBeInTheDocument();
    expect(screen.getByText("optimistic")).toBeInTheDocument();
    expect(screen.getByText("casual")).toBeInTheDocument();
  });

  it("calls onClick when card is clicked", async () => {
    const user = userEvent.setup();
    const handleClick = vi.fn();
    const profile = makeProfile();
    render(<BrandProfileCard profile={profile} onClick={handleClick} />);
    await user.click(screen.getByText("Friendly Brand"));
    expect(handleClick).toHaveBeenCalledWith(profile);
  });

  it("calls onDelete with stopPropagation when delete button clicked", async () => {
    const user = userEvent.setup();
    const handleClick = vi.fn();
    const handleDelete = vi.fn();
    const profile = makeProfile();
    render(<BrandProfileCard profile={profile} onClick={handleClick} onDelete={handleDelete} />);
    // The delete button contains a Trash2 icon; find the button by role
    const deleteButton = screen.getByRole("button");
    await user.click(deleteButton);
    expect(handleDelete).toHaveBeenCalledWith(profile);
    // onClick on the card should NOT have been triggered
    expect(handleClick).not.toHaveBeenCalled();
  });

  it("hides delete button when onDelete is not provided", () => {
    const profile = makeProfile();
    render(<BrandProfileCard profile={profile} onClick={vi.fn()} />);
    expect(screen.queryByRole("button")).toBeNull();
  });
});

// ---------------------------------------------------------------------------
// BrandProfileList
// ---------------------------------------------------------------------------

describe("BrandProfileList", () => {
  const defaultProps = {
    onSelect: vi.fn(),
    onCreate: vi.fn(),
    onCreateFromStarter: vi.fn(),
    onDelete: vi.fn().mockResolvedValue(undefined),
  };

  function renderList(profiles: VoiceProfile[] = [], overrides = {}) {
    return render(
      <BreadcrumbProvider>
        <BrandProfileList profiles={profiles} {...defaultProps} {...overrides} />
      </BreadcrumbProvider>,
    );
  }

  it("renders profile cards", () => {
    const profiles = [
      makeProfile({ id: "p1", name: "Brand A" }),
      makeProfile({ id: "p2", name: "Brand B" }),
    ];
    renderList(profiles);
    expect(screen.getByText("Brand A")).toBeInTheDocument();
    expect(screen.getByText("Brand B")).toBeInTheDocument();
  });

  it("shows empty state when no profiles", () => {
    renderList([]);
    expect(
      screen.getByText(
        "No brand voice profiles yet. Create one to define your brand's writing style.",
      ),
    ).toBeInTheDocument();
  });

  it("search filters profiles", async () => {
    const user = userEvent.setup();
    const profiles = [
      makeProfile({ id: "p1", name: "Technical Docs" }),
      makeProfile({ id: "p2", name: "Marketing Copy" }),
    ];
    renderList(profiles);
    const searchInput = screen.getByPlaceholderText("Search profiles...");
    await user.type(searchInput, "Technical");
    expect(screen.getByText("Technical Docs")).toBeInTheDocument();
    expect(screen.queryByText("Marketing Copy")).toBeNull();
  });

  it("calls onCreate when New Profile is clicked", async () => {
    const user = userEvent.setup();
    const onCreate = vi.fn();
    renderList([makeProfile()], { onCreate });
    await user.click(screen.getByText("New Profile"));
    expect(onCreate).toHaveBeenCalled();
  });
});
