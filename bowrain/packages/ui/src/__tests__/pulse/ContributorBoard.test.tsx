import { describe, it, expect } from "vitest";
import { render, screen } from "@testing-library/react";
import { ContributorBoard } from "../../components/pulse";
import { mockContributors } from "../../stories/pulse/pulse-fixtures";

describe("ContributorBoard", () => {
  it("renders contributor names", () => {
    render(<ContributorBoard contributors={mockContributors} />);
    expect(screen.getByText("Alice Chen")).toBeTruthy();
    expect(screen.getByText("Bob Smith")).toBeTruthy();
    expect(screen.getByText("Carlos Ruiz")).toBeTruthy();
    expect(screen.getByText("Yuki Tanaka")).toBeTruthy();
  });

  it("shows translation counts", () => {
    render(<ContributorBoard contributors={mockContributors} />);
    expect(screen.getByText("450 translations · 120 reviews")).toBeTruthy();
    expect(screen.getByText("320 translations · 85 reviews")).toBeTruthy();
  });

  it("shows empty state when no contributors provided", () => {
    render(<ContributorBoard contributors={[]} />);
    expect(screen.getByText("No contributors yet.")).toBeTruthy();
  });
});
