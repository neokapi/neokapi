import { describe, it, expect } from "vitest";
import { defToSpec, specToDef, parseBinding, formatBinding } from "../defAdapter";
import type { FlowDefinitionInfo, ToolInfo } from "../types";

describe("parseBinding (wire locator → internal FlowBinding)", () => {
  it("maps file/store/none kinds", () => {
    expect(parseBinding("file")).toEqual({ kind: "file" });
    expect(parseBinding("store")).toEqual({ kind: "store" });
    expect(parseBinding("none")).toEqual({ kind: "none" });
  });

  it("maps interchange format locators to interchange bindings", () => {
    expect(parseBinding("xliff")).toEqual({ kind: "interchange", format: "xliff" });
    expect(parseBinding("po")).toEqual({ kind: "interchange", format: "po" });
    expect(parseBinding("tmx")).toEqual({ kind: "interchange", format: "tmx" });
    expect(parseBinding("tbx")).toEqual({ kind: "interchange", format: "tbx" });
  });

  it("defaults to file for omitted / empty / unknown locators", () => {
    expect(parseBinding(undefined)).toEqual({ kind: "file" });
    expect(parseBinding(null)).toEqual({ kind: "file" });
    expect(parseBinding("")).toEqual({ kind: "file" });
    expect(parseBinding("bogus")).toEqual({ kind: "file" });
  });

  it("is case-insensitive and trims whitespace", () => {
    expect(parseBinding(" XLIFF ")).toEqual({ kind: "interchange", format: "xliff" });
    expect(parseBinding("Store")).toEqual({ kind: "store" });
  });
});

describe("formatBinding (internal FlowBinding → wire locator)", () => {
  it("omits the file default (returns undefined)", () => {
    expect(formatBinding({ kind: "file" })).toBeUndefined();
    expect(formatBinding(undefined)).toBeUndefined();
    expect(formatBinding(null)).toBeUndefined();
  });

  it("serializes store/none kinds", () => {
    expect(formatBinding({ kind: "store" })).toBe("store");
    expect(formatBinding({ kind: "none" })).toBe("none");
  });

  it("serializes interchange bindings as their format id", () => {
    expect(formatBinding({ kind: "interchange", format: "xliff" })).toBe("xliff");
    expect(formatBinding({ kind: "interchange", format: "po" })).toBe("po");
  });

  it("degrades a malformed interchange binding to the file default", () => {
    expect(formatBinding({ kind: "interchange" })).toBeUndefined();
    expect(formatBinding({ kind: "interchange", format: "weird" })).toBeUndefined();
  });

  it("round-trips through parseBinding for every non-default locator", () => {
    for (const locator of ["store", "none", "xliff", "po", "tmx", "tbx"]) {
      expect(formatBinding(parseBinding(locator))).toBe(locator);
    }
  });
});

const tools: ToolInfo[] = [
  { name: "translate", description: "Translate with AI", category: "translate" },
  { name: "qa", description: "QA check", category: "validate" },
  {
    name: "redact",
    description: "Redact sensitive content",
    category: "transform",
    isSourceTransform: true,
    recoverable: true,
  },
  { name: "unredact", description: "Restore originals", category: "transform" },
  { name: "term-check", description: "Term check", category: "validate" },
];

const base = { id: "f1", name: "My Flow", source: "user" as const };

describe("defToSpec", () => {
  it("converts a tool-only definition into a single-step spec", () => {
    const def: FlowDefinitionInfo = {
      id: "translate",
      name: "AI Translate",
      description: "Translate content",
      source: "built-in",
      nodes: [
        {
          id: "translate",
          type: "tool",
          name: "translate",
          label: "AI Translate",
          position: { x: 250, y: 100 },
        },
      ],
      edges: [],
    };

    const spec = defToSpec(def);
    expect(spec.steps).toHaveLength(1);
    expect(spec.steps[0].tool).toBe("translate");
    expect(spec.description).toBe("Translate content");
  });

  it("orders transformer nodes into spec.steps like any other node", () => {
    const def: FlowDefinitionInfo = {
      id: "secure-translate",
      name: "Secure Translate",
      source: "built-in",
      nodes: [
        {
          id: "redact",
          type: "tool",
          name: "redact",
          label: "Redact",
          position: { x: 250, y: 100 },
        },
        {
          id: "translate",
          type: "tool",
          name: "translate",
          label: "AI Translate",
          position: { x: 250, y: 360 },
        },
      ],
      edges: [],
    };

    const spec = defToSpec(def);
    expect(spec.steps.map((s) => s.tool)).toEqual(["redact", "translate"]);
  });

  it("carries config through to steps", () => {
    const def: FlowDefinitionInfo = {
      id: "f",
      name: "f",
      source: "user",
      nodes: [
        {
          id: "t",
          type: "tool",
          name: "translate",
          config: { provider: "anthropic", model: "claude" },
          position: { x: 250, y: 0 },
        },
      ],
      edges: [],
    };
    const spec = defToSpec(def);
    expect(spec.steps[0].config).toEqual({ provider: "anthropic", model: "claude" });
  });

  it("carries the I/O binding (string locators) through to spec.source / spec.sink", () => {
    const def: FlowDefinitionInfo = {
      id: "f",
      name: "f",
      source: "user",
      // Wire format: nested binding with string locators.
      binding: {
        source: "xliff",
        sink: "store",
      },
      nodes: [{ id: "t", type: "tool", name: "translate", position: { x: 0, y: 0 } }],
      edges: [],
    };
    const spec = defToSpec(def);
    // Steps spec uses the same flat string locators.
    expect(spec.source).toBe("xliff");
    expect(spec.sink).toBe("store");
  });
});

