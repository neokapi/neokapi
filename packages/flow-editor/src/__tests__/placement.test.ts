import { describe, it, expect } from "vitest";
import {
  computePlacement,
  placementByStep,
  RULE_TRANSFORMER_AFTER_TARGET,
  RULE_TRANSFORMER_AFTER_EGRESS,
  RULE_TRANSFORMER_LATE,
} from "../placement";
import { stepsToGraph, graphToSteps } from "../conversion";
import type { FlowSpec, ToolInfo } from "../types";

// ---------------------------------------------------------------------------
// computePlacement — the client-side mirror of the Go placement pass
// (core/flow/placement.go). Transformers are ordinary ordered steps; the pass
// validates their position.
// ---------------------------------------------------------------------------

function tool(name: string, extra: Partial<ToolInfo> = {}): [string, ToolInfo] {
  return [name, { name, description: "", category: "pipeline", ...extra }];
}

const tools = new Map<string, ToolInfo>([
  // Translators: produce a committed target. translate additionally sends
  // source to a remote provider (egress) unless configured with a local one.
  tool("pseudo-translate", {
    produces: [{ type: "target", side: "target" }],
  }),
  tool("translate", {
    produces: [{ type: "target", side: "target" }],
    side_effects: ["remote-source-egress"],
  }),
  // Plain transformer: rewrites the source, consumes nothing.
  tool("case-transform", {
    isSourceTransform: true,
    produces: [{ type: "source", side: "source" }],
  }),
  // Recoverable transformer: vaults originals; optionally entity-driven.
  tool("redact", {
    isSourceTransform: true,
    recoverable: true,
    consumes: [{ type: "entity", side: "source", optional: true }],
    produces: [
      { type: "source", side: "source" },
      { type: "redaction.secret", side: "source" },
    ],
  }),
  // unredact rewrites BOTH sides coherently — it produces the target port, so
  // it is exempt from the transformer-after-target rule.
  tool("unredact", {
    isSourceTransform: true,
    consumes: [{ type: "redaction.secret", side: "source" }],
    produces: [
      { type: "source", side: "source" },
      { type: "target", side: "target" },
    ],
  }),
  // Remote NER: produces the entity overlay, egresses source to do so.
  tool("entity-extract", {
    produces: [{ type: "entity", side: "source" }],
    side_effects: ["remote-source-egress"],
  }),
  // Overlay producer with no egress.
  tool("segmentation", {
    produces: [{ type: "segmentation", side: "source" }],
  }),
  tool("qa", {
    consumes: [{ type: "target", side: "target" }],
    produces: [{ type: "qa", side: "target" }],
  }),
]);

describe("computePlacement — transformer-after-target", () => {
  it("flags a transformer placed after a target-producing step", () => {
    const spec: FlowSpec = {
      steps: [{ tool: "pseudo-translate" }, { tool: "case-transform" }],
    };
    const diags = computePlacement(spec, tools);
    expect(diags).toHaveLength(1);
    expect(diags[0]).toMatchObject({
      severity: "error",
      rule: RULE_TRANSFORMER_AFTER_TARGET,
      stepIndex: 1,
      tool: "case-transform",
    });
  });

  it("does not flag a transformer placed before the translation", () => {
    const spec: FlowSpec = {
      steps: [{ tool: "case-transform" }, { tool: "pseudo-translate" }],
    };
    expect(computePlacement(spec, tools)).toEqual([]);
  });

  it("allows unredact after a translation (it produces the target port itself)", () => {
    const spec: FlowSpec = {
      steps: [{ tool: "redact" }, { tool: "translate" }, { tool: "unredact" }],
    };
    expect(computePlacement(spec, tools)).toEqual([]);
  });
});

