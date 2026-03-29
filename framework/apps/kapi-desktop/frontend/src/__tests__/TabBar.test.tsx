import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, vi } from "vitest";
import { TabBar } from "../components/TabBar";

describe("TabBar", () => {
  const tabs = [
    { id: "1", name: "Project A", path: "/a.kapi" },
    { id: "2", name: "Project B", path: "/b.kapi" },
  ];

  it("renders tab names", () => {
    render(
      <TabBar tabs={tabs} activeTabID="1" onSelect={vi.fn()} onClose={vi.fn()} />,
    );
    expect(screen.getByText("Project A")).toBeInTheDocument();
    expect(screen.getByText("Project B")).toBeInTheDocument();
  });

  it("calls onSelect when clicking a tab", async () => {
    const onSelect = vi.fn();
    render(
      <TabBar tabs={tabs} activeTabID="1" onSelect={onSelect} onClose={vi.fn()} />,
    );
    await userEvent.click(screen.getByText("Project B"));
    expect(onSelect).toHaveBeenCalledWith("2");
  });

  it("calls onClose when clicking close button", async () => {
    const onClose = vi.fn();
    render(
      <TabBar tabs={tabs} activeTabID="1" onSelect={vi.fn()} onClose={onClose} />,
    );
    await userEvent.click(screen.getByLabelText("Close Project A"));
    expect(onClose).toHaveBeenCalledWith("1");
  });

  it("renders nothing when no tabs", () => {
    const { container } = render(
      <TabBar tabs={[]} activeTabID={null} onSelect={vi.fn()} onClose={vi.fn()} />,
    );
    expect(container.innerHTML).toBe("");
  });

  it("highlights active tab", () => {
    render(
      <TabBar tabs={tabs} activeTabID="2" onSelect={vi.fn()} onClose={vi.fn()} />,
    );
    const activeTab = screen.getByText("Project B").closest("div");
    expect(activeTab?.className).toContain("border-primary");
  });
});
