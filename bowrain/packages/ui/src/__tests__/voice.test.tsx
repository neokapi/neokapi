import { describe, it, expect, vi } from "vite-plus/test";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { VoiceScoreGauge } from "../voice/VoiceScoreGauge";
import { VoiceDimensionBreakdown } from "../voice/VoiceDimensionBreakdown";
import { VoiceExamplePair } from "../voice/VoiceExamplePair";
import { VoiceFindingsList } from "../voice/VoiceFindingsList";
import { VoiceProfileCard } from "../voice/VoiceProfileCard";
import { VoiceProfileList } from "../voice/VoiceProfileList";
import { VoiceDashboard } from "../voice/VoiceDashboard";
import { DEFAULT_MIN_SCORE, barForProfile, complianceBar, scoreBand } from "../voice/complianceBar";
import { BreadcrumbProvider } from "../context/BreadcrumbContext";
import type { DimensionScore, VoiceExample, VoiceFinding, VoiceProfile } from "../voice/types";

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

function makeFinding(overrides: Partial<VoiceFinding> = {}): VoiceFinding {
  return {
    category: "tone",
    severity: "minor",
    message: "Tone is too formal for the target audience",
    position: { kind: "range", start: { run: 0 }, end: { run: 0, offset: 10 } },
    ...overrides,
  };
}

// ---------------------------------------------------------------------------
// VoiceScoreGauge
// ---------------------------------------------------------------------------

describe("VoiceScoreGauge", () => {
  it("renders the score number", () => {
    render(<VoiceScoreGauge score={75} />);
    expect(screen.getByText("75")).toBeInTheDocument();
  });

  it("clamps score below 0 to 0", () => {
    render(<VoiceScoreGauge score={-20} />);
    expect(screen.getByText("0")).toBeInTheDocument();
  });

  it("clamps score above 100 to 100", () => {
    render(<VoiceScoreGauge score={150} />);
    expect(screen.getByText("100")).toBeInTheDocument();
  });

  it("renders label when provided", () => {
    render(<VoiceScoreGauge score={80} label="Overall" />);
    expect(screen.getByText("Overall")).toBeInTheDocument();
  });

  it("does not render label when not provided", () => {
    const { container } = render(<VoiceScoreGauge score={80} />);
    expect(container.querySelector(".text-xs.text-muted-foreground")).toBeNull();
  });
});

// ---------------------------------------------------------------------------
// VoiceDimensionBreakdown
// ---------------------------------------------------------------------------

describe("VoiceDimensionBreakdown", () => {
  const dimensions: DimensionScore[] = [
    makeDimension({ dimension: "tone", score: 90, issues: 0 }),
    makeDimension({ dimension: "style", score: 72, issues: 2 }),
    makeDimension({ dimension: "vocabulary", score: 55, issues: 3 }),
    makeDimension({ dimension: "clarity", score: 40, issues: 1 }),
    makeDimension({ dimension: "compliance", score: 88, issues: 0 }),
  ];

  it("renders dimension labels", () => {
    render(<VoiceDimensionBreakdown dimensions={dimensions} />);
    expect(screen.getByText("Tone")).toBeInTheDocument();
    expect(screen.getByText("Style")).toBeInTheDocument();
    expect(screen.getByText("Vocabulary")).toBeInTheDocument();
    expect(screen.getByText("Clarity")).toBeInTheDocument();
    expect(screen.getByText("Brand")).toBeInTheDocument();
  });

  it("renders scores", () => {
    render(<VoiceDimensionBreakdown dimensions={dimensions} />);
    expect(screen.getByText("90")).toBeInTheDocument();
    expect(screen.getByText("72")).toBeInTheDocument();
    expect(screen.getByText("55")).toBeInTheDocument();
    expect(screen.getByText("40")).toBeInTheDocument();
    expect(screen.getByText("88")).toBeInTheDocument();
  });

  it("shows issue count when greater than 0", () => {
    render(<VoiceDimensionBreakdown dimensions={dimensions} />);
    expect(screen.getByText("2 issues")).toBeInTheDocument();
    expect(screen.getByText("3 issues")).toBeInTheDocument();
    expect(screen.getByText("1 issue")).toBeInTheDocument();
  });

  it("does not show issue count when issues is 0", () => {
    render(
      <VoiceDimensionBreakdown
        dimensions={[makeDimension({ dimension: "tone", score: 90, issues: 0 })]}
      />,
    );
    expect(screen.queryByText(/issue/)).toBeNull();
  });
});

// ---------------------------------------------------------------------------
// VoiceExamplePair
// ---------------------------------------------------------------------------

