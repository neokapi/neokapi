import { describe, it, expect } from "vitest";
import { render, screen } from "@testing-library/react";
import { TagValidationBar } from "../components/editor/TagValidationBar";
import type { TagValidationResult } from "../components/editor/tagSemantics";

describe("TagValidationBar", () => {
  it("renders nothing when validation is null", () => {
    const { container } = render(<TagValidationBar validation={null} />);
    expect(container.innerHTML).toBe("");
  });

  it("renders nothing when there are no errors or warnings", () => {
    const validation: TagValidationResult = { valid: true, errors: [], warnings: [] };
    const { container } = render(<TagValidationBar validation={validation} />);
    expect(container.innerHTML).toBe("");
  });

  it("renders error messages", () => {
    const validation: TagValidationResult = {
      valid: false,
      errors: [{ type: "missing_tag", message: 'Missing 1 opening "b" tag' }],
      warnings: [],
    };
    render(<TagValidationBar validation={validation} />);
    expect(screen.getByText('Missing 1 opening "b" tag')).toBeInTheDocument();
  });

  it("renders warning messages", () => {
    const validation: TagValidationResult = {
      valid: true,
      errors: [],
      warnings: [{ type: "extra_tag", message: 'Extra 1 closing "i" tag' }],
    };
    render(<TagValidationBar validation={validation} />);
    expect(screen.getByText('Extra 1 closing "i" tag')).toBeInTheDocument();
  });

  it("renders both errors and warnings", () => {
    const validation: TagValidationResult = {
      valid: false,
      errors: [{ type: "missing_tag", message: "Missing tag" }],
      warnings: [{ type: "extra_tag", message: "Extra tag" }],
    };
    render(<TagValidationBar validation={validation} />);
    expect(screen.getByText("Missing tag")).toBeInTheDocument();
    expect(screen.getByText("Extra tag")).toBeInTheDocument();
  });

  it("renders multiple errors", () => {
    const validation: TagValidationResult = {
      valid: false,
      errors: [
        { type: "missing_tag", message: "Error one" },
        { type: "unpaired", message: "Error two" },
      ],
      warnings: [],
    };
    render(<TagValidationBar validation={validation} />);
    expect(screen.getByText("Error one")).toBeInTheDocument();
    expect(screen.getByText("Error two")).toBeInTheDocument();
  });
});
