import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, vi } from "vitest";
import { TabBar } from "../components/TabBar";

const defaultProps = {
  onSelect: vi.fn(),
  onClose: vi.fn(),
  onRename: vi.fn(),
};

describe("TabBar", () => {
  const tabs = [
    { id: "1", name: "Project A", path: "/a.kapi" },
    { id: "2", name: "Project B", path: "/b.kapi" },
  ];

  it("renders tab names", () => {
    render(<TabBar tabs={tabs} activeTabID="1" {...defaultProps} />);
    expect(screen.getByText("Project A")).toBeInTheDocument();
    expect(screen.getByText("Project B")).toBeInTheDocument();
  });

  it("calls onSelect when clicking a tab", async () => {
    const onSelect = vi.fn();
    render(<TabBar tabs={tabs} activeTabID="1" {...defaultProps} onSelect={onSelect} />);
    await userEvent.click(screen.getByText("Project B"));
    expect(onSelect).toHaveBeenCalledWith("2");
  });

  it("calls onClose when clicking close button", async () => {
    const onClose = vi.fn();
    render(<TabBar tabs={tabs} activeTabID="1" {...defaultProps} onClose={onClose} />);
    await userEvent.click(screen.getByLabelText("Close Project A"));
    expect(onClose).toHaveBeenCalledWith("1");
  });

  it("renders nothing when no tabs", () => {
    const { container } = render(
      <TabBar tabs={[]} activeTabID={null} {...defaultProps} />,
    );
    expect(container.innerHTML).toBe("");
  });

  it("highlights active tab", () => {
    render(<TabBar tabs={tabs} activeTabID="2" {...defaultProps} />);
    const activeTab = screen.getByText("Project B").closest("div");
    expect(activeTab?.className).toContain("font-semibold");
  });

  it("enters edit mode on double-click", async () => {
    render(<TabBar tabs={tabs} activeTabID="1" {...defaultProps} />);
    await userEvent.dblClick(screen.getByText("Project A"));
    expect(screen.getByLabelText("Rename project")).toBeInTheDocument();
    expect(screen.getByLabelText("Rename project")).toHaveValue("Project A");
  });

  it("calls onRename on Enter", async () => {
    const onRename = vi.fn();
    render(<TabBar tabs={tabs} activeTabID="1" {...defaultProps} onRename={onRename} />);
    await userEvent.dblClick(screen.getByText("Project A"));
    const input = screen.getByLabelText("Rename project");
    await userEvent.clear(input);
    await userEvent.type(input, "New Name{Enter}");
    expect(onRename).toHaveBeenCalledWith("1", "New Name");
  });
});