describe("specToDef", () => {
  it("produces tool-only nodes (no reader/writer)", () => {
    const def = specToDef({ steps: [{ tool: "translate" }] }, base, tools);
    expect(def.id).toBe("f1");
    expect(def.name).toBe("My Flow");
    expect(def.source).toBe("user");

    // A flow owns no I/O (AD-026): every node is a tool node.
    expect(def.nodes.every((n) => n.type === "tool")).toBe(true);
    expect(def.nodes).toHaveLength(1);
    expect(def.nodes[0].name).toBe("translate");
    // A single tool is both entry and exit, so no edges.
    expect(def.edges).toHaveLength(0);
  });

  it("emits transformer steps as ordinary ordered tool nodes", () => {
    const def = specToDef({ steps: [{ tool: "redact" }, { tool: "translate" }] }, base, tools);
    expect(def.nodes.map((n) => n.name)).toEqual(["redact", "translate"]);
    expect(def.nodes.every((n) => n.type === "tool")).toBe(true);
    // The persisted chain axis is y: redact precedes translate.
    expect(def.nodes[0].position.y).toBeLessThan(def.nodes[1].position.y);
  });

  it("carries spec.source / spec.sink (string locators) onto def.binding", () => {
    const def = specToDef(
      {
        steps: [{ tool: "translate" }],
        source: "po",
        sink: "none",
      },
      base,
      tools,
    );
    expect(def.binding).toEqual({
      source: "po",
      sink: "none",
    });
  });

  it("omits def.binding when the spec has no source/sink", () => {
    const def = specToDef({ steps: [{ tool: "translate" }] }, base, tools);
    expect(def.binding).toBeUndefined();
  });
});

describe("round-trip def → spec → def", () => {
  it("preserves tool order and parallel structure", () => {
    const original: FlowDefinitionInfo = {
      id: "secure",
      name: "Secure Translate",
      description: "Redact then translate",
      source: "user",
      nodes: [
        {
          id: "tool-0",
          type: "tool",
          name: "redact",
          label: "Redact",
          position: { x: 200, y: 0 },
        },
        {
          id: "tool-1",
          type: "tool",
          name: "translate",
          label: "AI Translate",
          position: { x: 200, y: 260 },
        },
        {
          id: "tool-2",
          type: "tool",
          name: "unredact",
          label: "Unredact",
          position: { x: 200, y: 520 },
        },
      ],
      edges: [],
    };

    const spec = defToSpec(original);
    const back = specToDef(spec, original, tools);

    const toolNames = back.nodes.filter((n) => n.type === "tool").map((n) => n.name);
    expect(toolNames).toEqual(["redact", "translate", "unredact"]);

    // The spec round-trips identically through a second pass.
    const spec2 = defToSpec(back);
    expect(spec2.steps.map((s) => s.tool)).toEqual(["redact", "translate", "unredact"]);
  });

  it("preserves a parallel group through spec → def → spec", () => {
    const spec = {
      steps: [
        { tool: "translate" },
        { tool: "", parallel: [{ tool: "qa" }, { tool: "term-check" }] },
      ],
    };
    const def = specToDef(spec, base, tools);
    const spec2 = defToSpec(def);

    expect(spec2.steps[0].tool).toBe("translate");
    expect(spec2.steps[1].parallel).toBeDefined();
    expect(spec2.steps[1].parallel!.map((p) => p.tool).sort()).toEqual(["qa", "term-check"]);
  });
});
