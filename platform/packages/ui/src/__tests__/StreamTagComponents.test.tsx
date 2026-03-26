import { describe, it, expect, vi } from "vite-plus/test";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { StreamTagBadge } from "../components/StreamTagBadge";
import { StreamTagList } from "../components/StreamTagList";
import type { StreamTag } from "../types/api";

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function makeTag(overrides: Partial<StreamTag> = {}): StreamTag {
  return {
    id: "tag-1",
    project_id: "proj-1",
    stream: "feature/x",
    name: "v1.0",
    kind: "release",
    cursor: 42,
    created_by: "user-1",
    created_at: "2026-03-15T14:00:00Z",
    ...overrides,
  };
}

// ---------------------------------------------------------------------------
// StreamTagBadge
// ---------------------------------------------------------------------------

describe("StreamTagBadge", () => {
  it("renders the tag name", () => {
    render(<StreamTagBadge tag={makeTag({ name: "my-tag" })} />);
    expect(screen.getByText("my-tag")).toBeInTheDocument();
  });

  it("applies kind-specific color for merge", () => {
    const { container } = render(
      <StreamTagBadge tag={makeTag({ kind: "merge", name: "merged-main" })} />,
    );
    const badge = container.firstElementChild as HTMLElement;
    expect(badge.className).toContain("text-purple-");
  });

  it("applies kind-specific color for release", () => {
    const { container } = render(
      <StreamTagBadge tag={makeTag({ kind: "release", name: "v2.0" })} />,
    );
    const badge = container.firstElementChild as HTMLElement;
    expect(badge.className).toContain("text-blue-");
  });

  it("applies kind-specific color for milestone", () => {
    const { container } = render(
      <StreamTagBadge tag={makeTag({ kind: "milestone", name: "done" })} />,
    );
    const badge = container.firstElementChild as HTMLElement;
    expect(badge.className).toContain("text-emerald-");
  });

  it("applies kind-specific color for custom", () => {
    const { container } = render(
      <StreamTagBadge tag={makeTag({ kind: "custom", name: "qa" })} />,
    );
    const badge = container.firstElementChild as HTMLElement;
    expect(badge.className).toContain("text-gray-");
  });

  it("compact mode shows smaller layout", () => {
    const { container } = render(
      <StreamTagBadge tag={makeTag({ name: "compact-tag" })} compact />,
    );
    const badge = container.firstElementChild as HTMLElement;
    expect(badge.className).toContain("text-xs");
    // Compact mode should not have border styling.
    expect(badge.className).not.toContain("border");
  });

  it("shows title with kind label and name", () => {
    const { container } = render(
      <StreamTagBadge tag={makeTag({ name: "v3", kind: "release" })} compact />,
    );
    const badge = container.firstElementChild as HTMLElement;
    expect(badge.getAttribute("title")).toBe("Release: v3");
  });
});

// ---------------------------------------------------------------------------
// StreamTagList
// ---------------------------------------------------------------------------

describe("StreamTagList", () => {
  it("shows empty message when no tags", () => {
    render(<StreamTagList tags={[]} />);
    expect(screen.getByText(/No tags on this stream/)).toBeInTheDocument();
  });

  it("renders tags", () => {
    const tags = [
      makeTag({ id: "t1", name: "alpha", kind: "release" }),
      makeTag({ id: "t2", name: "beta", kind: "merge" }),
    ];
    render(<StreamTagList tags={tags} />);
    expect(screen.getByText("alpha")).toBeInTheDocument();
    expect(screen.getByText("beta")).toBeInTheDocument();
  });

  it("shows cursor values", () => {
    const tags = [makeTag({ cursor: 99 })];
    render(<StreamTagList tags={tags} />);
    expect(screen.getByText("cursor 99")).toBeInTheDocument();
  });

  it("hides delete buttons when onDelete is not provided", () => {
    const tags = [makeTag({ name: "no-delete" })];
    const { container } = render(<StreamTagList tags={tags} />);
    expect(container.querySelectorAll("button")).toHaveLength(0);
  });

  it("calls onDelete when delete button is clicked", async () => {
    const user = userEvent.setup();
    const onDelete = vi.fn();
    const tags = [makeTag({ name: "deletable" })];
    const { container } = render(<StreamTagList tags={tags} onDelete={onDelete} />);

    const btn = container.querySelector("button");
    expect(btn).toBeTruthy();
    await user.click(btn!);
    expect(onDelete).toHaveBeenCalledWith("deletable");
  });

  it("renders date from created_at", () => {
    const tags = [makeTag({ created_at: "2026-03-15T14:00:00Z" })];
    render(<StreamTagList tags={tags} />);
    // The formatted date should contain "Mar" and "2026".
    expect(screen.getByText(/Mar/)).toBeInTheDocument();
    expect(screen.getByText(/2026/)).toBeInTheDocument();
  });
});
