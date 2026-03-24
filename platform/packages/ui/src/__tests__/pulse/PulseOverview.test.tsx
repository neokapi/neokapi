import { describe, it, expect } from "vitest";
import { render, screen } from "@testing-library/react";
import { PulseOverview } from "../../components/pulse";
import { mockStats, mockProjects, mockLanguages } from "../../stories/pulse/pulse-fixtures";

describe("PulseOverview", () => {
  it("renders stat cards", () => {
    render(
      <PulseOverview stats={mockStats} projects={mockProjects} languages={mockLanguages} />,
    );
    expect(screen.getByText("Projects")).toBeTruthy();
    expect(screen.getByText("Languages")).toBeTruthy();
    expect(screen.getByText("Overall Progress")).toBeTruthy();
  });

  it("renders project cards", () => {
    render(
      <PulseOverview stats={mockStats} projects={mockProjects} languages={mockLanguages} />,
    );
    expect(screen.getByText("Web Application")).toBeTruthy();
    expect(screen.getByText("Mobile App")).toBeTruthy();
    expect(screen.getByText("Documentation")).toBeTruthy();
  });

  it("renders empty state when no projects", () => {
    const emptyStats = { ...mockStats, total_projects: 0 };
    render(
      <PulseOverview stats={emptyStats} projects={[]} languages={[]} />,
    );
    expect(screen.getByText("No public projects yet.")).toBeTruthy();
  });
});
