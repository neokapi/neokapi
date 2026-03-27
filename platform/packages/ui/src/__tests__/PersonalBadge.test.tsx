import { describe, it, expect } from "vite-plus/test";
import { render, screen } from "@testing-library/react";
import { PersonalBadge } from "../components/PersonalBadge";

describe("PersonalBadge", () => {
  it("renders the badge with 'Personal' text", () => {
    render(<PersonalBadge />);
    expect(screen.getByText("Personal")).toBeInTheDocument();
  });

  it("renders a User icon (svg element)", () => {
    const { container } = render(<PersonalBadge />);
    expect(container.querySelector("svg")).toBeInTheDocument();
  });

  it("uses the outline badge variant", () => {
    render(<PersonalBadge />);
    const badge = screen.getByText("Personal").closest("[data-slot='badge']");
    expect(badge).toHaveAttribute("data-variant", "outline");
  });

  it("applies custom className", () => {
    render(<PersonalBadge className="my-custom-class" />);
    const badge = screen.getByText("Personal").closest("[data-slot='badge']");
    expect(badge).toHaveClass("my-custom-class");
  });
});
