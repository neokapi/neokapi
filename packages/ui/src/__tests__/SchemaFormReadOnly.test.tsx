// @vitest-environment jsdom
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { SchemaForm } from "../components/schema-form";
import type { ComponentSchema } from "../components/schema-form";

afterEach(cleanup);

const SCHEMA: ComponentSchema = {
  title: "Translate options",
  type: "object",
  properties: {
    provider: { type: "string", title: "Provider", description: "Which provider to use." },
    strict: { type: "boolean", title: "Strict", description: "Fail on any finding." },
  },
};

describe("SchemaForm read-only", () => {
  it("disables every field and drops input when readOnly", async () => {
    const onChange = vi.fn();
    render(
      <SchemaForm
        schema={SCHEMA}
        values={{ provider: "anthropic", strict: true }}
        onChange={onChange}
        readOnly
        hideHeader
      />,
    );
    const fieldset = document.querySelector("fieldset");
    expect(fieldset?.disabled).toBe(true);

    // The fieldset carries the disabled state; the input reports it through
    // the :disabled pseudo-class (its own `disabled` attribute stays unset).
    const provider = screen.getByDisplayValue("anthropic");
    expect(provider.matches(":disabled")).toBe(true);
    await userEvent.type(provider, "x");
    expect(onChange).not.toHaveBeenCalled();
  });

  it("leaves the fields editable otherwise", async () => {
    const onChange = vi.fn();
    render(
      <SchemaForm
        schema={SCHEMA}
        values={{ provider: "anthropic", strict: true }}
        onChange={onChange}
        hideHeader
      />,
    );
    expect(document.querySelector("fieldset")?.disabled).toBe(false);
    const provider = screen.getByDisplayValue("anthropic");
    expect(provider.matches(":disabled")).toBe(false);
    await userEvent.type(provider, "x");
    expect(onChange).toHaveBeenCalled();
  });
});
