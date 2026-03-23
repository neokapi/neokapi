import { describe, it, expect, vi } from "vite-plus/test";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { StreamBadge } from "../components/StreamBadge";
import { StreamDiffView } from "../components/StreamDiffView";
import { StreamCreateDialog } from "../components/StreamCreateDialog";
import { StreamEditDialog } from "../components/StreamEditDialog";
import { StreamMergeDialog } from "../components/StreamMergeDialog";
import { StreamSelector } from "../components/StreamSelector";
import type {
  StreamInfo,
  StreamDiffResult,
  BlockChangeInfo,
  StreamMergeResult,
} from "../types/api";

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function makeStream(overrides: Partial<StreamInfo> = {}): StreamInfo {
  return {
    name: "feature/translations",
    parent: "main",
    base_cursor: 0,
    archived: false,
    visibility: "private",
    description: "A test stream",
    created_at: "2025-01-01T00:00:00Z",
    created_by: "user1",
    ...overrides,
  };
}

function makeDiff(overrides: Partial<StreamDiffResult> = {}): StreamDiffResult {
  return {
    stream_name: "feature/translations",
    parent_name: "main",
    changes: [],
    ...overrides,
  };
}

function makeChange(overrides: Partial<BlockChangeInfo> = {}): BlockChangeInfo {
  return {
    block_id: "block-1",
    change_type: "added",
    old_hash: "abc1234567890",
    new_hash: "def9876543210",
    ...overrides,
  };
}

function makeMergeResult(overrides: Partial<StreamMergeResult> = {}): StreamMergeResult {
  return {
    merged_blocks: 5,
    added_blocks: 2,
    modified_blocks: 2,
    removed_blocks: 1,
    ...overrides,
  };
}

// ---------------------------------------------------------------------------
// StreamBadge
// ---------------------------------------------------------------------------

describe("StreamBadge", () => {
  it("renders the stream name", () => {
    render(<StreamBadge stream={makeStream({ name: "my-stream" })} />);
    expect(screen.getByText("my-stream")).toBeInTheDocument();
  });

  it("compact mode shows shorter layout", () => {
    const { container } = render(
      <StreamBadge stream={makeStream({ name: "my-stream" })} compact />,
    );
    // Compact uses text-xs and a 1.5px dot; non-compact uses a border wrapper.
    // In compact mode there should be no border wrapper element.
    const outerSpan = container.firstElementChild as HTMLElement;
    expect(outerSpan.className).toContain("text-xs");
    expect(outerSpan.className).toContain("text-muted-foreground");
    // The dot in compact mode is h-1.5 w-1.5
    const dot = outerSpan.querySelector("span.rounded-full");
    expect(dot?.className).toContain("h-1.5");
  });

  it("shows title attribute with name and visibility in compact mode", () => {
    const { container } = render(
      <StreamBadge stream={makeStream({ name: "dev", visibility: "public" })} compact />,
    );
    const outerSpan = container.firstElementChild as HTMLElement;
    expect(outerSpan.getAttribute("title")).toBe("dev (public)");
  });
});

// ---------------------------------------------------------------------------
// StreamDiffView
// ---------------------------------------------------------------------------

