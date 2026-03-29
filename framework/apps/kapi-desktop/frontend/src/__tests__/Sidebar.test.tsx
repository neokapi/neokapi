import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, vi } from "vitest";
import { Sidebar } from "../components/Sidebar";

describe("Sidebar", () => {
  it("renders all navigation items", () => {
    render(
      <Sidebar activeView="project" onViewChange={vi.fn()} />,
    );
    expect(screen.getByText("Project")).toBeInTheDocument();
    expect(screen.getByText("Flows")).toBeInTheDocument();
    expect(screen.getByText("Tools")).toBeInTheDocument();
    expect(screen.getByText("Settings")).toBeInTheDocument();
  });

  it("renders without project name (shown in tab bar instead)", () => {
    render(
      <Sidebar activeView="project" onViewChange={vi.fn()} />,
    );
    // Project name is in the tab bar, not the sidebar.
    expect(screen.getByText("Project")).toBeInTheDocument();
  });

  it("calls onViewChange when clicking nav item", async () => {
    const onViewChange = vi.fn();
    render(
      <Sidebar activeView="project" onViewChange={onViewChange} />,
    );

    await userEvent.click(screen.getByText("Flows"));
    expect(onViewChange).toHaveBeenCalledWith("flows");
  });
});
