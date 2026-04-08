// @vitest-environment jsdom
import { describe, it, expect } from "vitest";
import { createElement } from "react";
import { createRoot } from "react-dom/client";
import { act } from "react";
import { TagValidationBar } from "../components/editor/TagValidationBar";
import type { TagValidationResult } from "../components/editor/tagSemantics";

function render(el: React.ReactElement): HTMLDivElement {
  const container = document.createElement("div");
  document.body.appendChild(container);
  act(() => {
    createRoot(container).render(el);
  });
  return container;
}

describe("TagValidationBar", () => {
  it("renders nothing when validation is null", () => {
    const c = render(createElement(TagValidationBar, { validation: null }));
    expect(c.innerHTML).toBe("");
  });

  it("renders nothing when there are no errors or warnings", () => {
    const validation: TagValidationResult = { valid: true, errors: [], warnings: [] };
    const c = render(createElement(TagValidationBar, { validation }));
    expect(c.innerHTML).toBe("");
  });

  it("renders error messages", () => {
    const validation: TagValidationResult = {
      valid: false,
      errors: [{ type: "missing_tag", message: 'Missing 1 opening "b" tag' }],
      warnings: [],
    };
    const c = render(createElement(TagValidationBar, { validation }));
    expect(c.textContent).toContain('Missing 1 opening "b" tag');
  });

  it("renders warning messages", () => {
    const validation: TagValidationResult = {
      valid: true,
      errors: [],
      warnings: [{ type: "extra_tag", message: 'Extra 1 closing "i" tag' }],
    };
    const c = render(createElement(TagValidationBar, { validation }));
    expect(c.textContent).toContain('Extra 1 closing "i" tag');
  });

  it("renders both errors and warnings", () => {
    const validation: TagValidationResult = {
      valid: false,
      errors: [{ type: "missing_tag", message: "Missing tag" }],
      warnings: [{ type: "extra_tag", message: "Extra tag" }],
    };
    const c = render(createElement(TagValidationBar, { validation }));
    expect(c.textContent).toContain("Missing tag");
    expect(c.textContent).toContain("Extra tag");
  });
});