describe("computePlacement — transformer-after-remote-egress", () => {
  it("flags a recoverable transformer after a remote-egress step", () => {
    const spec: FlowSpec = {
      steps: [{ tool: "entity-extract" }, { tool: "redact" }],
    };
    const diags = computePlacement(spec, tools);
    expect(diags).toHaveLength(1);
    expect(diags[0]).toMatchObject({
      severity: "error",
      rule: RULE_TRANSFORMER_AFTER_EGRESS,
      stepIndex: 1,
      tool: "redact",
    });
  });

  it("does not hold a non-recoverable transformer to the egress rule", () => {
    // case-transform after the remote NER: no targets upstream, not
    // recoverable — no diagnostics beyond the late-placement warning for the
    // unconsumed entity overlay.
    const spec: FlowSpec = {
      steps: [{ tool: "entity-extract" }, { tool: "case-transform" }],
    };
    const diags = computePlacement(spec, tools);
    expect(diags.filter((d) => d.rule === RULE_TRANSFORMER_AFTER_EGRESS)).toEqual([]);
  });

  it("exempts entity-driven redaction: the upstream NER produces a required port", () => {
    // detectors: ["entities"] makes redact's optional entity consume required,
    // and entity-extract is the step producing it — the AD-020 trade-off.
    const spec: FlowSpec = {
      steps: [{ tool: "entity-extract" }, { tool: "redact", config: { detectors: ["entities"] } }],
    };
    expect(computePlacement(spec, tools)).toEqual([]);
  });

  it("exempts an upstream configured with a local provider (egress stripped)", () => {
    const spec: FlowSpec = {
      steps: [{ tool: "entity-extract", config: { provider: "ollama" } }, { tool: "redact" }],
    };
    expect(computePlacement(spec, tools)).toEqual([]);
  });

  it("keeps the egress rule for a remote provider config", () => {
    const spec: FlowSpec = {
      steps: [{ tool: "entity-extract", config: { provider: "openai" } }, { tool: "redact" }],
    };
    const diags = computePlacement(spec, tools);
    expect(diags.map((d) => d.rule)).toEqual([RULE_TRANSFORMER_AFTER_EGRESS]);
  });
});

describe("computePlacement — transformer-late-placement", () => {
  it("warns when an overlay produced before the transformer must be rebased", () => {
    // segmentation writes a segmentation overlay; case-transform then rewrites
    // the source the overlay anchors to.
    const spec: FlowSpec = {
      steps: [{ tool: "segmentation" }, { tool: "case-transform" }],
    };
    const diags = computePlacement(spec, tools);
    expect(diags).toHaveLength(1);
    expect(diags[0]).toMatchObject({
      severity: "warning",
      rule: RULE_TRANSFORMER_LATE,
      stepIndex: 1,
      tool: "case-transform",
    });
    expect(diags[0].message).toContain("segmentation");
  });

  it("does not warn when the transformer consumes the overlay produced before it", () => {
    // The entity overlay is redact's (optional) input — producing it first is
    // exactly the right order, not a late placement.
    const spec: FlowSpec = {
      steps: [{ tool: "segmentation" }, { tool: "entity-extract" }],
    };
    // entity-extract is not a transformer at all, so nothing fires; and
    // redact directly after its entity feed is clean (local provider keeps the
    // egress rule out of the picture).
    expect(computePlacement(spec, tools)).toEqual([]);
    const fed: FlowSpec = {
      steps: [
        { tool: "entity-extract", config: { provider: "ollama" } },
        { tool: "redact", config: { detectors: ["entities"] } },
      ],
    };
    expect(computePlacement(fed, tools)).toEqual([]);
  });
});

describe("computePlacement — parallel groups", () => {
  it("a transformer after a parallel group that produces targets is flagged", () => {
    const spec: FlowSpec = {
      steps: [
        { tool: "", parallel: [{ tool: "pseudo-translate" }, { tool: "segmentation" }] },
        { tool: "case-transform" },
      ],
    };
    const diags = computePlacement(spec, tools);
    const afterTarget = diags.filter((d) => d.rule === RULE_TRANSFORMER_AFTER_TARGET);
    expect(afterTarget).toHaveLength(1);
    expect(afterTarget[0].stepIndex).toBe(1);
    // The group resolves as a merged contract named from its branches.
    expect(afterTarget[0].message).toContain("pseudo-translate+segmentation");
  });

  it("a transformer inside a parallel group is held to the rules at the group's slot", () => {
    const spec: FlowSpec = {
      steps: [
        { tool: "pseudo-translate" },
        { tool: "", parallel: [{ tool: "case-transform" }, { tool: "segmentation" }] },
      ],
    };
    const diags = computePlacement(spec, tools);
    expect(diags.filter((d) => d.rule === RULE_TRANSFORMER_AFTER_TARGET)).toHaveLength(1);
    expect(diags[0].stepIndex).toBe(1);
  });
});

