import { render, screen } from "./testUtils";
import { describe, it, expect } from "vitest";
import type { Pattern, TermRule, VoiceProfile } from "../types/voice";
import { AdvisoryChip, PatternRuleRow, RulesBlock, TermRuleRow } from "../components/voice/rules";

describe("voice rule rows", () => {
  it("shows a pattern's description as the label and hides the regex behind the affordance", () => {
    const pattern: Pattern = {
      regex: "\\bsynergy\\b",
      description: "Corporate filler.",
      advisory: true,
      rate: { max: 2, per_words: 1000 },
    };
    render(
      <ul>
        <PatternRuleRow pattern={pattern} />
      </ul>,
    );
    const row = screen.getByTestId("voice-pattern");
    expect(row).toHaveTextContent("Corporate filler.");
    expect(row).toHaveTextContent("up to 2 per 1000 words");
    // The regex is not on the row; it lives behind the tooltip affordance.
    expect(row).not.toHaveTextContent("synergy");
    expect(screen.getByTestId("voice-pattern-regex")).toBeInTheDocument();
  });

  it("renders an advisory rule as a neutral chip, not a coloured one", () => {
    render(<AdvisoryChip advisory />);
    const chip = screen.getByTestId("voice-advisory");
    expect(chip).toHaveTextContent("reports only");
    expect(chip.className).toContain("text-muted-foreground");
    expect(chip.className).not.toContain("text-destructive");
    expect(chip.className).not.toContain("amber");
  });

  it("draws no chip for a rule that fails, which is every rule not marked advisory", () => {
    render(<AdvisoryChip advisory={undefined} />);
    expect(screen.queryByTestId("voice-advisory")).not.toBeInTheDocument();
  });

  it("states a term rule's replacement, and says when there is none", () => {
    const withReplacement: TermRule = {
      term: "log in",
      replacement: "sign in",
    };
    const { unmount } = render(
      <ul>
        <TermRuleRow rule={withReplacement} />
      </ul>,
    );
    const row = screen.getByTestId("voice-term-rule");
    expect(row).toHaveTextContent("log in");
    expect(row).toHaveTextContent("sign in");
    expect(row).not.toHaveTextContent("reports only");
    unmount();

    render(
      <ul>
        <TermRuleRow rule={{ term: "bulletproof" }} />
      </ul>,
    );
    expect(screen.getByTestId("voice-term-rule")).toHaveTextContent(
      "no replacement, so tools skip it",
    );
  });

  it("groups every kind of rule under one list, say-this first", () => {
    const profile: VoiceProfile = {
      name: "Northsea",
      vocabulary: {
        preferred_terms: [{ term: "log in", replacement: "sign in" }],
        forbidden_terms: [{ term: "bulletproof" }],
      },
      style: {
        prohibited_patterns: [
          { regex: "\\bsynergy\\b", description: "Corporate filler.", advisory: true },
        ],
      },
    };
    render(<RulesBlock profile={profile} />);
    expect(screen.getByText("Say this")).toBeInTheDocument();
    expect(screen.getByText("Never say")).toBeInTheDocument();
    expect(screen.getByText("Never write")).toBeInTheDocument();
    expect(screen.getAllByTestId("voice-term-rule")).toHaveLength(2);
    expect(screen.getAllByTestId("voice-pattern")).toHaveLength(1);
  });

  it("says a profile constrains no wording when it holds no rules", () => {
    render(<RulesBlock profile={{ name: "Plain" }} />);
    expect(screen.getByText("This profile constrains no wording.")).toBeInTheDocument();
  });
});