describe("StreamDiffView", () => {
  it("shows 'No differences' message when changes array is empty", () => {
    render(<StreamDiffView diff={makeDiff()} />);
    expect(screen.getByText(/No differences/)).toBeInTheDocument();
  });

  it("renders stream names", () => {
    const diff = makeDiff({
      stream_name: "feature/a",
      parent_name: "main",
      changes: [makeChange()],
    });
    render(<StreamDiffView diff={diff} />);
    expect(screen.getByText("feature/a")).toBeInTheDocument();
    expect(screen.getByText("main")).toBeInTheDocument();
  });

  it("renders change counts", () => {
    const diff = makeDiff({
      changes: [makeChange(), makeChange({ block_id: "block-2" })],
    });
    render(<StreamDiffView diff={diff} />);
    expect(screen.getByText("2 changes")).toBeInTheDocument();
  });

  it("renders singular 'change' for a single change", () => {
    const diff = makeDiff({ changes: [makeChange()] });
    render(<StreamDiffView diff={diff} />);
    expect(screen.getByText("1 change")).toBeInTheDocument();
  });

  it("groups changes by type with labels", () => {
    const diff = makeDiff({
      changes: [
        makeChange({ block_id: "b1", change_type: "added" }),
        makeChange({ block_id: "b2", change_type: "modified" }),
        makeChange({ block_id: "b3", change_type: "removed" }),
      ],
    });
    render(<StreamDiffView diff={diff} />);
    expect(screen.getByText("Added (1)")).toBeInTheDocument();
    expect(screen.getByText("Modified (1)")).toBeInTheDocument();
    expect(screen.getByText("Removed (1)")).toBeInTheDocument();
  });

  it("renders block IDs and hash prefixes", () => {
    const diff = makeDiff({
      changes: [
        makeChange({
          block_id: "block-42",
          change_type: "modified",
          old_hash: "abc1234567890",
          new_hash: "def9876543210",
        }),
      ],
    });
    render(<StreamDiffView diff={diff} />);
    expect(screen.getByText("block-42")).toBeInTheDocument();
    // Hashes are sliced to 7 chars, joined by arrow
    expect(screen.getByText(/abc1234/)).toBeInTheDocument();
    expect(screen.getByText(/def9876/)).toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// StreamCreateDialog
// ---------------------------------------------------------------------------

describe("StreamCreateDialog", () => {
  const mainStream = makeStream({ name: "main", visibility: "public" });
  const devStream = makeStream({ name: "dev", visibility: "private" });

  it("renders form fields", () => {
    render(
      <StreamCreateDialog
        streams={[mainStream, devStream]}
        onSubmit={() => {}}
        onClose={() => {}}
        open={true}
      />,
    );
    expect(screen.getByText("Create Stream")).toBeInTheDocument();
    expect(screen.getByText("Name")).toBeInTheDocument();
    expect(screen.getByText("Parent Stream")).toBeInTheDocument();
    expect(screen.getByText("Visibility")).toBeInTheDocument();
    expect(screen.getByText("Description")).toBeInTheDocument();
  });

  it("calls onSubmit with form data", async () => {
    const user = userEvent.setup();
    const onSubmit = vi.fn();
    render(
      <StreamCreateDialog
        streams={[mainStream, devStream]}
        onSubmit={onSubmit}
        onClose={() => {}}
        open={true}
      />,
    );

    const nameInput = screen.getByPlaceholderText("feature/translations");
    await user.type(nameInput, "feature/new");

    const createButton = screen.getByRole("button", { name: "Create" });
    await user.click(createButton);

    expect(onSubmit).toHaveBeenCalledWith(
      expect.objectContaining({
        name: "feature/new",
        parent: "main",
        visibility: "private",
      }),
    );
  });

  it("validates required fields — Create button is disabled when name is empty", () => {
    render(
      <StreamCreateDialog
        streams={[mainStream]}
        onSubmit={() => {}}
        onClose={() => {}}
        open={true}
      />,
    );
    const createButton = screen.getByRole("button", { name: "Create" });
    expect(createButton).toBeDisabled();
  });
});

// ---------------------------------------------------------------------------
// StreamEditDialog
// ---------------------------------------------------------------------------

describe("StreamEditDialog", () => {
  const stream = makeStream({
    name: "feature/edit-me",
    description: "Original description",
    visibility: "shared",
  });

  it("renders with existing stream data", () => {
    render(<StreamEditDialog stream={stream} onSubmit={() => {}} onClose={() => {}} open={true} />);
    expect(screen.getByText(/Edit Stream/)).toBeInTheDocument();
    expect(screen.getByText(/feature\/edit-me/)).toBeInTheDocument();
    const descInput = screen.getByPlaceholderText("What is this stream for?");
    expect(descInput).toHaveValue("Original description");
  });

  it("calls onSubmit on save", async () => {
    const user = userEvent.setup();
    const onSubmit = vi.fn();
    render(<StreamEditDialog stream={stream} onSubmit={onSubmit} onClose={() => {}} open={true} />);

    const descInput = screen.getByPlaceholderText("What is this stream for?");
    await user.clear(descInput);
    await user.type(descInput, "Updated description");

    const saveButton = screen.getByRole("button", { name: "Save" });
    await user.click(saveButton);

    expect(onSubmit).toHaveBeenCalledWith(
      expect.objectContaining({
        description: "Updated description",
      }),
    );
  });
});

// ---------------------------------------------------------------------------
// StreamMergeDialog
// ---------------------------------------------------------------------------

describe("StreamMergeDialog", () => {
  it("renders merge confirmation UI", () => {
    render(
      <StreamMergeDialog
        result={makeMergeResult()}
        streamName="feature/done"
        parentName="main"
        onConfirm={() => {}}
        onClose={() => {}}
        open={true}
      />,
    );
    expect(screen.getByText("Merge Stream")).toBeInTheDocument();
    expect(screen.getByText("feature/done")).toBeInTheDocument();
    expect(screen.getByText("main")).toBeInTheDocument();
    expect(screen.getByText("Blocks added")).toBeInTheDocument();
    expect(screen.getByText("Blocks modified")).toBeInTheDocument();
    expect(screen.getByText("Blocks removed")).toBeInTheDocument();
  });

  it("calls onConfirm when merge button clicked", async () => {
    const user = userEvent.setup();
    const onConfirm = vi.fn();
    render(
      <StreamMergeDialog
        result={makeMergeResult()}
        streamName="feature/done"
        parentName="main"
        onConfirm={onConfirm}
        onClose={() => {}}
        open={true}
      />,
    );

    const confirmButton = screen.getByRole("button", { name: "Confirm Merge" });
    await user.click(confirmButton);

    expect(onConfirm).toHaveBeenCalledOnce();
  });

  it("disables Confirm Merge button when there are no changes", () => {
    render(
      <StreamMergeDialog
        result={makeMergeResult({
          merged_blocks: 0,
          added_blocks: 0,
          modified_blocks: 0,
          removed_blocks: 0,
        })}
        streamName="feature/empty"
        parentName="main"
        onConfirm={() => {}}
        onClose={() => {}}
        open={true}
      />,
    );
    const confirmButton = screen.getByRole("button", { name: "Confirm Merge" });
    expect(confirmButton).toBeDisabled();
  });
});

// ---------------------------------------------------------------------------
// StreamSelector
// ---------------------------------------------------------------------------

describe("StreamSelector", () => {
  const mainStream = makeStream({ name: "main", visibility: "public" });
  const devStream = makeStream({ name: "dev", visibility: "private" });
  const sharedStream = makeStream({ name: "shared-work", visibility: "shared" });

  it("renders active stream name", () => {
    render(
      <StreamSelector
        streams={[mainStream, devStream]}
        activeStream={devStream}
        onStreamChange={() => {}}
      />,
    );
    expect(screen.getByText("dev")).toBeInTheDocument();
  });

  it("shows available streams in dropdown", async () => {
    const user = userEvent.setup();
    render(
      <StreamSelector
        streams={[mainStream, devStream, sharedStream]}
        activeStream={mainStream}
        onStreamChange={() => {}}
      />,
    );

    // Open the dropdown by clicking the trigger
    const trigger = screen.getByText("main");
    await user.click(trigger);

    await waitFor(() => {
      expect(screen.getByText("dev")).toBeInTheDocument();
      expect(screen.getByText("shared-work")).toBeInTheDocument();
    });
  });

  it("shows 'Create stream' option when onCreateStream is provided", async () => {
    const user = userEvent.setup();
    render(
      <StreamSelector
        streams={[mainStream]}
        activeStream={mainStream}
        onStreamChange={() => {}}
        onCreateStream={() => {}}
      />,
    );

    const trigger = screen.getByText("main");
    await user.click(trigger);

    await waitFor(() => {
      expect(screen.getByText("Create stream")).toBeInTheDocument();
    });
  });
});
