import { render, screen } from "./testUtils";
import { describe, it, expect } from "vite-plus/test";
import type { Pattern, TermRule, VoiceProfile } from "../types/voice";
import { patternOutcome, ruleOutcome, withPatternOutcome } from "../types/voice";
import {
  AdvisoryChip,
  FailsChip,
  PatternRuleRow,
  RulesBlock,
  TermRuleRow,
} from "../components/voice/rules";

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

  it("marks a pattern that fails the check with a neutral chip, and no other pattern", () => {
    const { unmount } = render(
      <ul>
        <PatternRuleRow pattern={{ regex: "x", description: "Marked.", fails: true }} />
      </ul>,
    );
    const chip = screen.getByTestId("voice-fails");
    expect(chip).toHaveTextContent("fails the check");
    expect(chip.className).toContain("text-muted-foreground");
    expect(chip.className).not.toContain("text-destructive");
    unmount();

    for (const pattern of [
      { regex: "x", description: "Unmarked." },
      { regex: "x", description: "Advisory.", advisory: true },
      { regex: "x", description: "Both.", fails: true, advisory: true },
    ]) {
      const { unmount: done } = render(
        <ul>
          <PatternRuleRow pattern={pattern} />
        </ul>,
      );
      expect(screen.queryByTestId("voice-fails")).not.toBeInTheDocument();
      expect(screen.queryByTestId("voice-advisory")).not.toBeInTheDocument();
      done();
    }
  });

  it("draws no fails chip for a pattern that reports", () => {
    render(<FailsChip fails={false} />);
    expect(screen.queryByTestId("voice-fails")).not.toBeInTheDocument();
  });

  it("reads a pattern's outcome from fails and advisory, and a term rule's from advisory", () => {
    expect(patternOutcome({})).toBe("reports");
    expect(patternOutcome({ fails: true })).toBe("fails");
    expect(patternOutcome({ advisory: true })).toBe("reports");
    expect(patternOutcome({ fails: true, advisory: true })).toBe("reports");
    expect(ruleOutcome(undefined)).toBe("fails");
    expect(ruleOutcome(true)).toBe("reports");
  });

  it("writes the chosen outcome as fails and clears advisory", () => {
    const base: Pattern = { regex: "x", description: "d", advisory: true, scope: "prose" };
    expect(withPatternOutcome(base, "fails")).toEqual({
      regex: "x",
      description: "d",
      scope: "prose",
      fails: true,
    });
    expect(withPatternOutcome({ ...base, fails: true }, "reports")).toEqual({
      regex: "x",
      description: "d",
      scope: "prose",
    });
    expect(patternOutcome(withPatternOutcome(base, "fails"))).toBe("fails");
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

  it("lists the voice's pattern rules", () => {
    const profile: VoiceProfile = {
      name: "Northsea",
      style: {
        prohibited_patterns: [
          { regex: "\\bsynergy\\b", description: "Corporate filler.", advisory: true },
        ],
      },
    };
    render(<RulesBlock profile={profile} />);
    expect(screen.getByText("Never write")).toBeInTheDocument();
    expect(screen.getAllByTestId("voice-pattern")).toHaveLength(1);
  });

  it("says a profile declares no pattern rule when it holds none", () => {
    render(<RulesBlock profile={{ name: "Plain" }} />);
    expect(screen.getByText("This profile declares no pattern rule.")).toBeInTheDocument();
  });
});
