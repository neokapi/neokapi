import { describe, expect, it } from "vitest";
import {
  listBranchesInOrder,
  parallelStepError,
  sequenceIssues,
  type SequenceStep,
} from "../components/flow-editor/sequence";

// The refusal mirrors core/flow.CheckSequential word for word, so a flow the
// editor shows as invalid fails `kapi run` with the same text.
const refusal = (index: number, tools: string) =>
  `step[${index}] holds a parallel: list (${tools}), and flow steps run in order, one after another: list those tools as ordered steps`;

describe("parallelStepError", () => {
  it.each<[string, SequenceStep, number, string | null]>([
    ["a single tool", { tool: "qa" }, 0, null],
    ["an empty group", { tool: "", parallel: [] }, 0, null],
    [
      "branches named by tool",
      { tool: "", parallel: [{ tool: "qa" }, { tool: "placeholder-check" }] },
      1,
      refusal(1, "qa, placeholder-check"),
    ],
    [
      "a branch named by its label",
      { tool: "", parallel: [{ tool: "", label: "Checks" }, { tool: "qa" }] },
      2,
      refusal(2, "Checks, qa"),
    ],
    [
      "a branch with neither",
      { tool: "", parallel: [{ tool: "" }] },
      0,
      refusal(0, "a step with no tool"),
    ],
  ])("%s", (_name, step, index, want) => {
    expect(parallelStepError(step, index)).toBe(want);
  });
});

describe("sequenceIssues", () => {
  it("lists every step holding a group, in flow order", () => {
    const steps: SequenceStep[] = [
      { tool: "translate" },
      { tool: "", parallel: [{ tool: "qa" }] },
      { tool: "word-count" },
      { tool: "", parallel: [{ tool: "a" }, { tool: "b" }] },
    ];
    expect(sequenceIssues(steps)).toEqual([
      { index: 1, message: refusal(1, "qa") },
      { index: 3, message: refusal(3, "a, b") },
    ]);
    expect(sequenceIssues([{ tool: "translate" }, { tool: "qa" }])).toEqual([]);
  });
});

describe("listBranchesInOrder", () => {
  const steps: SequenceStep[] = [
    { tool: "translate" },
    { tool: "", label: "checks", parallel: [{ tool: "qa" }, { tool: "word-count" }] },
    { tool: "", parallel: [] },
    { tool: "", parallel: [{ tool: "a" }] },
  ];

  it("replaces every group with its branches", () => {
    expect(listBranchesInOrder(steps)).toEqual([
      { tool: "translate" },
      { tool: "qa" },
      { tool: "word-count" },
      { tool: "a" },
    ]);
  });

  it("replaces only the group at an index", () => {
    expect(listBranchesInOrder(steps, 1)).toEqual([
      { tool: "translate" },
      { tool: "qa" },
      { tool: "word-count" },
      { tool: "", parallel: [] },
      { tool: "", parallel: [{ tool: "a" }] },
    ]);
  });

  it("leaves a flow without groups as it is", () => {
    const plain: SequenceStep[] = [{ tool: "translate" }, { tool: "qa" }];
    expect(listBranchesInOrder(plain)).toEqual(plain);
  });
});
