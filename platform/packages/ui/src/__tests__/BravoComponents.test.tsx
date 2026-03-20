import { describe, it, expect, vi } from "vite-plus/test";
import { render, screen, fireEvent } from "@testing-library/react";
import { BravoPanelTrigger } from "../components/bravo/BravoPanelTrigger";
import { BravoConversationList } from "../components/bravo/BravoConversationList";
import { BravoApprovalCard } from "../components/bravo/BravoApprovalCard";
import type { BravoConversation } from "../types/api";

// ---------------------------------------------------------------------------
// Fixtures
// ---------------------------------------------------------------------------

const conv1: BravoConversation = {
  id: "c1",
  workspace_id: "1",
  user_id: "u1",
  project_id: "",
  title: "First chat",
  status: "active",
  created_at: new Date().toISOString(),
  updated_at: new Date().toISOString(),
};

const conv2: BravoConversation = {
  id: "c2",
  workspace_id: "1",
  user_id: "u1",
  project_id: "",
  title: "Second chat",
  status: "completed",
  created_at: new Date().toISOString(),
  updated_at: new Date().toISOString(),
};

// ---------------------------------------------------------------------------
// BravoPanelTrigger
// ---------------------------------------------------------------------------

describe("BravoPanelTrigger", () => {
  it("renders trigger button with aria-label", () => {
    render(<BravoPanelTrigger onClick={vi.fn()} />);
    expect(screen.getByLabelText("Toggle @bravo assistant")).toBeDefined();
  });

  it("calls onClick when clicked", () => {
    const onClick = vi.fn();
    render(<BravoPanelTrigger onClick={onClick} />);
    fireEvent.click(screen.getByLabelText("Toggle @bravo assistant"));
    expect(onClick).toHaveBeenCalledTimes(1);
  });

  it("shows unread indicator when hasUnread is true", () => {
    const { container } = render(<BravoPanelTrigger onClick={vi.fn()} hasUnread />);
    const dot = container.querySelector(".rounded-full.bg-primary");
    expect(dot).not.toBeNull();
  });

  it("does not show unread indicator by default", () => {
    const { container } = render(<BravoPanelTrigger onClick={vi.fn()} />);
    const dot = container.querySelector(".rounded-full.bg-primary");
    expect(dot).toBeNull();
  });
});

// ---------------------------------------------------------------------------
// BravoConversationList
// ---------------------------------------------------------------------------

describe("BravoConversationList", () => {
  it("renders new conversation button", () => {
    render(<BravoConversationList conversations={[]} />);
    expect(screen.getByText("New conversation")).toBeDefined();
  });

  it("shows empty state when no conversations and not loading", () => {
    render(<BravoConversationList conversations={[]} />);
    expect(screen.getByText("No conversations yet")).toBeDefined();
  });

  it("shows loading state when loading with no conversations", () => {
    render(<BravoConversationList conversations={[]} loading />);
    expect(screen.getByText("Loading...")).toBeDefined();
  });

  it("renders conversation titles", () => {
    render(<BravoConversationList conversations={[conv1, conv2]} />);
    expect(screen.getByText("First chat")).toBeDefined();
    expect(screen.getByText("Second chat")).toBeDefined();
  });

  it("shows 'Untitled' for conversations without a title", () => {
    const untitled = { ...conv1, title: "" };
    render(<BravoConversationList conversations={[untitled]} />);
    expect(screen.getByText("Untitled")).toBeDefined();
  });

  it("calls onNew when new conversation button is clicked", () => {
    const onNew = vi.fn();
    render(<BravoConversationList conversations={[]} onNew={onNew} />);
    fireEvent.click(screen.getByText("New conversation"));
    expect(onNew).toHaveBeenCalledTimes(1);
  });

  it("calls onSelect when a conversation is clicked", () => {
    const onSelect = vi.fn();
    render(<BravoConversationList conversations={[conv1]} onSelect={onSelect} />);
    fireEvent.click(screen.getByText("First chat"));
    expect(onSelect).toHaveBeenCalledWith(conv1);
  });

  it("calls onDelete with the conversation", () => {
    const onDelete = vi.fn();
    render(<BravoConversationList conversations={[conv1]} onDelete={onDelete} />);
    const deleteBtn = screen.getByLabelText("Delete conversation");
    fireEvent.click(deleteBtn);
    expect(onDelete).toHaveBeenCalledWith(conv1);
  });
});

// ---------------------------------------------------------------------------
// BravoApprovalCard
// ---------------------------------------------------------------------------

describe("BravoApprovalCard", () => {
  it("renders tool name and approval required text", () => {
    render(
      <BravoApprovalCard
        toolCallId="tc1"
        toolName="translate"
        onApprove={vi.fn()}
        onDeny={vi.fn()}
      />,
    );
    expect(screen.getByText("Approval required")).toBeDefined();
    expect(screen.getByText("translate")).toBeDefined();
  });

  it("renders input JSON when provided", () => {
    render(
      <BravoApprovalCard
        toolCallId="tc1"
        toolName="translate"
        input={{ text: "hello" }}
        onApprove={vi.fn()}
        onDeny={vi.fn()}
      />,
    );
    expect(screen.getByText(/"text": "hello"/)).toBeDefined();
  });

  it("calls onApprove with toolCallId", () => {
    const onApprove = vi.fn();
    render(
      <BravoApprovalCard
        toolCallId="tc1"
        toolName="translate"
        onApprove={onApprove}
        onDeny={vi.fn()}
      />,
    );
    fireEvent.click(screen.getByText("Approve"));
    expect(onApprove).toHaveBeenCalledWith("tc1");
  });

  it("calls onDeny with toolCallId", () => {
    const onDeny = vi.fn();
    render(
      <BravoApprovalCard
        toolCallId="tc1"
        toolName="translate"
        onApprove={vi.fn()}
        onDeny={onDeny}
      />,
    );
    fireEvent.click(screen.getByText("Deny"));
    expect(onDeny).toHaveBeenCalledWith("tc1");
  });

  it("disables buttons when loading", () => {
    render(
      <BravoApprovalCard
        toolCallId="tc1"
        toolName="translate"
        onApprove={vi.fn()}
        onDeny={vi.fn()}
        loading
      />,
    );
    const approveBtn = screen.getByText("Approve").closest("button");
    const denyBtn = screen.getByText("Deny").closest("button");
    expect(approveBtn?.disabled).toBe(true);
    expect(denyBtn?.disabled).toBe(true);
  });
});
