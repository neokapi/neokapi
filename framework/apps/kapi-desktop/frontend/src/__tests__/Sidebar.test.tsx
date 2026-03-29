import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, vi } from "vitest";
import { Sidebar } from "../components/Sidebar";

describe("Sidebar", () => {
  it("renders project navigation items", () => {
    render(<Sidebar activeView="project" onViewChange={vi.fn()} />);
    expect(screen.getByText("Project")).toBeInTheDocument();
    expect(screen.getByText("Flows")).toBeInTheDocument();
    expect(screen.getByText("Tools")).toBeInTheDocument();
  });

  it("calls onViewChange when clicking nav item", async () => {
    const onViewChange = vi.fn();
    render(<Sidebar activeView="project" onViewChange={onViewChange} />);
    await userEvent.click(screen.getByText("Flows"));
    expect(onViewChange).toHaveBeenCalledWith("flows");
  });

  it("shows flow list when flows view is active", () => {
    render(
      <Sidebar
        activeView="flows"
        onViewChange={vi.fn()}
        flowNames={["translate", "pseudo"]}
        selectedFlow="translate"
        onSelectFlow={vi.fn()}
        onAddFlow={vi.fn()}
        onDeleteFlow={vi.fn()}
      />,
    );
    expect(screen.getByText("translate")).toBeInTheDocument();
    expect(screen.getByText("pseudo")).toBeInTheDocument();
  });

  it("does not show flow list when project view is active", () => {
    render(
      <Sidebar
        activeView="project"
        onViewChange={vi.fn()}
        flowNames={["translate"]}
      />,
    );
    expect(screen.queryByText("translate")).not.toBeInTheDocument();
  });

  it("calls onSelectFlow when clicking a flow", async () => {
    const onSelectFlow = vi.fn();
    render(
      <Sidebar
        activeView="flows"
        onViewChange={vi.fn()}
        flowNames={["translate"]}
        onSelectFlow={onSelectFlow}
      />,
    );
    await userEvent.click(screen.getByText("translate"));
    expect(onSelectFlow).toHaveBeenCalledWith("translate");
  });

  it("calls onAddFlow when clicking +", async () => {
    const onAddFlow = vi.fn();
    render(
      <Sidebar
        activeView="flows"
        onViewChange={vi.fn()}
        flowNames={[]}
        onAddFlow={onAddFlow}
      />,
    );
    await userEvent.click(screen.getByLabelText("New flow"));
    expect(onAddFlow).toHaveBeenCalledOnce();
  });
});
