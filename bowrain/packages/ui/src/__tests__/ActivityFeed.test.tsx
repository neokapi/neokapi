import { describe, it, expect, vi } from "vite-plus/test";
import { render, screen, act } from "@testing-library/react";
import { ActivityFeed } from "../components/ActivityFeed";
import type { ActivityInfo } from "../types/api";

function makeActivity(overrides: Partial<ActivityInfo> = {}): ActivityInfo {
  return {
    id: "act-1",
    workspace_id: "ws-1",
    project_id: "proj-1",
    stream: "",
    actor_id: "user-1",
    actor_name: "Alice",
    type: "project.created",
    entity_type: "project",
    entity_id: "proj-1",
    summary: "created project Demo",
    data: { name: "Demo" },
    created_at: new Date().toISOString(),
    ...overrides,
  };
}

describe("ActivityFeed", () => {
  it("renders empty state when no activities", () => {
    render(<ActivityFeed activities={[]} />);
    expect(screen.getByText("No activities yet")).toBeInTheDocument();
  });

  it("renders a skeleton when loading with no activities", () => {
    const { container } = render(<ActivityFeed activities={[]} loading />);
    expect(container.querySelectorAll('[data-slot="skeleton"]').length).toBeGreaterThan(0);
  });

  it("renders activity items", () => {
    const activities = [
      makeActivity({ id: "a1", actor_name: "Alice", summary: "created project" }),
      makeActivity({ id: "a2", actor_name: "Bob", summary: "pushed files", type: "item.pushed" }),
    ];
    render(<ActivityFeed activities={activities} />);
    expect(screen.getByText("Alice")).toBeInTheDocument();
    expect(screen.getByText("Bob")).toBeInTheDocument();
    expect(screen.getByText("created project")).toBeInTheDocument();
    expect(screen.getByText("pushed files")).toBeInTheDocument();
  });

  it("calls onActivityClick when an activity is clicked", () => {
    const handleClick = vi.fn();
    const activity = makeActivity();
    render(<ActivityFeed activities={[activity]} onActivityClick={handleClick} />);
    act(() => screen.getByText("Alice").closest("button")!.click());
    expect(handleClick).toHaveBeenCalledWith(activity);
  });

  it("shows Load more button when hasMore is true", () => {
    render(<ActivityFeed activities={[makeActivity()]} hasMore />);
    expect(screen.getByText("Load more")).toBeInTheDocument();
  });

  it("calls onLoadMore when Load more is clicked", () => {
    const handleLoadMore = vi.fn();
    render(<ActivityFeed activities={[makeActivity()]} hasMore onLoadMore={handleLoadMore} />);
    act(() => screen.getByText("Load more").click());
    expect(handleLoadMore).toHaveBeenCalledOnce();
  });

  it("shows Loading... on load more button when loading with existing data", () => {
    render(<ActivityFeed activities={[makeActivity()]} hasMore loading />);
    expect(screen.getByText("Loading...")).toBeInTheDocument();
  });

  it("does not show Load more when hasMore is false", () => {
    render(<ActivityFeed activities={[makeActivity()]} hasMore={false} />);
    expect(screen.queryByText("Load more")).not.toBeInTheDocument();
  });

  it("shows System when actor_name is empty", () => {
    const activity = makeActivity({ actor_name: "" });
    render(<ActivityFeed activities={[activity]} />);
    expect(screen.getByText("System")).toBeInTheDocument();
  });

  it("renders the compact push summary verbatim", () => {
    const activity = makeActivity({
      type: "item.pushed",
      summary: "pushed 474 files · 20,345 blocks",
    });
    render(<ActivityFeed activities={[activity]} />);
    expect(screen.getByText("pushed 474 files · 20,345 blocks")).toBeInTheDocument();
  });

  it("truncates a legacy wall-of-paths summary so no row is a wall of text", () => {
    // A historical row that enumerated every pushed path in one string.
    const paths = Array.from({ length: 300 }, (_, i) => `web/docs/section-${i}/page.md`);
    const legacy = "pushed " + paths.join(", ");
    const activity = makeActivity({ id: "legacy", type: "item.pushed", summary: legacy });
    const { container } = render(<ActivityFeed activities={[activity]} />);

    const summaryEl = container.querySelector("p.text-sm span.text-muted-foreground");
    expect(summaryEl).not.toBeNull();
    const text = summaryEl!.textContent ?? "";
    expect(text.length).toBeLessThanOrEqual(201); // 200 chars + ellipsis
    expect(text.endsWith("…")).toBe(true);
    // The full list is not rendered.
    expect(text).not.toContain("section-299");
  });
});
