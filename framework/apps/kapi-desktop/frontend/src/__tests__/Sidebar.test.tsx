import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, vi } from "vitest";
import { Sidebar } from "../components/Sidebar";

describe("Sidebar", () => {
  it("renders all navigation items", () => {
    render(
      <Sidebar
        activeView="project"
        onViewChange={vi.fn()}
        onCloseProject={vi.fn()}
      />,
    );
    expect(screen.getByText("Project")).toBeInTheDocument();
    expect(screen.getByText("Flows")).toBeInTheDocument();
    expect(screen.getByText("Tools")).toBeInTheDocument();
    expect(screen.getByText("Plugins")).toBeInTheDocument();
    expect(screen.getByText("Credentials")).toBeInTheDocument();
    expect(screen.getByText("Settings")).toBeInTheDocument();
  });

  it("displays project name when provided", () => {
    render(
      <Sidebar
        activeView="project"
        onViewChange={vi.fn()}
        projectName="My Project"
        onCloseProject={vi.fn()}
      />,
    );
    expect(screen.getByText("My Project")).toBeInTheDocument();
  });

  it("calls onViewChange when clicking nav item", async () => {
    const onViewChange = vi.fn();
    render(
      <Sidebar
        activeView="project"
        onViewChange={onViewChange}
        onCloseProject={vi.fn()}
      />,
    );

    await userEvent.click(screen.getByText("Flows"));
    expect(onViewChange).toHaveBeenCalledWith("flows");
  });

  it("calls onCloseProject when clicking close button", async () => {
    const onClose = vi.fn();
    render(
      <Sidebar
        activeView="project"
        onViewChange={vi.fn()}
        projectName="Test"
        onCloseProject={onClose}
      />,
    );

    await userEvent.click(screen.getByTitle("Close project"));
    expect(onClose).toHaveBeenCalledOnce();
  });
});
