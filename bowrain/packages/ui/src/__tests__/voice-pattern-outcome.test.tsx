import { describe, it, expect, vi } from "vite-plus/test";
import { render, screen } from "@testing-library/react";
import { PatternListEditor } from "../voice/PatternListEditor";
import { patternOutcome, withPatternOutcome } from "../voice/patternOutcome";
import type { Pattern } from "../voice/types";

// A voice pattern reports until a person marks it `fails: true`, and
// `advisory: true` keeps it reporting. The editors read the outcome from both
// keys and write the choice as `fails`.

describe("pattern outcome", () => {
  it("reads fails and advisory together", () => {
    expect(patternOutcome({})).toBe("reports");
    expect(patternOutcome({ fails: true })).toBe("fails");
    expect(patternOutcome({ advisory: true })).toBe("reports");
    expect(patternOutcome({ fails: true, advisory: true })).toBe("reports");
  });

  it("writes the chosen outcome as fails and clears advisory", () => {
    const base: Pattern = { regex: "x", description: "d", advisory: true };
    expect(withPatternOutcome(base, "fails")).toEqual({
      regex: "x",
      description: "d",
      fails: true,
    });
    expect(withPatternOutcome({ ...base, fails: true }, "reports")).toEqual({
      regex: "x",
      description: "d",
    });
  });

  it("shows each pattern's outcome in the list editor", () => {
    render(
      <PatternListEditor
        label="Prohibited"
        patterns={[
          { regex: "a", description: "unmarked" },
          { regex: "b", description: "marked", fails: true },
          { regex: "c", description: "both", fails: true, advisory: true },
        ]}
        onChange={vi.fn()}
      />,
    );
    const outcomes = screen.getAllByLabelText("When it matches").map((el) => el.textContent);
    expect(outcomes).toEqual(["Reports", "Fails", "Reports"]);
  });
});