describe("computePlacement — unknown tools and grouping", () => {
  it("skips steps whose tools are unknown to the tool map", () => {
    const spec: FlowSpec = {
      steps: [{ tool: "mystery-tool" }, { tool: "case-transform" }, { tool: "another-unknown" }],
    };
    expect(computePlacement(spec, tools)).toEqual([]);
  });

  it("an unknown upstream cannot trigger diagnostics against a transformer", () => {
    const spec: FlowSpec = {
      steps: [{ tool: "mystery-translate" }, { tool: "redact" }],
    };
    expect(computePlacement(spec, tools)).toEqual([]);
  });

  it("placementByStep groups diagnostics by step index", () => {
    const spec: FlowSpec = {
      steps: [{ tool: "translate" }, { tool: "redact" }],
    };
    // redact after translate violates BOTH rules: targets exist upstream and
    // the source already egressed.
    const diags = computePlacement(spec, tools);
    expect(diags.map((d) => d.rule).sort()).toEqual([
      RULE_TRANSFORMER_AFTER_EGRESS,
      RULE_TRANSFORMER_AFTER_TARGET,
    ]);
    const byStep = placementByStep(diags);
    expect([...byStep.keys()]).toEqual([1]);
    expect(byStep.get(1)).toHaveLength(2);
  });
});

// ---------------------------------------------------------------------------
// Conversion round-trips — transformers are plain ordered steps (no stage).
// ---------------------------------------------------------------------------

describe("conversion — ordered steps round-trip (no stage)", () => {
  it("round-trips ordered steps including leading transformers", () => {
    const original: FlowSpec = {
      steps: [
        { tool: "redact", config: { mode: "placeholder" } },
        { tool: "translate" },
        { tool: "qa" },
        { tool: "unredact" },
      ],
    };
    const { nodes } = stepsToGraph(original);
    const result = graphToSteps(nodes);
    expect(result.steps.map((s) => s.tool)).toEqual(["redact", "translate", "qa", "unredact"]);
    expect(result.steps[0].config).toEqual({ mode: "placeholder" });
  });

  it("threads stepIndex (and never a stage) onto node data, in declaration order", () => {
    const spec: FlowSpec = {
      steps: [{ tool: "redact" }, { tool: "case-transform" }, { tool: "translate" }],
    };
    const { nodes, edges } = stepsToGraph(spec, tools);
    expect(nodes.map((n) => n.id)).toEqual(["tool-0", "tool-1", "tool-2"]);
    nodes.forEach((n, i) => {
      expect(n.data.stepIndex).toBe(i);
      expect("stage" in n.data).toBe(false);
    });
    // Linear chain: tool-0 → tool-1 → tool-2.
    expect(edges.map((e) => [e.source, e.target])).toEqual([
      ["tool-0", "tool-1"],
      ["tool-1", "tool-2"],
    ]);
  });

  it("passes isSourceTransform (transformer flag) through to node data", () => {
    const spec: FlowSpec = { steps: [{ tool: "redact" }, { tool: "translate" }] };
    const { nodes } = stepsToGraph(spec, tools);
    expect(nodes[0].data.isSourceTransform).toBe(true);
    expect(nodes[1].data.isSourceTransform).toBeUndefined();
  });

  it("multiple round-trip passes are stable", () => {
    const original: FlowSpec = {
      steps: [{ tool: "redact" }, { tool: "translate" }, { tool: "qa" }],
    };
    const pass1 = graphToSteps(stepsToGraph(original).nodes);
    const pass2 = graphToSteps(stepsToGraph(pass1).nodes);
    expect(pass2).toEqual(pass1);
    expect(pass1.steps.map((s) => s.tool)).toEqual(["redact", "translate", "qa"]);
  });

  it("round-trips an empty flow", () => {
    const { nodes } = stepsToGraph({ steps: [] });
    expect(graphToSteps(nodes)).toEqual({ steps: [] });
  });
});
