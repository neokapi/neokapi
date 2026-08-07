import { render, screen } from "./testUtils";
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
    { id: "1", name: "Project A", path: "/a/kapi.yaml" },
    { id: "2", name: "Project B", path: "/b/kapi.yaml" },
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
    const { container } = render(<TabBar tabs={[]} activeTabID={null} {...defaultProps} />);
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

  it("exposes a tablist with tab roles and roving tabindex", () => {
    render(<TabBar tabs={tabs} activeTabID="2" {...defaultProps} />);
    expect(screen.getByRole("tablist")).toBeInTheDocument();
    const tabEls = screen.getAllByRole("tab");
    expect(tabEls).toHaveLength(2);
    // Only the active tab is in the tab order.
    expect(tabEls[0]).toHaveAttribute("tabindex", "-1");
    expect(tabEls[1]).toHaveAttribute("tabindex", "0");
    expect(tabEls[1]).toHaveAttribute("aria-selected", "true");
  });

  it("selects the focused tab on Enter", async () => {
    const onSelect = vi.fn();
    render(<TabBar tabs={tabs} activeTabID="1" {...defaultProps} onSelect={onSelect} />);
    const tabEls = screen.getAllByRole("tab");
    tabEls[1].focus();
    await userEvent.keyboard("{Enter}");
    expect(onSelect).toHaveBeenCalledWith("2");
  });

  it("moves selection with the arrow keys", async () => {
    const onSelect = vi.fn();
    render(<TabBar tabs={tabs} activeTabID="1" {...defaultProps} onSelect={onSelect} />);
    const tabEls = screen.getAllByRole("tab");
    tabEls[0].focus();
    await userEvent.keyboard("{ArrowRight}");
    expect(onSelect).toHaveBeenCalledWith("2");
  });

  it("enters edit mode on F2", async () => {
    render(<TabBar tabs={tabs} activeTabID="1" {...defaultProps} />);
    const tabEls = screen.getAllByRole("tab");
    tabEls[0].focus();
    await userEvent.keyboard("{F2}");
    expect(screen.getByLabelText("Rename project")).toBeInTheDocument();
  });
});