describe("VoiceExamplePair", () => {
  it("renders before and after text", () => {
    const example: VoiceExample = {
      before: "We are pleased to inform you",
      after: "Great news!",
    };
    render(<VoiceExamplePair example={example} />);
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
    render(<VoiceExamplePair example={example} />);
    expect(screen.getByText("More casual tone matches our brand")).toBeInTheDocument();
  });

  it("hides explanation when not provided", () => {
    const example: VoiceExample = { before: "Old text", after: "New text" };
    const { container } = render(<VoiceExamplePair example={example} />);
    // Only the before/after paragraphs, no explanation paragraph
    const paragraphs = container.querySelectorAll("p");
    expect(paragraphs).toHaveLength(2);
  });
});

// ---------------------------------------------------------------------------
// VoiceFindingsList
// ---------------------------------------------------------------------------

describe("VoiceFindingsList", () => {
  it("renders findings with severity badges", () => {
    const findings = [
      makeFinding({ severity: "minor", message: "Slightly too formal" }),
      makeFinding({ severity: "critical", message: "Forbidden term used" }),
    ];
    render(<VoiceFindingsList findings={findings} />);
    expect(screen.getByText("Slightly too formal")).toBeInTheDocument();
    expect(screen.getByText("Forbidden term used")).toBeInTheDocument();
    expect(screen.getByText("minor")).toBeInTheDocument();
    expect(screen.getByText("critical")).toBeInTheDocument();
  });

  it("shows suggestion when present", () => {
    const findings = [makeFinding({ message: "Too wordy", suggestion: "Use shorter sentences" })];
    render(<VoiceFindingsList findings={findings} />);
    expect(screen.getByText("Suggestion: Use shorter sentences")).toBeInTheDocument();
  });

  // The wire field is `category`; the list read `dimension`, which no server
  // response carries, so this chip was empty for every real finding.
  it("names the category the server grouped the finding under", () => {
    render(<VoiceFindingsList findings={[makeFinding({ category: "compliance" })]} />);
    expect(screen.getByText("compliance")).toBeInTheDocument();
  });

  it("shows empty message when no findings", () => {
    render(<VoiceFindingsList findings={[]} />);
    expect(screen.getByText("No findings. Content is fully compliant.")).toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// VoiceProfileCard
// ---------------------------------------------------------------------------

describe("VoiceProfileCard", () => {
  it("renders profile name and description", () => {
    const profile = makeProfile();
    render(<VoiceProfileCard profile={profile} onClick={vi.fn()} />);
    expect(screen.getByText("Friendly Brand")).toBeInTheDocument();
    expect(screen.getByText("A warm and approachable voice")).toBeInTheDocument();
  });

  it("shows personality tags and formality badge", () => {
    const profile = makeProfile();
    render(<VoiceProfileCard profile={profile} onClick={vi.fn()} />);
    expect(screen.getByText("friendly")).toBeInTheDocument();
    expect(screen.getByText("helpful")).toBeInTheDocument();
    expect(screen.getByText("optimistic")).toBeInTheDocument();
    expect(screen.getByText("casual")).toBeInTheDocument();
  });

  it("calls onClick when card is clicked", async () => {
    const user = userEvent.setup();
    const handleClick = vi.fn();
    const profile = makeProfile();
    render(<VoiceProfileCard profile={profile} onClick={handleClick} />);
    await user.click(screen.getByText("Friendly Brand"));
    expect(handleClick).toHaveBeenCalledWith(profile);
  });

  it("calls onDelete with stopPropagation when delete button clicked", async () => {
    const user = userEvent.setup();
    const handleClick = vi.fn();
    const handleDelete = vi.fn();
    const profile = makeProfile();
    render(<VoiceProfileCard profile={profile} onClick={handleClick} onDelete={handleDelete} />);
    // The delete button contains a Trash2 icon; find the button by role
    const deleteButton = screen.getByRole("button");
    await user.click(deleteButton);
    expect(handleDelete).toHaveBeenCalledWith(profile);
    // onClick on the card should NOT have been triggered
    expect(handleClick).not.toHaveBeenCalled();
  });

  it("hides delete button when onDelete is not provided", () => {
    const profile = makeProfile();
    render(<VoiceProfileCard profile={profile} onClick={vi.fn()} />);
    expect(screen.queryByRole("button")).toBeNull();
  });
});

// ---------------------------------------------------------------------------
// VoiceProfileList
// ---------------------------------------------------------------------------

describe("VoiceProfileList", () => {
  const defaultProps = {
    onSelect: vi.fn(),
    onCreate: vi.fn(),
    onDelete: vi.fn().mockResolvedValue(undefined),
  };

  function renderList(profiles: VoiceProfile[] = [], overrides = {}) {
    return render(
      <BreadcrumbProvider>
        <VoiceProfileList profiles={profiles} {...defaultProps} {...overrides} />
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
    // Without onScanVoice (a server without the brand-scan job system) the
    // empty state must not advertise the hosted scan.
    expect(
      screen.getByText(
        "No voice profiles yet. A profile binds one under profiles[].voice: in a recipe. Define one by hand, or draft one locally with the kapi Agent Skill.",
      ),
    ).toBeInTheDocument();
    expect(screen.queryByText("Scan your brand")).not.toBeInTheDocument();
    // The local lane (kapi Agent Skill) is always positioned in the empty state.
    expect(screen.getByText("Have a codebase?")).toBeInTheDocument();
  });

  it("shows the scan CTA in the empty state when onScanVoice is provided", async () => {
    const user = userEvent.setup();
    const onScanVoice = vi.fn();
    renderList([], { onScanVoice });
    await user.click(screen.getByText("Scan your brand"));
    expect(onScanVoice).toHaveBeenCalled();
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

  it("calls onCreate when New profile is clicked", async () => {
    const user = userEvent.setup();
    const onCreate = vi.fn();
    renderList([makeProfile()], { onCreate });
    await user.click(screen.getByText("New profile"));
    expect(onCreate).toHaveBeenCalled();
  });
});

// ---------------------------------------------------------------------------
// The compliance bar (core/profile.ComplianceBar)
// ---------------------------------------------------------------------------

describe("complianceBar", () => {
  it("defaults when a profile sets no bar of its own", () => {
    expect(complianceBar(undefined)).toBe(DEFAULT_MIN_SCORE);
    expect(complianceBar(null)).toBe(DEFAULT_MIN_SCORE);
    expect(complianceBar({})).toBe(DEFAULT_MIN_SCORE);
    expect(complianceBar({ min_score: 0 })).toBe(DEFAULT_MIN_SCORE);
  });

  it("takes the profile's bar, capped at 100", () => {
    expect(complianceBar({ min_score: 90 })).toBe(90);
    expect(complianceBar({ min_score: 140 })).toBe(100);
  });

  it("resolves a profile's bar by id, defaulting when it is not loaded", () => {
    const profiles = [{ id: "p1", min_score: 90 }, { id: "p2" }];
    expect(barForProfile(profiles, "p1")).toBe(90);
    expect(barForProfile(profiles, "p2")).toBe(DEFAULT_MIN_SCORE);
    expect(barForProfile(profiles, "gone")).toBe(DEFAULT_MIN_SCORE);
    expect(barForProfile(undefined, "p1")).toBe(DEFAULT_MIN_SCORE);
  });

  it("bands a score against the bar it is given", () => {
    expect(scoreBand(85, 80)).toBe("pass");
    expect(scoreBand(85, 90)).toBe("warn");
    expect(scoreBand(20, 90)).toBe("fail");
  });
});

describe("a profile with min_score 90 renders an 85 below the bar everywhere", () => {
  const bar = complianceBar(makeProfile({ min_score: 90 }));

  it("in the score gauge", () => {
    const { rerender } = render(<VoiceScoreGauge score={85} bar={bar} />);
    expect(screen.getByTestId("voice-score").dataset.belowBar).toBe("true");
    expect(screen.getByTestId("voice-score").className).toContain("text-warning");
    // The same 85 clears the default bar.
    rerender(<VoiceScoreGauge score={85} />);
    expect(screen.getByTestId("voice-score").dataset.belowBar).toBe("false");
    expect(screen.getByTestId("voice-score").className).toContain("text-success");
  });

  it("in the dimension breakdown", () => {
    const dimensions: DimensionScore[] = [makeDimension({ dimension: "tone", score: 85 })];
    const { rerender } = render(<VoiceDimensionBreakdown dimensions={dimensions} bar={bar} />);
    expect(screen.getByTestId("dimension-score-tone").dataset.belowBar).toBe("true");
    rerender(<VoiceDimensionBreakdown dimensions={dimensions} />);
    expect(screen.getByTestId("dimension-score-tone").dataset.belowBar).toBe("false");
  });

  it("in the compliance dashboard's gauge and recent checks", () => {
    const score = {
      overall: 85,
      dimensions: [makeDimension({ dimension: "tone", score: 85 })],
      findings: [],
    } as unknown as Parameters<typeof VoiceDashboard>[0]["score"];
    const recent = [
      {
        id: "s1",
        project_id: "p",
        stream: "main",
        block_id: "b1",
        profile_id: "prof-1",
        locale: "fr",
        score: 85,
        dimensions: [],
        findings: [],
        checked_at: "2026-01-01T00:00:00Z",
      },
    ];
    render(<VoiceDashboard score={score} trends={[]} recentScores={recent} bar={bar} />);
    expect(screen.getByTestId("voice-score").dataset.belowBar).toBe("true");
    expect(screen.getByTestId("recent-score").dataset.belowBar).toBe("true");
    expect(screen.getByTestId("dimension-score-tone").dataset.belowBar).toBe("true");
  });
});
