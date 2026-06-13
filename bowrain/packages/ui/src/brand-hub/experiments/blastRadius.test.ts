import { describe, it, expect } from "vite-plus/test";
import type { ChangeSetImpact } from "../../types/brand-graph";
import {
  byProject,
  byLocale,
  netViolationDelta,
  affectedShare,
  formatCompact,
  formatPercent,
  effortHours,
  formatEffort,
} from "./blastRadius";

const impact: ChangeSetImpact = {
  total_blocks: 1280,
  affected_blocks: 34,
  new_violations: 12,
  resolved: 7,
  words: 210,
  projects: [
    {
      project_id: "p-app",
      project_name: "Mobile App",
      affected_blocks: 12,
      new_violations: 4,
      resolved: 2,
      words: 70,
      collections: [
        {
          collection_id: "col-1",
          collection_name: "Strings",
          affected_blocks: 12,
          new_violations: 4,
          resolved: 2,
          words: 70,
          locales: [
            {
              stream: "main",
              locale: "de-DE",
              affected_blocks: 7,
              new_violations: 3,
              resolved: 1,
              words: 40,
            },
            {
              stream: "main",
              locale: "fr-FR",
              affected_blocks: 5,
              new_violations: 1,
              resolved: 1,
              words: 30,
            },
          ],
        },
      ],
    },
    {
      project_id: "p-web",
      project_name: "Marketing Website",
      affected_blocks: 22,
      new_violations: 8,
      resolved: 5,
      words: 140,
      collections: [
        {
          collection_id: "col-2",
          collection_name: "Pages",
          affected_blocks: 22,
          new_violations: 8,
          resolved: 5,
          words: 140,
          locales: [
            {
              stream: "main",
              locale: "de-DE",
              affected_blocks: 14,
              new_violations: 5,
              resolved: 3,
              words: 90,
            },
            {
              stream: "main",
              locale: "fr-FR",
              affected_blocks: 8,
              new_violations: 3,
              resolved: 2,
              words: 50,
            },
          ],
        },
      ],
    },
  ],
  samples: [],
};

describe("byProject", () => {
  it("orders projects by affected blocks, descending", () => {
    const bars = byProject(impact);
    expect(bars.map((b) => b.label)).toEqual(["Marketing Website", "Mobile App"]);
    expect(bars[0].affected_blocks).toBe(22);
  });
});

describe("byLocale", () => {
  it("merges a locale across every project and collection", () => {
    const bars = byLocale(impact);
    const de = bars.find((b) => b.label === "de-DE");
    const fr = bars.find((b) => b.label === "fr-FR");
    expect(de?.affected_blocks).toBe(21); // 7 + 14
    expect(de?.new_violations).toBe(8); // 3 + 5
    expect(fr?.affected_blocks).toBe(13); // 5 + 8
    expect(fr?.words).toBe(80); // 30 + 50
    // de-DE has the larger blast radius, so it sorts first.
    expect(bars[0].label).toBe("de-DE");
  });

  it("returns an empty break-down when there are no collections", () => {
    expect(byLocale({ ...impact, projects: [] })).toEqual([]);
  });
});

describe("scalar roll-ups", () => {
  it("computes net violation delta and affected share", () => {
    expect(netViolationDelta(impact)).toBe(5); // 12 - 7
    expect(affectedShare(impact)).toBeCloseTo(34 / 1280, 6);
    expect(affectedShare({ ...impact, total_blocks: 0 })).toBe(0);
  });
});

describe("formatting", () => {
  it("formats compact integers", () => {
    expect(formatCompact(980)).toBe("980");
    expect(formatCompact(1234)).toBe("1.2k");
    expect(formatCompact(2_000_000)).toBe("2.0M");
  });
  it("formats percentages with a sub-1% floor", () => {
    expect(formatPercent(0)).toBe("0%");
    expect(formatPercent(0.5)).toBe("50%");
    expect(formatPercent(0.005)).toBe("0.5%");
    expect(formatPercent(34 / 1280)).toBe("3%");
  });
  it("estimates review effort from words", () => {
    expect(effortHours(500)).toBe(1);
    expect(formatEffort(0)).toBe("—");
    expect(formatEffort(210)).toBe("~25 min");
    expect(formatEffort(2000)).toBe("~4.0 h");
  });
});
