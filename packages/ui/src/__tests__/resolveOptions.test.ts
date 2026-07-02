import { describe, it, expect } from "vitest";
import { resolveEffectiveOptions } from "../components/schema-form/hooks/resolveOptions";
import type { PropertySchema } from "../components/schema-form/types";

// A cascading provider select whose options depend on the `engine` field —
// the translate tool's two-level engine→provider shape.
const provider: PropertySchema = {
  type: "string",
  options: [
    { value: "anthropic", label: "Anthropic" },
    { value: "acme-mt", label: "Acme MT" },
  ],
  "ui:option-sets": [
    { when: { field: "engine", eq: "llm" }, options: [{ value: "anthropic", label: "Anthropic" }] },
    { when: { field: "engine", eq: "mt" }, options: [{ value: "acme-mt", label: "Acme MT" }] },
  ],
};

function values(p?: PropertySchema, allValues?: Record<string, unknown>): string[] {
  return (resolveEffectiveOptions(p ?? provider, allValues) ?? []).map((o) => String(o.value));
}

describe("resolveEffectiveOptions", () => {
  it("offers only the LLM providers when engine=llm", () => {
    expect(values(provider, { engine: "llm" })).toEqual(["anthropic"]);
  });

  it("offers only the MT providers when engine=mt", () => {
    expect(values(provider, { engine: "mt" })).toEqual(["acme-mt"]);
  });

  it("falls back to the flat union when the gating field is unset", () => {
    expect(values(provider, {})).toEqual(["anthropic", "acme-mt"]);
  });

  it("returns the plain options when no option-sets are declared", () => {
    const plain: PropertySchema = {
      type: "string",
      options: [{ value: "a", label: "A" }],
    };
    expect(values(plain, { engine: "llm" })).toEqual(["a"]);
  });
});
