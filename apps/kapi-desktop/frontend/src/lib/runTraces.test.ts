import { describe, expect, it } from "vitest";
import { sameSteps, traceFileKey, traceFileLabel } from "./runTraces";
import type { FlowStep } from "../types/api";

describe("sameSteps", () => {
  const linear: FlowStep[] = [
    { tool: "translate", config: { provider: "demo", model: "m" } },
    { tool: "qa" },
  ];

  it("accepts the same tools in the same order with the same options", () => {
    expect(sameSteps(structuredClone(linear), linear)).toBe(true);
  });

  it("ignores option key order, labels, and an absent versus empty config", () => {
    const ran: FlowStep[] = [
      { tool: "translate", config: { model: "m", provider: "demo" }, label: "Draft" },
      { tool: "qa", config: {} },
    ];
    expect(sameSteps(ran, linear)).toBe(true);
  });

  it("rejects a different tool, order, count, or option", () => {
    expect(sameSteps([{ tool: "translate" }, { tool: "qa" }], linear)).toBe(false);
    expect(sameSteps([linear[1], linear[0]], linear)).toBe(false);
    expect(sameSteps([linear[0]], linear)).toBe(false);
    expect(
      sameSteps(
        [{ tool: "translate", config: { provider: "other", model: "m" } }, { tool: "qa" }],
        linear,
      ),
    ).toBe(false);
    expect(sameSteps(undefined, linear)).toBe(false);
    expect(sameSteps(null, linear)).toBe(false);
  });

  it("compares parallel groups branch by branch", () => {
    const grouped: FlowStep[] = [
      { tool: "translate" },
      { tool: "", parallel: [{ tool: "qa" }, { tool: "word-count" }] },
    ];
    expect(sameSteps(structuredClone(grouped), grouped)).toBe(true);
    expect(
      sameSteps([{ tool: "translate" }, { tool: "", parallel: [{ tool: "qa" }] }], grouped),
    ).toBe(false);
    expect(
      sameSteps(
        [{ tool: "translate" }, { tool: "", parallel: [{ tool: "word-count" }, { tool: "qa" }] }],
        grouped,
      ),
    ).toBe(false);
  });

  it("compares nested option values structurally", () => {
    const ran: FlowStep[] = [{ tool: "qa", config: { rules: ["a", "b"], on: { x: 1 } } }];
    expect(sameSteps(ran, [{ tool: "qa", config: { on: { x: 1 }, rules: ["a", "b"] } }])).toBe(
      true,
    );
    expect(sameSteps(ran, [{ tool: "qa", config: { on: { x: 2 }, rules: ["a", "b"] } }])).toBe(
      false,
    );
    expect(sameSteps(ran, [{ tool: "qa", config: { on: { x: 1 }, rules: ["b", "a"] } }])).toBe(
      false,
    );
  });
});

describe("retained file naming", () => {
  it("labels a file by its name and locale pass", () => {
    expect(traceFileLabel({ file_path: "/p/src/messages.json", locale: "fr-FR" })).toBe(
      "messages.json · fr-FR",
    );
    expect(traceFileLabel({ file_path: "C:\\p\\src\\messages.json" })).toBe("messages.json");
    expect(traceFileLabel({ file_path: "messages.json" })).toBe("messages.json");
  });

  it("keys a file by path and locale so two passes of one file stay apart", () => {
    const nb = traceFileKey({ file_path: "/p/a.json", locale: "nb" });
    const de = traceFileKey({ file_path: "/p/a.json", locale: "de" });
    expect(nb).not.toBe(de);
    expect(traceFileKey({ file_path: "/p/a.json", locale: "nb" })).toBe(nb);
    expect(traceFileKey({ file_path: "/p/a.json" })).not.toBe(nb);
  });
});
