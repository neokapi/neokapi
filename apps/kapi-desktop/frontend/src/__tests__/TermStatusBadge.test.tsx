import { render, screen } from "@testing-library/react";
import { describe, it, expect } from "vitest";
import { TermStatusBadge } from "@neokapi/ui-primitives";

describe("TermStatusBadge", () => {
  it("renders preferred status", () => {
    render(<TermStatusBadge status="preferred" />);
    expect(screen.getByText("preferred")).toBeInTheDocument();
  });

  it("renders approved status", () => {
    render(<TermStatusBadge status="approved" />);
    expect(screen.getByText("approved")).toBeInTheDocument();
  });

  it("renders admitted status", () => {
    render(<TermStatusBadge status="admitted" />);
    expect(screen.getByText("admitted")).toBeInTheDocument();
  });

  it("renders proposed status", () => {
    render(<TermStatusBadge status="proposed" />);
    expect(screen.getByText("proposed")).toBeInTheDocument();
  });

  it("renders deprecated status", () => {
    render(<TermStatusBadge status="deprecated" />);
    expect(screen.getByText("deprecated")).toBeInTheDocument();
  });

  it("renders forbidden status", () => {
    render(<TermStatusBadge status="forbidden" />);
    expect(screen.getByText("forbidden")).toBeInTheDocument();
  });

  it("applies line-through for deprecated status", () => {
    render(<TermStatusBadge status="deprecated" />);
    const badge = screen.getByText("deprecated");
    expect(badge.className).toContain("line-through");
  });

  it("applies line-through for forbidden status", () => {
    render(<TermStatusBadge status="forbidden" />);
    const badge = screen.getByText("forbidden");
    expect(badge.className).toContain("line-through");
  });

  it("does not apply line-through for preferred status", () => {
    render(<TermStatusBadge status="preferred" />);
    const badge = screen.getByText("preferred");
    expect(badge.className).not.toContain("line-through");
  });

  it("does not apply line-through for approved status", () => {
    render(<TermStatusBadge status="approved" />);
    const badge = screen.getByText("approved");
    expect(badge.className).not.toContain("line-through");
  });

  it("does not apply line-through for admitted status", () => {
    render(<TermStatusBadge status="admitted" />);
    const badge = screen.getByText("admitted");
    expect(badge.className).not.toContain("line-through");
  });

  it("does not apply line-through for proposed status", () => {
    render(<TermStatusBadge status="proposed" />);
    const badge = screen.getByText("proposed");
    expect(badge.className).not.toContain("line-through");
  });

  it("renders unknown status as-is", () => {
    render(<TermStatusBadge status="custom-status" />);
    expect(screen.getByText("custom-status")).toBeInTheDocument();
  });

  it("uses different hues for different statuses", () => {
    const { rerender } = render(<TermStatusBadge status="preferred" />);
    const preferredBg = screen.getByText("preferred").style.backgroundColor;

    rerender(<TermStatusBadge status="forbidden" />);
    const forbiddenBg = screen.getByText("forbidden").style.backgroundColor;

    // Different statuses should have different colors
    expect(preferredBg).not.toEqual(forbiddenBg);
  });

  it("applies custom className", () => {
    render(<TermStatusBadge status="approved" className="extra-class" />);
    const badge = screen.getByText("approved");
    expect(badge.className).toContain("extra-class");
  });
});
